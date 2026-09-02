package narrativeledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Store owns Narrative Ledger persistence for one project database.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// NewStore binds a migrated project database to the ledger.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db, now: time.Now}
}

// WithClock replaces wall time for deterministic tests.
func (s *Store) WithClock(now func() time.Time) *Store {
	if now != nil {
		s.now = now
	}
	return s
}

// Close closes the database owned by this store.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// ApplyAcceptedFinal is the production authority boundary for generated chapters.
func (s *Store) ApplyAcceptedFinal(ctx context.Context, input ChangeSet) (ApplyResult, error) {
	if input.Source.Authority != AuthorityAcceptedFinal {
		return ApplyResult{}, newError("LEDGER_AUTHORITY_REJECTED", "accepted-final authority is required", ErrAuthority)
	}
	return s.apply(ctx, input)
}

// ApplyHuman records an explicit local human mutation through the same audit path.
func (s *Store) ApplyHuman(ctx context.Context, input ChangeSet) (ApplyResult, error) {
	if input.Source.Authority != AuthorityHuman {
		return ApplyResult{}, newError("LEDGER_AUTHORITY_REJECTED", "human authority is required", ErrAuthority)
	}
	return s.apply(ctx, input)
}

func (s *Store) apply(ctx context.Context, input ChangeSet) (ApplyResult, error) {
	if s == nil || s.db == nil {
		return ApplyResult{}, newError("LEDGER_DATABASE_UNAVAILABLE", "narrative ledger database is unavailable", errors.New("nil database"))
	}
	normalized, err := normalizeChangeSet(input)
	if err != nil {
		return ApplyResult{}, err
	}
	hash, _, err := contentHash(normalized)
	if err != nil {
		return ApplyResult{}, newError("LEDGER_SERIALIZATION_FAILED", "narrative ledger change could not be serialized", err)
	}
	provenance, err := json.Marshal(normalized.Source.Provenance)
	if err != nil {
		return ApplyResult{}, newError("LEDGER_SERIALIZATION_FAILED", "narrative ledger provenance could not be serialized", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ApplyResult{}, newError("LEDGER_DATABASE_WRITE_FAILED", "narrative ledger transaction could not start", err)
	}
	defer tx.Rollback()

	existing, found, err := readCommit(ctx, tx, normalized.Source.TransactionID)
	if err != nil {
		return ApplyResult{}, err
	}
	if found {
		if existing.ContentHash != hash {
			return ApplyResult{}, newError("LEDGER_SOURCE_CONTENT_CONFLICT", "source transaction was replayed with different content", ErrConflict)
		}
		return ApplyResult{Replay: true, Commit: existing}, nil
	}

	committedAt := s.now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO narrative_ledger_commits(
		source_transaction_id, source_candidate_id, chapter, authority,
		content_hash, provenance_json, committed_at
	) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		normalized.Source.TransactionID,
		normalized.Source.CandidateID,
		normalized.Source.Chapter,
		normalized.Source.Authority,
		hash,
		string(provenance),
		committedAt.Format(time.RFC3339Nano),
	); err != nil {
		return ApplyResult{}, newError("LEDGER_DATABASE_WRITE_FAILED", "narrative ledger commit could not be recorded", err)
	}
	for _, change := range normalized.Foreshadows {
		if err := s.applyForeshadow(ctx, tx, normalized.Source, change, committedAt); err != nil {
			return ApplyResult{}, err
		}
	}
	for _, change := range normalized.Secrets {
		if err := s.applySecret(ctx, tx, normalized.Source, change, committedAt); err != nil {
			return ApplyResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ApplyResult{}, newError("LEDGER_DATABASE_WRITE_FAILED", "narrative ledger transaction could not commit", err)
	}
	return ApplyResult{
		Commit: Commit{
			TransactionID: normalized.Source.TransactionID,
			CandidateID:   normalized.Source.CandidateID,
			Chapter:       normalized.Source.Chapter,
			Authority:     normalized.Source.Authority,
			ContentHash:   hash,
			CommittedAt:   committedAt,
		},
	}, nil
}

func readCommit(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, transactionID string) (Commit, bool, error) {
	var result Commit
	var committedAt string
	err := queryer.QueryRowContext(ctx, `SELECT
		source_transaction_id, source_candidate_id, chapter, authority,
		content_hash, committed_at
	FROM narrative_ledger_commits WHERE source_transaction_id = ?`, transactionID).Scan(
		&result.TransactionID,
		&result.CandidateID,
		&result.Chapter,
		&result.Authority,
		&result.ContentHash,
		&committedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Commit{}, false, nil
	}
	if err != nil {
		return Commit{}, false, newError("LEDGER_DATABASE_READ_FAILED", "narrative ledger commit could not be read", err)
	}
	result.CommittedAt, err = time.Parse(time.RFC3339Nano, committedAt)
	if err != nil {
		return Commit{}, false, newError("LEDGER_DATA_INVALID", "narrative ledger commit timestamp is invalid", err)
	}
	return result, true, nil
}

func (s *Store) applyForeshadow(
	ctx context.Context,
	tx *sql.Tx,
	source Source,
	change ForeshadowChange,
	now time.Time,
) error {
	existing, found, err := scanForeshadow(ctx, tx, change.Key, source.Chapter)
	if err != nil {
		return err
	}
	action := change.Action
	if !found {
		if action == "reveal" || action == "abandon" || action == "reinforce" || action == "transition" || action == "update" {
			return newError("LEDGER_FORESHADOW_NOT_FOUND", "foreshadow does not exist for this transition", ErrNotFound)
		}
		priority := PriorityNormal
		if change.Priority != nil {
			priority = *change.Priority
		}
		status := ForeshadowPlanned
		if change.Status != nil {
			status = *change.Status
		}
		if action == "plant" {
			status = ForeshadowPlanted
		}
		title := change.Title
		if title == "" {
			title = change.Key
		}
		planted := change.PlantedChapter
		if status == ForeshadowPlanted && planted == nil {
			chapter := source.Chapter
			planted = &chapter
		}
		existing = Foreshadow{
			ID:                deterministicID("foreshadow", change.Key),
			Key:               change.Key,
			Title:             title,
			Description:       change.Description,
			Priority:          priority,
			Status:            status,
			EffectiveStatus:   status,
			PlantedChapter:    planted,
			DueChapter:        change.DueChapter,
			RevealChapter:     change.RevealChapter,
			SourceTransaction: source.TransactionID,
			UpdatedChapter:    source.Chapter,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if existing.Status == ForeshadowRevealed && existing.RevealChapter == nil {
			chapter := source.Chapter
			existing.RevealChapter = &chapter
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO foreshadows(
			id, key, title, description, priority, status, planted_chapter,
			due_chapter, reveal_chapter, source_transaction_id, updated_chapter,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			existing.ID,
			existing.Key,
			existing.Title,
			existing.Description,
			existing.Priority,
			existing.Status,
			nullableInt(existing.PlantedChapter),
			nullableInt(existing.DueChapter),
			nullableInt(existing.RevealChapter),
			source.TransactionID,
			source.Chapter,
			now.Format(time.RFC3339Nano),
			now.Format(time.RFC3339Nano),
		); err != nil {
			return newError("LEDGER_DATABASE_WRITE_FAILED", "foreshadow could not be created", err)
		}
	} else {
		target := existing.Status
		switch action {
		case "plant":
			target = ForeshadowPlanted
		case "reinforce":
			target = ForeshadowReinforced
		case "reveal":
			target = ForeshadowRevealed
		case "abandon":
			target = ForeshadowAbandoned
		case "transition", "upsert", "update", "create":
			if change.Status != nil {
				target = *change.Status
			}
		default:
			return newError("LEDGER_FORESHADOW_ACTION_INVALID", "foreshadow action is invalid", ErrValidation)
		}
		if !foreshadowTransitionAllowed(existing.Status, target) {
			return newError("LEDGER_INVALID_TRANSITION", fmt.Sprintf("foreshadow transition %s to %s is not allowed", existing.Status, target), ErrConflict)
		}
		if change.Title != "" {
			existing.Title = change.Title
		}
		if change.Description != "" {
			existing.Description = change.Description
		}
		if change.Priority != nil {
			existing.Priority = *change.Priority
		}
		if change.PlantedChapter != nil {
			existing.PlantedChapter = change.PlantedChapter
		}
		if change.DueChapter != nil {
			existing.DueChapter = change.DueChapter
		}
		if change.RevealChapter != nil {
			existing.RevealChapter = change.RevealChapter
		}
		if target == ForeshadowPlanted && existing.PlantedChapter == nil {
			chapter := source.Chapter
			existing.PlantedChapter = &chapter
		}
		if target == ForeshadowRevealed && existing.RevealChapter == nil {
			chapter := source.Chapter
			existing.RevealChapter = &chapter
		}
		existing.Status = target
		if _, err := tx.ExecContext(ctx, `UPDATE foreshadows SET
			title = ?, description = ?, priority = ?, status = ?, planted_chapter = ?,
			due_chapter = ?, reveal_chapter = ?, source_transaction_id = ?,
			updated_chapter = ?, updated_at = ? WHERE id = ?`,
			existing.Title,
			existing.Description,
			existing.Priority,
			existing.Status,
			nullableInt(existing.PlantedChapter),
			nullableInt(existing.DueChapter),
			nullableInt(existing.RevealChapter),
			source.TransactionID,
			source.Chapter,
			now.Format(time.RFC3339Nano),
			existing.ID,
		); err != nil {
			return newError("LEDGER_DATABASE_WRITE_FAILED", "foreshadow could not be updated", err)
		}
	}
	payload, err := json.Marshal(change)
	if err != nil {
		return newError("LEDGER_SERIALIZATION_FAILED", "foreshadow event could not be serialized", err)
	}
	provenance, _ := json.Marshal(source.Provenance)
	if _, err := tx.ExecContext(ctx, `INSERT INTO foreshadow_events(
		event_id, foreshadow_id, source_transaction_id, chapter, action,
		payload_json, provenance_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		deterministicID("foreshadow-event", source.TransactionID, existing.ID, action),
		existing.ID,
		source.TransactionID,
		source.Chapter,
		action,
		string(payload),
		string(provenance),
		now.Format(time.RFC3339Nano),
	); err != nil {
		return newError("LEDGER_DATABASE_WRITE_FAILED", "foreshadow event could not be recorded", err)
	}
	return nil
}

func (s *Store) applySecret(
	ctx context.Context,
	tx *sql.Tx,
	source Source,
	change SecretChange,
	now time.Time,
) error {
	existing, found, err := scanSecret(ctx, tx, change.Key, source.Chapter)
	if err != nil {
		return err
	}
	action := change.Action
	if !found {
		if action == "reveal" || action == "hint" || action == "retire" || action == "transition" || action == "update" {
			return newError("LEDGER_SECRET_NOT_FOUND", "secret does not exist for this transition", ErrNotFound)
		}
		status := SecretHidden
		if change.Status != nil {
			status = *change.Status
		}
		title := change.Title
		if title == "" {
			title = change.Key
		}
		existing = Secret{
			ID:                deterministicID("secret", change.Key),
			Key:               change.Key,
			Title:             title,
			Description:       change.Description,
			Status:            status,
			PublicFromChapter: change.PublicFromChapter,
			Holders:           []string{},
			SourceTransaction: source.TransactionID,
			UpdatedChapter:    source.Chapter,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO secrets(
			id, key, title, description, status, public_from_chapter,
			source_transaction_id, updated_chapter, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			existing.ID,
			existing.Key,
			existing.Title,
			existing.Description,
			existing.Status,
			nullableInt(existing.PublicFromChapter),
			source.TransactionID,
			source.Chapter,
			now.Format(time.RFC3339Nano),
			now.Format(time.RFC3339Nano),
		); err != nil {
			return newError("LEDGER_DATABASE_WRITE_FAILED", "secret could not be created", err)
		}
	} else {
		target := existing.Status
		switch action {
		case "hint":
			target = SecretHinted
		case "reveal":
			target = SecretRevealed
		case "retire":
			target = SecretRetired
		case "transition", "upsert", "update", "create":
			if change.Status != nil {
				target = *change.Status
			}
		default:
			return newError("LEDGER_SECRET_ACTION_INVALID", "secret action is invalid", ErrValidation)
		}
		if !secretTransitionAllowed(existing.Status, target) {
			return newError("LEDGER_INVALID_TRANSITION", fmt.Sprintf("secret transition %s to %s is not allowed", existing.Status, target), ErrConflict)
		}
		if change.Title != "" {
			existing.Title = change.Title
		}
		if change.Description != "" {
			existing.Description = change.Description
		}
		if change.PublicFromChapter != nil {
			existing.PublicFromChapter = change.PublicFromChapter
		}
		if action == "reveal" && existing.PublicFromChapter == nil {
			chapter := source.Chapter
			existing.PublicFromChapter = &chapter
		}
		existing.Status = target
		if _, err := tx.ExecContext(ctx, `UPDATE secrets SET
			title = ?, description = ?, status = ?, public_from_chapter = ?,
			source_transaction_id = ?, updated_chapter = ?, updated_at = ?
			WHERE id = ?`,
			existing.Title,
			existing.Description,
			existing.Status,
			nullableInt(existing.PublicFromChapter),
			source.TransactionID,
			source.Chapter,
			now.Format(time.RFC3339Nano),
			existing.ID,
		); err != nil {
			return newError("LEDGER_DATABASE_WRITE_FAILED", "secret could not be updated", err)
		}
	}
	for _, knowledge := range change.Knowledge {
		if _, err := tx.ExecContext(ctx, `INSERT INTO secret_knowledge(
			knowledge_id, secret_id, holder, known_from_chapter,
			known_until_chapter, source_transaction_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(secret_id, holder, known_from_chapter) DO UPDATE SET
			known_until_chapter = excluded.known_until_chapter,
			source_transaction_id = excluded.source_transaction_id`,
			deterministicID("secret-knowledge", existing.ID, strings.ToLower(knowledge.Holder), fmt.Sprint(knowledge.KnownFromChapter)),
			existing.ID,
			knowledge.Holder,
			knowledge.KnownFromChapter,
			nullableInt(knowledge.KnownUntilChapter),
			source.TransactionID,
			now.Format(time.RFC3339Nano),
		); err != nil {
			return newError("LEDGER_DATABASE_WRITE_FAILED", "secret knowledge boundary could not be recorded", err)
		}
	}
	payload, err := json.Marshal(change)
	if err != nil {
		return newError("LEDGER_SERIALIZATION_FAILED", "secret event could not be serialized", err)
	}
	provenance, _ := json.Marshal(source.Provenance)
	if _, err := tx.ExecContext(ctx, `INSERT INTO secret_events(
		event_id, secret_id, source_transaction_id, chapter, action,
		payload_json, provenance_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		deterministicID("secret-event", source.TransactionID, existing.ID, action),
		existing.ID,
		source.TransactionID,
		source.Chapter,
		action,
		string(payload),
		string(provenance),
		now.Format(time.RFC3339Nano),
	); err != nil {
		return newError("LEDGER_DATABASE_WRITE_FAILED", "secret event could not be recorded", err)
	}
	return nil
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, newError("LEDGER_DATA_INVALID", "narrative ledger timestamp is invalid", err)
	}
	return parsed, nil
}

func nullIntPointer(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	converted := int(value.Int64)
	return &converted
}

func nextOffset(total, offset, limit int) *int {
	next := offset + limit
	if next >= total {
		return nil
	}
	return &next
}

func stableHolders(values []string) []string {
	seen := map[string]string{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			seen[strings.ToLower(trimmed)] = trimmed
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, seen[key])
	}
	return result
}
