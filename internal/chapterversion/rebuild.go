package chapterversion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/contextcompiler"
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
	affected := map[string]any{}
	affected["truth_projection"] = true
	affected["timeline"] = true
	affected["relations"] = true
	affected["knowledge"] = true
	affected["inventory"] = true
	affected["character_state"] = true
	affected["context_documents"] = true
	affected["foreshadows"] = ledgerAffected["foreshadows"]
	affected["secrets"] = ledgerAffected["secrets"]
	affected["secret_holders"] = ledgerAffected["secret_holders"]
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
	payload := map[string]any{}
	payload["operation_id"] = operationID
	payload["before_digest"] = before.ProjectionDigest
	_ = c.Store.AppendEvent(ctx, source.Chapter, source.ID, "rebuild_started", "derived state invalidated from chapter boundary", mustJSON(payload))

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
	completed := c.Store.now().UTC()
	_, err = c.Store.db.ExecContext(ctx, `UPDATE derived_state_rebuilds SET state='completed',current_step='completed',after_digest=?,completed_at=?,error_code='' WHERE operation_id=?`,
		after.ProjectionDigest, completed.Format(time.RFC3339Nano), operationID)
	if err != nil {
		return Rebuild{}, newError(CodeRebuildFailed, "boundary rebuild completion could not be recorded", true, err)
	}
	completePayload := map[string]any{}
	completePayload["operation_id"] = operationID
	completePayload["before_digest"] = before.ProjectionDigest
	completePayload["after_digest"] = after.ProjectionDigest
	completePayload["truth_events_replayed"] = truthResult.EventsReplayed
	completePayload["truth_facts_projected"] = truthResult.FactsProjected
	if err := c.Store.AppendEvent(ctx, source.Chapter, source.ID, "rebuild_completed", "derived state boundary rebuild completed", mustJSON(completePayload)); err != nil {
		return Rebuild{}, err
	}
	result, _, err := c.Store.getRebuildByOperation(ctx, operationID)
	return result, err
}

func (c *Coordinator) failRebuild(ctx context.Context, operationID string, source Version, code string, cause error) (Rebuild, error) {
	_, _ = c.Store.db.ExecContext(ctx, `UPDATE derived_state_rebuilds SET state='failed',current_step='failed',error_code=?,completed_at=? WHERE operation_id=?`,
		code, c.Store.now().UTC().Format(time.RFC3339Nano), operationID)
	payload := map[string]string{}
	payload["operation_id"] = operationID
	payload["error_code"] = code
	_ = c.Store.AppendEvent(ctx, source.Chapter, source.ID, "rebuild_failed", "derived state boundary rebuild failed", mustJSON(payload))
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

	// The project SQLite handle deliberately allows one open connection. Fully
	// materialize and close the read cursor before FTS writes so rebuild cannot
	// deadlock waiting for a second connection from the same sql.DB.
	documents := []contextcompiler.Document{}
	for rows.Next() {
		var id string
		var content string
		var created string
		var chapter int
		if err := rows.Scan(&id, &chapter, &content, &created); err != nil {
			_ = rows.Close()
			return newError(CodeRebuildFailed, "active final context source could not be decoded", true, err)
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, created)
		document := contextcompiler.Document{}
		document.ID = fmt.Sprintf("chapter-final:%04d", chapter)
		document.ProjectID = c.Store.projectID
		document.Kind = "chapter_final"
		document.Title = fmt.Sprintf("Chapter %d Final", chapter)
		document.Content = content
		document.SourceChapter = chapter
		document.SourceVersion = id
		document.Priority = 100
		document.CreatedAt = createdAt.UTC()
		documents = append(documents, document)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return newError(CodeRebuildFailed, "active final context source iteration failed", true, err)
	}
	if err := rows.Close(); err != nil {
		return newError(CodeRebuildFailed, "active final context sources could not be closed", true, err)
	}

	fts := contextcompiler.NewFTSStore(c.Store.db)
	for _, document := range documents {
		if err := fts.Upsert(ctx, document); err != nil {
			return newError(CodeRebuildFailed, "active final context source could not be rebuilt", true, err)
		}
	}
	return nil
}

func (s *Store) ledgerAffectedCounts(ctx context.Context, boundary int) (map[string]int, error) {
	result := map[string]int{}
	var foreshadows int
	var secrets int
	var holders int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT foreshadow_id) FROM foreshadow_events WHERE project_id=? AND chapter>=?`, s.projectID, boundary).Scan(&foreshadows); err != nil {
		return nil, newError(CodeRebuildFailed, "Narrative Ledger foreshadow impact could not be measured", true, err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT secret_id) FROM secret_events WHERE project_id=? AND chapter>=?`, s.projectID, boundary).Scan(&secrets); err != nil {
		return nil, newError(CodeRebuildFailed, "Narrative Ledger secret impact could not be measured", true, err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM secret_holders h JOIN secrets s ON s.id=h.secret_id WHERE s.project_id=? AND (h.valid_to_chapter IS NULL OR h.valid_to_chapter>=?)`, s.projectID, boundary).Scan(&holders); err != nil {
		return nil, newError(CodeRebuildFailed, "Narrative Ledger holder impact could not be measured", true, err)
	}
	result["foreshadows"] = foreshadows
	result["secrets"] = secrets
	result["secret_holders"] = holders
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
		id := "impact_" + hashText(source.ID + "\x00" + strconv.Itoa(index) + "\x00" + affectedFact)[:28]
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
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, raw); err == nil {
		return buffer.String()
	}
	return strings.TrimSpace(string(raw))
}

func isSQLiteBusyOrUnique(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "busy") || strings.Contains(message, "locked") || strings.Contains(message, "unique") || strings.Contains(message, "constraint")
}
