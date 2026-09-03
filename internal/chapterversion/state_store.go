package chapterversion

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) decorate(ctx context.Context, version *Version) error {
	var accepted, rejected int
	var reason string
	if err := s.db.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM chapter_version_events WHERE version_id=? AND event_type='accept'),
		EXISTS(SELECT 1 FROM chapter_version_events WHERE version_id=? AND event_type='reject'),
		COALESCE((SELECT reason FROM chapter_version_events WHERE version_id=? AND event_type='reject' ORDER BY sequence DESC LIMIT 1),'')`,
		version.ID, version.ID, version.ID).Scan(&accepted, &rejected, &reason); err != nil {
		return newError(CodeStorage, "chapter version state could not be projected", true, err)
	}
	version.Accepted = accepted == 1
	version.Rejected = rejected == 1
	version.RejectionReason = reason
	if version.Rejected {
		version.Status = "rejected"
	} else if version.Accepted && version.Type != TypeFinal {
		version.Status = "accepted"
	}

	var activeID, authority string
	err := s.db.QueryRowContext(ctx, `SELECT version_id,authority FROM chapter_active_finals WHERE project_id=? AND chapter=?`, s.projectID, version.Chapter).Scan(&activeID, &authority)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return newError(CodeStorage, "active final state could not be projected", true, err)
	}
	if activeID == version.ID {
		version.ActiveFinal = true
		version.Authority = authority
		version.Status = "final"
	}
	return nil
}

func appendEventTx(ctx context.Context, tx *sql.Tx, projectID string, chapter int, versionID, eventType, reason string, payload json.RawMessage, now time.Time) error {
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	id, err := newOpaqueID("cve_")
	if err != nil {
		return newError(CodeStorage, "chapter version event id could not be generated", true, err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO chapter_version_events(id,project_id,chapter,version_id,event_type,reason,payload_json,created_at)
		VALUES(?,?,?,?,?,?,?,?)`, id, projectID, chapter, versionID, eventType, strings.TrimSpace(reason), string(payload), now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return newError(CodeStorage, "chapter version event could not be written", true, err)
	}
	return nil
}

func (s *Store) AppendEvent(ctx context.Context, chapter int, versionID, eventType, reason string, payload json.RawMessage) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return newError(CodeStorage, "chapter version event transaction could not start", true, err)
	}
	defer tx.Rollback()
	if err := appendEventTx(ctx, tx, s.projectID, chapter, versionID, eventType, reason, payload, s.now().UTC()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return newError(CodeStorage, "chapter version event could not commit", true, err)
	}
	return nil
}

func (s *Store) BeginOperation(ctx context.Context, key, operationName string, chapter int, versionID, requestHash string) (operation, bool, error) {
	key = strings.TrimSpace(key)
	operationName = strings.TrimSpace(operationName)
	requestHash = strings.TrimSpace(requestHash)
	if key == "" || operationName == "" || chapter < 1 || len(requestHash) != 64 {
		return operation{}, false, newError(CodeValidation, "idempotent operation identity is invalid", false, nil)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return operation{}, false, newError(CodeStorage, "chapter operation transaction could not start", true, err)
	}
	defer tx.Rollback()

	var existing operation
	var result string
	err = tx.QueryRowContext(ctx, `SELECT idempotency_key,operation,project_id,chapter,version_id,request_hash,status,result_json
		FROM chapter_revision_operations WHERE idempotency_key=?`, key).Scan(
		&existing.Key,
		&existing.Operation,
		&existing.ProjectID,
		&existing.Chapter,
		&existing.VersionID,
		&existing.Hash,
		&existing.Status,
		&result,
	)
	if err == nil {
		existing.Result = json.RawMessage(result)
		if existing.Operation != operationName || existing.ProjectID != s.projectID || existing.Chapter != chapter || existing.VersionID != versionID || existing.Hash != requestHash {
			return operation{}, false, newError(CodeIdempotencyConflict, "Idempotency-Key was already used for a different request", false, nil)
		}
		if err := tx.Commit(); err != nil {
			return operation{}, false, newError(CodeStorage, "chapter operation replay could not commit", true, err)
		}
		return existing, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return operation{}, false, newError(CodeStorage, "chapter operation could not be read", true, err)
	}

	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT INTO chapter_revision_operations(idempotency_key,operation,project_id,chapter,version_id,request_hash,status,result_json,created_at)
		VALUES(?,?,?,?,?,?, 'pending','{}',?)`, key, operationName, s.projectID, chapter, versionID, requestHash, now)
	if err != nil {
		return operation{}, false, newError(CodeConflict, "chapter operation could not be reserved", true, err)
	}
	if err := tx.Commit(); err != nil {
		return operation{}, false, newError(CodeStorage, "chapter operation could not commit", true, err)
	}
	return operation{
		Key:       key,
		Operation: operationName,
		ProjectID: s.projectID,
		Chapter:   chapter,
		VersionID: versionID,
		Hash:      requestHash,
		Status:    "pending",
		Result:    json.RawMessage(`{}`),
	}, false, nil
}

func (s *Store) CompleteOperation(ctx context.Context, key string, result any) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return newError(CodeStorage, "chapter operation result could not be encoded", false, err)
	}
	_, err = s.db.ExecContext(ctx, `UPDATE chapter_revision_operations SET status='completed',result_json=?,completed_at=? WHERE idempotency_key=? AND status!='completed'`,
		string(encoded), s.now().UTC().Format(time.RFC3339Nano), strings.TrimSpace(key))
	if err != nil {
		return newError(CodeStorage, "chapter operation could not be completed", true, err)
	}
	return nil
}

func (s *Store) FailOperation(ctx context.Context, key, code string) error {
	payload, _ := json.Marshal(map[string]string{"error_code": code})
	_, err := s.db.ExecContext(ctx, `UPDATE chapter_revision_operations SET status='failed',result_json=?,completed_at=? WHERE idempotency_key=? AND status!='completed'`,
		string(payload), s.now().UTC().Format(time.RFC3339Nano), strings.TrimSpace(key))
	if err != nil {
		return newError(CodeStorage, "chapter operation failure could not be recorded", true, err)
	}
	return nil
}

func (s *Store) Accept(ctx context.Context, chapter int, versionID, reason string) (Version, error) {
	version, err := s.Get(ctx, chapter, versionID, true)
	if err != nil {
		return Version{}, err
	}
	if version.Rejected {
		return Version{}, newError(CodeRejected, "rejected versions cannot be accepted", false, nil)
	}
	if continuityBlocks(version.Continuity) {
		return Version{}, newError(CodeContinuityBlocked, "continuity FAIL blocks acceptance", false, nil)
	}
	if version.Accepted {
		return version, nil
	}
	if err := s.AppendEvent(ctx, chapter, versionID, "accept", reason, json.RawMessage(`{}`)); err != nil {
		return Version{}, err
	}
	return s.Get(ctx, chapter, versionID, true)
}

func (s *Store) Reject(ctx context.Context, chapter int, versionID, reason string) (Version, error) {
	if strings.TrimSpace(reason) == "" {
		return Version{}, newError(CodeValidation, "rejection reason is required", false, nil)
	}
	version, err := s.Get(ctx, chapter, versionID, true)
	if err != nil {
		return Version{}, err
	}
	if version.ActiveFinal {
		return Version{}, newError(CodeFinalizeNotAllowed, "the active final cannot be rejected in place", false, nil)
	}
	if version.Rejected {
		return version, nil
	}
	if err := s.AppendEvent(ctx, chapter, versionID, "reject", reason, json.RawMessage(`{}`)); err != nil {
		return Version{}, err
	}
	return s.Get(ctx, chapter, versionID, true)
}

func (s *Store) SwitchActiveFinal(ctx context.Context, chapter int, finalVersionID, authority string) error {
	if authority != AuthorityGeneratedFinal && authority != AuthorityHumanFinal {
		return newError(CodeValidation, "active final authority is invalid", false, nil)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return newError(CodeStorage, "active final transaction could not start", true, err)
	}
	defer tx.Rollback()

	var typ string
	if err := tx.QueryRowContext(ctx, `SELECT version_type FROM chapter_versions WHERE id=? AND project_id=? AND chapter=?`, finalVersionID, s.projectID, chapter).Scan(&typ); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return newError(CodeNotFound, "final chapter version was not found", false, ErrNotFound)
		}
		return newError(CodeStorage, "final chapter version could not be validated", true, err)
	}
	if VersionType(typ) != TypeFinal {
		return newError(CodeFinalizeNotAllowed, "only final versions can become active", false, nil)
	}

	var currentID, currentAuthority string
	currentErr := tx.QueryRowContext(ctx, `SELECT version_id,authority FROM chapter_active_finals WHERE project_id=? AND chapter=?`, s.projectID, chapter).Scan(&currentID, &currentAuthority)
	if currentErr != nil && !errors.Is(currentErr, sql.ErrNoRows) {
		return newError(CodeStorage, "current active final could not be read", true, currentErr)
	}
	if currentID == finalVersionID && currentAuthority == authority {
		return tx.Commit()
	}
	if currentAuthority == AuthorityHumanFinal && authority != AuthorityHumanFinal {
		return newError(CodeActiveFinalConflict, "Accepted Human Final authority cannot be downgraded by a generated final", false, nil)
	}

	now := s.now().UTC()
	_, err = tx.ExecContext(ctx, `INSERT INTO chapter_active_finals(project_id,chapter,version_id,authority,activated_at)
		VALUES(?,?,?,?,?) ON CONFLICT(project_id,chapter) DO UPDATE SET version_id=excluded.version_id,authority=excluded.authority,activated_at=excluded.activated_at`,
		s.projectID, chapter, finalVersionID, authority, now.Format(time.RFC3339Nano))
	if err != nil {
		return newError(CodeActiveFinalConflict, "active final could not be switched", true, err)
	}
	payload, _ := json.Marshal(map[string]string{"authority": authority})
	if err := appendEventTx(ctx, tx, s.projectID, chapter, finalVersionID, "active_final_switched", "active final switched", payload, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return newError(CodeStorage, "active final switch could not commit", true, err)
	}
	return nil
}

func (s *Store) CountUnresolvedTruthConflicts(ctx context.Context, chapter int) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM truth_conflicts WHERE status='unresolved' AND from_chapter<=? AND (to_chapter IS NULL OR to_chapter>=?)`, chapter, chapter).Scan(&count); err != nil {
		return 0, newError(CodeStorage, "truth conflicts could not be checked", true, err)
	}
	return count, nil
}

func continuityBlocks(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "{}" {
		return false
	}
	var state struct {
		Status   string `json:"status"`
		Blocking bool   `json:"blocking"`
	}
	if json.Unmarshal(raw, &state) != nil {
		return true
	}
	return strings.EqualFold(state.Status, "FAIL") || state.Blocking
}

func (s *Store) DebugCounts(ctx context.Context, chapter int) (map[string]int, error) {
	queries := map[string]string{
		"versions":     `SELECT COUNT(*) FROM chapter_versions WHERE project_id=? AND chapter=?`,
		"active_final": `SELECT COUNT(*) FROM chapter_active_finals WHERE project_id=? AND chapter=?`,
		"events":       `SELECT COUNT(*) FROM chapter_version_events WHERE project_id=? AND chapter=?`,
	}
	out := map[string]int{}
	for key, query := range queries {
		var count int
		if err := s.db.QueryRowContext(ctx, query, s.projectID, chapter).Scan(&count); err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		out[key] = count
	}
	return out, nil
}
