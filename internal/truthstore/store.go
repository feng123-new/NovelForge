package truthstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
)

const eventColumns = `sequence, id, idempotency_key, request_hash, kind,
	subject_type, subject_id, predicate, value_json, valid_from_chapter,
	valid_to_chapter, known_from_chapter, known_to_chapter, authority,
	confidence, source_type, source_id, source_chapter, source_version,
	source_extractor, source_confirmed_by, source_excerpt,
	supersedes_event_id, created_at, checksum`

type Option func(*Store)

func WithClock(now func() time.Time) Option {
	return func(store *Store) {
		if now != nil {
			store.now = now
		}
	}
}

func WithRandom(reader io.Reader) Option {
	return func(store *Store) {
		if reader != nil {
			store.random = reader
		}
	}
}

type Store struct {
	db      *sql.DB
	now     func() time.Time
	random  io.Reader
	writeMu sync.Mutex
}

func OpenExisting(path string, busyTimeout time.Duration, options ...Option) (*Store, error) {
	db, err := migrate.Open(path, busyTimeout)
	if err != nil {
		return nil, classifyStorageError("truth database could not be opened", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='truth_events'`).Scan(&count); err != nil {
		_ = db.Close()
		return nil, classifyStorageError("truth schema could not be inspected", err)
	}
	if count != 1 {
		_ = db.Close()
		return nil, newError(CodeStorage, "truth schema is not initialized", false, nil)
	}
	store := &Store{db: db, now: time.Now, random: rand.Reader}
	for _, option := range options {
		option(store)
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Append(ctx context.Context, input AppendInput) (AppendResult, error) {
	normalized, err := normalizeAppendInput(input)
	if err != nil {
		return AppendResult{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AppendResult{}, classifyStorageError("truth transaction could not start", err)
	}
	defer tx.Rollback()

	existing, err := eventByIdempotencyTx(ctx, tx, normalized.IdempotencyKey)
	if err == nil {
		if existing.RequestHash != normalized.RequestHash {
			return AppendResult{}, newError(CodeIdempotencyConflict, "Idempotency-Key was already used with a different truth event", false, nil)
		}
		return AppendResult{Event: existing, Replayed: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return AppendResult{}, classifyStorageError("truth idempotency record could not be read", err)
	}

	var target Event
	if normalized.SupersedesEventID != "" {
		target, err = eventByIDTx(ctx, tx, normalized.SupersedesEventID)
		if errors.Is(err, sql.ErrNoRows) {
			return AppendResult{}, newError(CodeNotFound, "superseded truth event was not found", false, err)
		}
		if err != nil {
			return AppendResult{}, classifyStorageError("superseded truth event could not be read", err)
		}
		if target.Kind == EventRetract || !sameKey(target, Event{SubjectType: normalized.SubjectType, SubjectID: normalized.SubjectID, Predicate: normalized.Predicate}) {
			return AppendResult{}, newError(CodeConflict, "supersede target must be an active fact with the same subject and predicate", false, nil)
		}
		newRank, _ := normalized.Authority.rank()
		targetRank, _ := target.Authority.rank()
		if newRank < targetRank {
			return AppendResult{}, newError(CodeAuthority, "lower-authority truth cannot supersede a higher-authority fact", false, nil)
		}
		var targetEffectiveFrom int
		var already sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT effective_from_chapter, superseded_by_event_id FROM truth_facts WHERE event_id = ?`, target.ID).Scan(&targetEffectiveFrom, &already); errors.Is(err, sql.ErrNoRows) {
			return AppendResult{}, newError(CodeConflict, "supersede target is not projected as a fact", false, err)
		} else if err != nil {
			return AppendResult{}, classifyStorageError("supersede target projection could not be read", err)
		}
		replacementFrom, _ := effectiveBounds(normalized.ValidFromChapter, normalized.ValidToChapter, normalized.KnownFromChapter, normalized.KnownToChapter)
		if replacementFrom < targetEffectiveFrom {
			return AppendResult{}, newError(CodeConflict, "supersede cannot begin before the target fact becomes effective", false, nil)
		}
		if already.Valid {
			return AppendResult{}, newError(CodeConflict, "truth event was already superseded", false, nil)
		}
	}

	id, err := randomID(s.random)
	if err != nil {
		return AppendResult{}, newError(CodeStorage, "truth event id could not be generated", false, err)
	}
	createdAt := s.now().UTC()
	event := Event{
		ID: id, IdempotencyKey: normalized.IdempotencyKey, RequestHash: normalized.RequestHash,
		Kind: normalized.Kind, SubjectType: normalized.SubjectType, SubjectID: normalized.SubjectID,
		Predicate: normalized.Predicate, Value: normalized.Value, ValidFromChapter: normalized.ValidFromChapter,
		ValidToChapter: normalized.ValidToChapter, KnownFromChapter: normalized.KnownFromChapter,
		KnownToChapter: normalized.KnownToChapter, Authority: normalized.Authority,
		Confidence: normalized.Confidence, Source: normalized.Source,
		SupersedesEventID: normalized.SupersedesEventID, CreatedAt: createdAt,
	}
	event.Checksum = eventChecksum(event)
	result, err := tx.ExecContext(ctx, `INSERT INTO truth_events(
		id, idempotency_key, request_hash, kind, subject_type, subject_id,
		predicate, value_json, valid_from_chapter, valid_to_chapter,
		known_from_chapter, known_to_chapter, authority, confidence,
		source_type, source_id, source_chapter, source_version,
		source_extractor, source_confirmed_by, source_excerpt,
		supersedes_event_id, created_at, checksum
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.IdempotencyKey, event.RequestHash, event.Kind,
		event.SubjectType, event.SubjectID, event.Predicate, string(event.Value),
		event.ValidFromChapter, nullableInt(event.ValidToChapter), event.KnownFromChapter,
		nullableInt(event.KnownToChapter), event.Authority, event.Confidence,
		event.Source.Type, event.Source.ID, event.Source.Chapter, event.Source.Version,
		event.Source.Extractor, event.Source.ConfirmedBy, event.Source.Excerpt,
		nullableString(event.SupersedesEventID), event.CreatedAt.Format(time.RFC3339Nano), event.Checksum)
	if err != nil {
		_ = tx.Rollback()
		existing, lookupErr := eventByIdempotencyDB(ctx, s.db, normalized.IdempotencyKey)
		if lookupErr == nil {
			if existing.RequestHash != normalized.RequestHash {
				return AppendResult{}, newError(CodeIdempotencyConflict, "Idempotency-Key was already used with a different truth event", false, nil)
			}
			return AppendResult{Event: existing, Replayed: true}, nil
		}
		return AppendResult{}, classifyStorageError("truth event could not be appended", err)
	}
	sequence, err := result.LastInsertId()
	if err != nil {
		return AppendResult{}, classifyStorageError("truth event sequence could not be read", err)
	}
	event.Sequence = sequence

	effectiveFrom, effectiveTo := effectiveBounds(event.ValidFromChapter, event.ValidToChapter, event.KnownFromChapter, event.KnownToChapter)
	if target.ID != "" {
		if err := closeFactTx(ctx, tx, target.ID, event.ID, effectiveFrom-1); err != nil {
			return AppendResult{}, err
		}
	}
	if event.Kind != EventRetract {
		rank, _ := event.Authority.rank()
		if _, err := tx.ExecContext(ctx, `INSERT INTO truth_facts(
			event_id, sequence, subject_type, subject_id, predicate, value_json,
			value_hash, valid_from_chapter, valid_to_chapter, known_from_chapter,
			known_to_chapter, effective_from_chapter, effective_to_chapter,
			authority, authority_rank, confidence, superseded_by_event_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
			event.ID, event.Sequence, event.SubjectType, event.SubjectID, event.Predicate,
			string(event.Value), normalized.ValueHash, event.ValidFromChapter,
			nullableInt(event.ValidToChapter), event.KnownFromChapter,
			nullableInt(event.KnownToChapter), effectiveFrom, nullableInt(effectiveTo),
			event.Authority, rank, event.Confidence); err != nil {
			return AppendResult{}, classifyStorageError("truth projection could not be written", err)
		}
	}
	if err := recomputeConflictsForKeyTx(ctx, tx, event.SubjectType, event.SubjectID, event.Predicate); err != nil {
		return AppendResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE truth_projection_meta SET last_sequence=?, updated_at=? WHERE id=1`, event.Sequence, createdAt.Format(time.RFC3339Nano)); err != nil {
		return AppendResult{}, classifyStorageError("truth projection metadata could not be updated", err)
	}
	if err := tx.Commit(); err != nil {
		return AppendResult{}, classifyStorageError("truth transaction could not commit", err)
	}
	return AppendResult{Event: event}, nil
}

func (s *Store) State(ctx context.Context, query StateQuery) (StatePage, error) {
	if query.Chapter < 0 {
		return StatePage{}, newError(CodeValidation, "chapter must not be negative", false, nil)
	}
	limit, offset, err := normalizePage(query.Limit, query.Offset)
	if err != nil {
		return StatePage{}, err
	}
	query.SubjectType, err = trimFilter(query.SubjectType, true)
	if err != nil {
		return StatePage{}, err
	}
	query.SubjectID, err = trimFilter(query.SubjectID, false)
	if err != nil {
		return StatePage{}, err
	}
	query.Predicate, err = trimFilter(query.Predicate, true)
	if err != nil {
		return StatePage{}, err
	}
	where := `f.effective_from_chapter <= ? AND (f.effective_to_chapter IS NULL OR f.effective_to_chapter >= ?)`
	args := []any{query.Chapter, query.Chapter}
	where, args = addFactFilters(where, args, query.SubjectType, query.SubjectID, query.Predicate)
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM truth_facts f WHERE `+where, args...).Scan(&total); err != nil {
		return StatePage{}, classifyStorageError("truth state count could not be read", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+prefixedEventColumns("e")+`,
		f.effective_from_chapter, f.effective_to_chapter, f.superseded_by_event_id,
		EXISTS(SELECT 1 FROM truth_conflicts c WHERE
			(c.left_event_id=f.event_id OR c.right_event_id=f.event_id) AND c.status='unresolved' AND
			c.from_chapter <= ? AND (c.to_chapter IS NULL OR c.to_chapter >= ?))
		FROM truth_facts f JOIN truth_events e ON e.id=f.event_id
		WHERE `+where+`
		ORDER BY f.authority_rank DESC, f.sequence DESC LIMIT ? OFFSET ?`,
		append([]any{query.Chapter, query.Chapter}, append(args, limit, offset)...)...)
	if err != nil {
		return StatePage{}, classifyStorageError("truth state could not be queried", err)
	}
	defer rows.Close()
	page := StatePage{Facts: []Fact{}, Total: total, Limit: limit, Offset: offset}
	for rows.Next() {
		fact, err := scanFact(rows)
		if err != nil {
			return StatePage{}, classifyStorageError("truth fact could not be decoded", err)
		}
		if fact.Conflicted {
			page.Conflicts++
		}
		page.Facts = append(page.Facts, fact)
	}
	if err := rows.Err(); err != nil {
		return StatePage{}, classifyStorageError("truth state iteration failed", err)
	}
	if offset+len(page.Facts) < total {
		next := offset + len(page.Facts)
		page.NextOffset = &next
	}
	return page, nil
}

func (s *Store) Events(ctx context.Context, query EventQuery) (EventPage, error) {
	if query.AfterSequence < 0 {
		return EventPage{}, newError(CodeValidation, "after_sequence must not be negative", false, nil)
	}
	limit, _, err := normalizePage(query.Limit, 0)
	if err != nil {
		return EventPage{}, err
	}
	query.SubjectType, err = trimFilter(query.SubjectType, true)
	if err != nil {
		return EventPage{}, err
	}
	query.SubjectID, err = trimFilter(query.SubjectID, false)
	if err != nil {
		return EventPage{}, err
	}
	query.Predicate, err = trimFilter(query.Predicate, true)
	if err != nil {
		return EventPage{}, err
	}
	where := "sequence > ?"
	args := []any{query.AfterSequence}
	if query.ThroughChapter != nil {
		if *query.ThroughChapter < 0 {
			return EventPage{}, newError(CodeValidation, "through_chapter must not be negative", false, nil)
		}
		where += " AND valid_from_chapter <= ? AND known_from_chapter <= ?"
		args = append(args, *query.ThroughChapter, *query.ThroughChapter)
	}
	if query.SubjectType != "" {
		where += " AND subject_type=?"
		args = append(args, query.SubjectType)
	}
	if query.SubjectID != "" {
		where += " AND subject_id=?"
		args = append(args, query.SubjectID)
	}
	if query.Predicate != "" {
		where += " AND predicate=?"
		args = append(args, query.Predicate)
	}
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, `SELECT `+eventColumns+` FROM truth_events WHERE `+where+` ORDER BY sequence LIMIT ?`, args...)
	if err != nil {
		return EventPage{}, classifyStorageError("truth events could not be queried", err)
	}
	defer rows.Close()
	page := EventPage{Events: []Event{}, AfterSequence: query.AfterSequence, Limit: limit}
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return EventPage{}, classifyStorageError("truth event could not be decoded", err)
		}
		page.Events = append(page.Events, event)
	}
	if err := rows.Err(); err != nil {
		return EventPage{}, classifyStorageError("truth event iteration failed", err)
	}
	if len(page.Events) > limit {
		page.Events = page.Events[:limit]
		next := page.Events[len(page.Events)-1].Sequence
		page.NextSequence = &next
	}
	return page, nil
}

func (s *Store) Conflicts(ctx context.Context, query ConflictQuery) (ConflictPage, error) {
	limit, offset, err := normalizePage(query.Limit, query.Offset)
	if err != nil {
		return ConflictPage{}, err
	}
	query.SubjectType, err = trimFilter(query.SubjectType, true)
	if err != nil {
		return ConflictPage{}, err
	}
	query.SubjectID, err = trimFilter(query.SubjectID, false)
	if err != nil {
		return ConflictPage{}, err
	}
	query.Predicate, err = trimFilter(query.Predicate, true)
	if err != nil {
		return ConflictPage{}, err
	}
	if query.Status != "" && query.Status != ConflictUnresolved && query.Status != ConflictResolved {
		return ConflictPage{}, newError(CodeValidation, "conflict status must be unresolved or resolved", false, nil)
	}
	where := "1=1"
	args := []any{}
	if query.Chapter != nil {
		if *query.Chapter < 0 {
			return ConflictPage{}, newError(CodeValidation, "chapter must not be negative", false, nil)
		}
		where += ` AND from_chapter <= ? AND (to_chapter IS NULL OR to_chapter >= ?)`
		args = append(args, *query.Chapter, *query.Chapter)
	}
	where, args = addConflictFilters(where, args, query.SubjectType, query.SubjectID, query.Predicate)
	if query.Status != "" {
		where += " AND status=?"
		args = append(args, query.Status)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM truth_conflicts WHERE `+where, args...).Scan(&total); err != nil {
		return ConflictPage{}, classifyStorageError("truth conflict count could not be read", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, subject_type, subject_id, predicate,
		left_event_id, right_event_id, from_chapter, to_chapter, status, reason
		FROM truth_conflicts WHERE `+where+` ORDER BY from_chapter, id LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return ConflictPage{}, classifyStorageError("truth conflicts could not be queried", err)
	}
	defer rows.Close()
	page := ConflictPage{Conflicts: []Conflict{}, Total: total, Limit: limit, Offset: offset}
	for rows.Next() {
		var conflict Conflict
		var to sql.NullInt64
		if err := rows.Scan(&conflict.ID, &conflict.SubjectType, &conflict.SubjectID, &conflict.Predicate,
			&conflict.LeftEventID, &conflict.RightEventID, &conflict.FromChapter, &to, &conflict.Status, &conflict.Reason); err != nil {
			return ConflictPage{}, classifyStorageError("truth conflict could not be decoded", err)
		}
		conflict.ToChapter = intPointer(to)
		page.Conflicts = append(page.Conflicts, conflict)
	}
	if err := rows.Err(); err != nil {
		return ConflictPage{}, classifyStorageError("truth conflict iteration failed", err)
	}
	if offset+len(page.Conflicts) < total {
		next := offset + len(page.Conflicts)
		page.NextOffset = &next
	}
	return page, nil
}

func eventByIdempotencyTx(ctx context.Context, tx *sql.Tx, key string) (Event, error) {
	return scanEvent(tx.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM truth_events WHERE idempotency_key=?`, key))
}

func eventByIdempotencyDB(ctx context.Context, db *sql.DB, key string) (Event, error) {
	return scanEvent(db.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM truth_events WHERE idempotency_key=?`, key))
}

func eventByIDTx(ctx context.Context, tx *sql.Tx, id string) (Event, error) {
	return scanEvent(tx.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM truth_events WHERE id=?`, id))
}

type scanner interface{ Scan(dest ...any) error }

func scanEvent(row scanner) (Event, error) {
	var event Event
	var value, created string
	var validTo, knownTo sql.NullInt64
	var supersedes sql.NullString
	if err := row.Scan(&event.Sequence, &event.ID, &event.IdempotencyKey, &event.RequestHash,
		&event.Kind, &event.SubjectType, &event.SubjectID, &event.Predicate, &value,
		&event.ValidFromChapter, &validTo, &event.KnownFromChapter, &knownTo,
		&event.Authority, &event.Confidence, &event.Source.Type, &event.Source.ID,
		&event.Source.Chapter, &event.Source.Version, &event.Source.Extractor,
		&event.Source.ConfirmedBy, &event.Source.Excerpt, &supersedes, &created,
		&event.Checksum); err != nil {
		return Event{}, err
	}
	event.Value = json.RawMessage(value)
	event.ValidToChapter = intPointer(validTo)
	event.KnownToChapter = intPointer(knownTo)
	if supersedes.Valid {
		event.SupersedesEventID = supersedes.String
	}
	parsed, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Event{}, err
	}
	event.CreatedAt = parsed
	return event, nil
}

func scanFact(row scanner) (Fact, error) {
	var fact Fact
	var value, created string
	var validTo, knownTo, effectiveTo sql.NullInt64
	var supersedes, supersededBy sql.NullString
	var conflicted int
	if err := row.Scan(&fact.Sequence, &fact.ID, &fact.IdempotencyKey, &fact.RequestHash,
		&fact.Kind, &fact.SubjectType, &fact.SubjectID, &fact.Predicate, &value,
		&fact.ValidFromChapter, &validTo, &fact.KnownFromChapter, &knownTo,
		&fact.Authority, &fact.Confidence, &fact.Source.Type, &fact.Source.ID,
		&fact.Source.Chapter, &fact.Source.Version, &fact.Source.Extractor,
		&fact.Source.ConfirmedBy, &fact.Source.Excerpt, &supersedes, &created,
		&fact.Checksum, &fact.EffectiveFromChapter, &effectiveTo, &supersededBy,
		&conflicted); err != nil {
		return Fact{}, err
	}
	fact.Value = json.RawMessage(value)
	fact.ValidToChapter = intPointer(validTo)
	fact.KnownToChapter = intPointer(knownTo)
	fact.EffectiveToChapter = intPointer(effectiveTo)
	if supersedes.Valid {
		fact.SupersedesEventID = supersedes.String
	}
	if supersededBy.Valid {
		fact.SupersededByEventID = supersededBy.String
	}
	parsed, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Fact{}, err
	}
	fact.CreatedAt = parsed
	fact.Conflicted = conflicted != 0
	return fact, nil
}

func closeFactTx(ctx context.Context, tx *sql.Tx, targetID, byID string, end int) error {
	result, err := tx.ExecContext(ctx, `UPDATE truth_facts SET
		effective_to_chapter=CASE WHEN effective_to_chapter IS NULL OR ? < effective_to_chapter THEN ? ELSE effective_to_chapter END,
		superseded_by_event_id=? WHERE event_id=?`, end, end, byID, targetID)
	if err != nil {
		return classifyStorageError("superseded truth projection could not be closed", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return newError(CodeConflict, "supersede target is not an active projected fact", false, err)
	}
	return nil
}

func recomputeConflictsForKeyTx(ctx context.Context, tx *sql.Tx, subjectType, subjectID, predicate string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM truth_conflicts WHERE subject_type=? AND subject_id=? AND predicate=?`, subjectType, subjectID, predicate); err != nil {
		return classifyStorageError("truth conflicts could not be reset", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT event_id, value_hash, effective_from_chapter, effective_to_chapter
		FROM truth_facts WHERE subject_type=? AND subject_id=? AND predicate=? ORDER BY sequence`, subjectType, subjectID, predicate)
	if err != nil {
		return classifyStorageError("truth conflict candidates could not be read", err)
	}
	type candidate struct {
		id, valueHash string
		from          int
		to            *int
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		var to sql.NullInt64
		if err := rows.Scan(&item.id, &item.valueHash, &item.from, &to); err != nil {
			_ = rows.Close()
			return classifyStorageError("truth conflict candidate could not be decoded", err)
		}
		item.to = intPointer(to)
		candidates = append(candidates, item)
	}
	if err := rows.Close(); err != nil {
		return classifyStorageError("truth conflict candidates could not be closed", err)
	}
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			left, right := candidates[i], candidates[j]
			if left.valueHash == right.valueHash {
				continue
			}
			from, to, ok := intervalIntersection(left.from, left.to, right.from, right.to)
			if !ok {
				continue
			}
			conflict := newConflict(subjectType, subjectID, predicate, left.id, right.id, from, to)
			if _, err := tx.ExecContext(ctx, `INSERT INTO truth_conflicts(id, subject_type, subject_id, predicate,
				left_event_id, right_event_id, from_chapter, to_chapter, status, reason) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				conflict.ID, conflict.SubjectType, conflict.SubjectID, conflict.Predicate,
				conflict.LeftEventID, conflict.RightEventID, conflict.FromChapter,
				nullableInt(conflict.ToChapter), conflict.Status, conflict.Reason); err != nil {
				return classifyStorageError("truth conflict could not be projected", err)
			}
		}
	}
	return nil
}

func newConflict(subjectType, subjectID, predicate, leftID, rightID string, from int, to *int) Conflict {
	if rightID < leftID {
		leftID, rightID = rightID, leftID
	}
	toValue := "open"
	if to != nil {
		toValue = fmt.Sprint(*to)
	}
	sum := sha256Text(strings.Join([]string{subjectType, subjectID, predicate, leftID, rightID, fmt.Sprint(from), toValue}, "\x00"))
	status := ConflictUnresolved
	if to != nil {
		status = ConflictResolved
	}
	return Conflict{ID: sum, SubjectType: subjectType, SubjectID: subjectID, Predicate: predicate,
		LeftEventID: leftID, RightEventID: rightID, FromChapter: from, ToChapter: to,
		Status: status, Reason: "overlapping distinct values without explicit supersede"}
}

func intervalIntersection(leftFrom int, leftTo *int, rightFrom int, rightTo *int) (int, *int, bool) {
	from := leftFrom
	if rightFrom > from {
		from = rightFrom
	}
	var to *int
	if leftTo != nil {
		value := *leftTo
		to = &value
	}
	if rightTo != nil && (to == nil || *rightTo < *to) {
		value := *rightTo
		to = &value
	}
	return from, to, to == nil || *to >= from
}

func addFactFilters(where string, args []any, subjectType, subjectID, predicate string) (string, []any) {
	if subjectType != "" {
		where += " AND f.subject_type=?"
		args = append(args, subjectType)
	}
	if subjectID != "" {
		where += " AND f.subject_id=?"
		args = append(args, subjectID)
	}
	if predicate != "" {
		where += " AND f.predicate=?"
		args = append(args, predicate)
	}
	return where, args
}

func addConflictFilters(where string, args []any, subjectType, subjectID, predicate string) (string, []any) {
	if subjectType != "" {
		where += " AND subject_type=?"
		args = append(args, subjectType)
	}
	if subjectID != "" {
		where += " AND subject_id=?"
		args = append(args, subjectID)
	}
	if predicate != "" {
		where += " AND predicate=?"
		args = append(args, predicate)
	}
	return where, args
}

func prefixedEventColumns(prefix string) string {
	parts := strings.Split(eventColumns, ",")
	for index, part := range parts {
		parts[index] = prefix + "." + strings.TrimSpace(part)
	}
	return strings.Join(parts, ", ")
}

func randomID(reader io.Reader) (string, error) {
	buffer := make([]byte, 16)
	if _, err := io.ReadFull(reader, buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func intPointer(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	converted := int(value.Int64)
	return &converted
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func sha256Text(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func classifyStorageError(message string, err error) error {
	lower := strings.ToLower(fmt.Sprint(err))
	if strings.Contains(lower, "locked") || strings.Contains(lower, "busy") || strings.Contains(lower, "timeout") {
		return newError(CodeBusy, message, true, err)
	}
	return newError(CodeStorage, message, false, err)
}
