package chapterversion

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/contextcompiler"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
)

// RebuildDerivedState invalidates future context documents before replaying
// Truth from the Chapter-N boundary. Deleting stale FTS rows before replay means
// concurrent context compilation can be incomplete but can never keep serving
// the superseded future chapter document as authoritative state.
func (c *Coordinator) RebuildDerivedState(ctx context.Context, operationID string, source Version, evaluation Evaluation) (Rebuild, error) {
	if c.Truth == nil {
		return Rebuild{}, newError(CodeRebuildFailed, "Truth Store is required for boundary rebuild", false, nil)
	}
	if existing, ok, err := c.Store.getRebuildByOperation(ctx, operationID); err != nil {
		return Rebuild{}, err
	} else if ok && existing.State == "completed" {
		return existing, nil
	}
	before, err := c.Truth.Verify(ctx)
	if err != nil {
		return Rebuild{}, newError(CodeRebuildFailed, "pre-rebuild Truth digest could not be verified", true, err)
	}
	ledgerAffected, err := c.Store.ledgerAffectedCounts(ctx, source.Chapter)
	if err != nil {
		return Rebuild{}, err
	}
	affected := map[string]any{
		"truth_projection": true,
		"timeline":         true,
		"relations":        true,
		"knowledge":        true,
		"inventory":        true,
		"character_state":  true,
		"context_documents": true,
		"foreshadows":      ledgerAffected["foreshadows"],
		"secrets":          ledgerAffected["secrets"],
		"secret_holders":   ledgerAffected["secret_holders"],
	}
	affectedJSON, _ := json.Marshal(affected)
	now := c.Store.now().UTC()
	_, err = c.Store.db.ExecContext(ctx, `INSERT INTO derived_state_rebuilds(operation_id,project_id,boundary_chapter,source_version,state,current_step,affected_json,before_digest,started_at)
		VALUES(?,?,?,?, 'running','invalidating',?,?,?) ON CONFLICT(operation_id) DO UPDATE SET state='running',current_step='invalidating',error_code=''`,
		operationID, c.Store.projectID, source.Chapter, source.ID, string(affectedJSON), before.ProjectionDigest, now.Format(time.RFC3339Nano))
	if err != nil {
		if isSQLiteBusyOrUnique(err) {
			return Rebuild{}, newError(CodeRebuildInProgress, "another boundary rebuild is in progress", true, err)
		}
		return Rebuild{}, newError(CodeRebuildFailed, "boundary rebuild record could not be started", true, err)
	}
	_ = c.Store.AppendEvent(ctx, source.Chapter, source.ID, "rebuild_started", "derived state invalidated from chapter boundary", mustJSON(map[string]any{"operation_id": operationID, "before_digest": before.ProjectionDigest}))

	// Invalidate FTS/summary-style derived documents at the boundary before Truth
	// replay so stale future prose/facts cannot be selected by Context Compiler.
	if _, err := c.Store.db.ExecContext(ctx, `DELETE FROM context_documents WHERE project_id=? AND source_chapter>=?`, c.Store.projectID, source.Chapter); err != nil {
		return c.failRebuild(ctx, operationID, source, "CONTEXT_INVALIDATION_FAILED", err)
	}
	if err := c.Store.advanceRebuild(ctx, operationID, "truth_rebuild"); err != nil {
		return Rebuild{}, err
	}
	truthResult, err := c.Truth.Rebuild(ctx, source.Chapter)
	if err != nil {
		return c.failRebuild(ctx, operationID, source, "TRUTH_BOUNDARY_REBUILD_FAILED", err)
	}
	if err := c.Store.advanceRebuild(ctx, operationID, "context_rebuild"); err != nil {
		return Rebuild{}, err
	}
	if err := c.rebuildContextDocuments(ctx, source.Chapter); err != nil {
		return c.failRebuild(ctx, operationID, source, "CONTEXT_REBUILD_FAILED", err)
	}
	if err := c.Store.advanceRebuild(ctx, operationID, "plan_impact"); err != nil {
		return Rebuild{}, err
	}
	if err := c.Store.recordPlanImpacts(ctx, source, evaluation); err != nil {
		return c.failRebuild(ctx, operationID, source, "PLAN_IMPACT_FAILED", err)
	}
	after, err := c.Truth.Verify(ctx)
	if err != nil || !after.Valid {
		if err == nil {
			err = errors.New("Truth projection verification failed after boundary rebuild")
		}
		return c.failRebuild(ctx, operationID, source, "REBUILD_VERIFY_FAILED", err)
	}
	// Use the Truth Store's deterministic projection digest as the authoritative
	// before/after boundary evidence. Context rows are rebuilt only from Active
	// Finals and do not alter Chapter N-1 Truth state.
	completed := c.Store.now().UTC()
	_, err = c.Store.db.ExecContext(ctx, `UPDATE derived_state_rebuilds SET state='completed',current_step='completed',after_digest=?,completed_at=?,error_code='' WHERE operation_id=?`,
		after.ProjectionDigest, completed.Format(time.RFC3339Nano), operationID)
	if err != nil {
		return Rebuild{}, newError(CodeRebuildFailed, "boundary rebuild completion could not be recorded", true, err)
	}
	payload := mustJSON(map[string]any{
		"operation_id": operationID, "before_digest": before.ProjectionDigest, "after_digest": after.ProjectionDigest,
		"truth_events_replayed": truthResult.EventsReplayed, "truth_facts_projected": truthResult.FactsProjected,
	})
	if err := c.Store.AppendEvent(ctx, source.Chapter, source.ID, "rebuild_completed", "derived state boundary rebuild completed", payload); err != nil {
		return Rebuild{}, err
	}
	result, _, err := c.Store.getRebuildByOperation(ctx, operationID)
	return result, err
}

func (c *Coordinator) failRebuild(ctx context.Context, operationID string, source Version, code string, cause error) (Rebuild, error) {
	_, _ = c.Store.db.ExecContext(ctx, `UPDATE derived_state_rebuilds SET state='failed',current_step='failed',error_code=?,completed_at=? WHERE operation_id=?`,
		code, c.Store.now().UTC().Format(time.RFC3339Nano), operationID)
	_ = c.Store.AppendEvent(ctx, source.Chapter, source.ID, "rebuild_failed", "derived state boundary rebuild failed", mustJSON(map[string]string{"operation_id": operationID, "error_code": code}))
	return Rebuild{}, newError(CodeRebuildFailed, "derived state boundary rebuild failed", true, cause)
}

func (s *Store) advanceRebuild(ctx context.Context, operationID, step string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE derived_state_rebuilds SET state='running',current_step=? WHERE operation_id=?`, step, operationID)
	if err != nil {
		return newError(CodeRebuildFailed, "boundary rebuild step could not be recorded", true, err)
	}
	return nil
}

func (c *Coordinator) rebuildContextDocuments(ctx context.Context, boundary int) error {
	rows, err := c.Store.db.QueryContext(ctx, `SELECT v.id,v.chapter,v.content,v.created_at FROM chapter_active_finals a
		JOIN chapter_versions v ON v.id=a.version_id WHERE a.project_id=? AND v.chapter>=? ORDER BY v.chapter`, c.Store.projectID, boundary)
	if err != nil {
		return newError(CodeRebuildFailed, "active final context sources could not be read", true, err)
	}
	defer rows.Close()
	fts := contextcompiler.NewFTSStore(c.Store.db)
	for rows.Next() {
		var id, content, created string
		var chapter int
		if err := rows.Scan(&id, &chapter, &content, &created); err != nil {
			return newError(CodeRebuildFailed, "active final context source could not be decoded", true, err)
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, created)
		if err := fts.Upsert(ctx, contextcompiler.Document{
			ID: fmt.Sprintf("chapter-final:%04d", chapter), ProjectID: c.Store.projectID, Kind: "chapter_final",
			Title: fmt.Sprintf("Chapter %d Final", chapter), Content: content, SourceChapter: chapter, SourceVersion: id,
			Priority: 100, CreatedAt: createdAt.UTC(),
		}); err != nil {
			return newError(CodeRebuildFailed, "active final context source could not be rebuilt", true, err)
		}
	}
	return rows.Err()
}

func (s *Store) ledgerAffectedCounts(ctx context.Context, boundary int) (map[string]int, error) {
	result := map[string]int{"foreshadows": 0, "secrets": 0, "secret_holders": 0}
	queries := map[string]string{
		"foreshadows": `SELECT COUNT(DISTINCT foreshadow_id) FROM foreshadow_events WHERE project_id=? AND chapter>=?`,
		"secrets": `SELECT COUNT(DISTINCT secret_id) FROM secret_events WHERE project_id=? AND chapter>=?`,
		"secret_holders": `SELECT COUNT(*) FROM secret_holders h JOIN secrets s ON s.id=h.secret_id WHERE s.project_id=? AND (h.valid_to_chapter IS NULL OR h.valid_to_chapter>=?)`,
	}
	for key, query := range queries {
		if err := s.db.QueryRowContext(ctx, query, s.projectID, boundary).Scan(&result[key]); err != nil {
			return nil, newError(CodeRebuildFailed, "Narrative Ledger boundary impact could not be measured", true, err)
		}
	}
	return result, nil
}

func (s *Store) recordPlanImpacts(ctx context.Context, source Version, evaluation Evaluation) error {
	for index, change := range evaluation.Proposal.AllChanges() {
		subjectType, subjectID := splitSubject(change.Subject)
		affectedFact := subjectType + ":" + subjectID + "/" + change.Predicate
		previous := "unknown"
		for _, conflict := range evaluation.Conflicts {
			if conflict.SubjectType == subjectType && conflict.SubjectID == subjectID && conflict.Predicate == change.Predicate {
				previous = compactJSON(conflict.ExistingValue)
				break
			}
		}
		severity := "warning"
		predicate := strings.ToLower(change.Predicate)
		if strings.Contains(predicate, "alive") || strings.Contains(predicate, "dead") || strings.Contains(predicate, "location") || strings.Contains(predicate, "injur") {
			severity = "blocking"
		}
		id := "impact_" + hashText(source.ID+"\x00"+strconv.Itoa(index)+"\x00"+affectedFact)[:28]
		planID := fmt.Sprintf("chapter:%d", source.Chapter+1)
		reason := fmt.Sprintf("Accepted Human Final at Chapter %d changed a fact consumed by downstream planning", source.Chapter)
		_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO chapter_plan_impacts(id,project_id,source_version,boundary_chapter,plan_id,chapter,severity,affected_fact,previous_assumption,new_truth,action_required,reason,created_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, s.projectID, source.ID, source.Chapter, planID, source.Chapter+1, severity,
			affectedFact, previous, compactJSON(change.Object), "review_or_replan", reason, s.now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return newError(CodeRebuildFailed, "Chapter N+1 plan impact could not be recorded", true, err)
		}
	}
	return nil
}

func compactJSON(raw json.RawMessage) string {
	var buffer strings.Builder
	if err := json.Compact(writerString{&buffer}, raw); err == nil {
		return buffer.String()
	}
	return strings.TrimSpace(string(raw))
}

type writerString struct{ *strings.Builder }
func (w writerString) Write(p []byte) (int, error) { return w.Builder.Write(p) }

func (s *Store) getRebuildByOperation(ctx context.Context, operationID string) (Rebuild, bool, error) {
	var item Rebuild
	var affected, started, completed sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT operation_id,project_id,boundary_chapter,source_version,state,current_step,affected_json,before_digest,after_digest,started_at,completed_at,error_code
		FROM derived_state_rebuilds WHERE operation_id=?`, operationID).Scan(&item.OperationID, &item.ProjectID, &item.BoundaryChapter, &item.SourceVersion,
		&item.State, &item.CurrentStep, &affected, &item.BeforeDigest, &item.AfterDigest, &started, &completed, &item.ErrorCode)
	if errors.Is(err, sql.ErrNoRows) {
		return Rebuild{}, false, nil
	}
	if err != nil {
		return Rebuild{}, false, newError(CodeRebuildFailed, "boundary rebuild could not be read", true, err)
	}
	item.Affected = json.RawMessage(affected.String)
	if parsed, err := time.Parse(time.RFC3339Nano, started.String); err == nil {
		item.StartedAt = parsed.UTC()
	}
	if completed.Valid {
		if parsed, err := time.Parse(time.RFC3339Nano, completed.String); err == nil {
			item.CompletedAt = &parsed
		}
	}
	return item, true, nil
}

func (s *Store) LatestRebuild(ctx context.Context, boundary int) (Rebuild, bool, error) {
	var operationID string
	err := s.db.QueryRowContext(ctx, `SELECT operation_id FROM derived_state_rebuilds WHERE project_id=? AND boundary_chapter<=? ORDER BY started_at DESC,operation_id DESC LIMIT 1`, s.projectID, boundary).Scan(&operationID)
	if errors.Is(err, sql.ErrNoRows) {
		return Rebuild{}, false, nil
	}
	if err != nil {
		return Rebuild{}, false, newError(CodeRebuildFailed, "latest boundary rebuild could not be read", true, err)
	}
	return s.getRebuildByOperation(ctx, operationID)
}

func (s *Store) ListPlanImpacts(ctx context.Context, chapter, limit, offset int) ([]PlanImpact, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chapter_plan_impacts WHERE project_id=? AND chapter>=?`, s.projectID, chapter).Scan(&total); err != nil {
		return nil, 0, newError(CodeStorage, "plan impact count could not be read", true, err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,plan_id,chapter,severity,affected_fact,previous_assumption,new_truth,action_required,reason,source_version,created_at
		FROM chapter_plan_impacts WHERE project_id=? AND chapter>=? ORDER BY chapter,severity DESC,id LIMIT ? OFFSET ?`, s.projectID, chapter, limit, offset)
	if err != nil {
		return nil, 0, newError(CodeStorage, "plan impacts could not be read", true, err)
	}
	defer rows.Close()
	items := []PlanImpact{}
	for rows.Next() {
		var item PlanImpact
		var created string
		if err := rows.Scan(&item.ID, &item.PlanID, &item.Chapter, &item.Severity, &item.AffectedFact, &item.PreviousAssumption, &item.NewTruth, &item.ActionRequired, &item.Reason, &item.SourceVersion, &created); err != nil {
			return nil, 0, newError(CodeStorage, "plan impact row could not be decoded", true, err)
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *Store) ProjectionBoundaryDigest(ctx context.Context, throughChapter int) (string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT event_id,subject_type,subject_id,predicate,value_hash,effective_from_chapter,COALESCE(effective_to_chapter,-1),authority
		FROM truth_facts WHERE effective_from_chapter<=? ORDER BY event_id`, throughChapter)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	lines := []string{}
	for rows.Next() {
		var id, st, sid, pred, valueHash, authority string
		var from, to int
		if err := rows.Scan(&id, &st, &sid, &pred, &valueHash, &from, &to, &authority); err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("%s|%s|%s|%s|%s|%d|%d|%s", id, st, sid, pred, valueHash, from, to, authority))
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:]), rows.Err()
}

func isSQLiteBusyOrUnique(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "busy") || strings.Contains(message, "locked") || strings.Contains(message, "unique") || strings.Contains(message, "constraint")
}

var _ qualitygate.FactProposal
