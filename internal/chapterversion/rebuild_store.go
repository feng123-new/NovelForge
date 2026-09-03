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
)

func (s *Store) getRebuildByOperation(ctx context.Context, operationID string) (Rebuild, bool, error) {
	var item Rebuild
	var affected string
	var started string
	var completed sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT operation_id,project_id,boundary_chapter,source_version,state,current_step,affected_json,before_digest,after_digest,started_at,completed_at,error_code
		FROM derived_state_rebuilds WHERE operation_id=?`, operationID).Scan(
		&item.OperationID,
		&item.ProjectID,
		&item.BoundaryChapter,
		&item.SourceVersion,
		&item.State,
		&item.CurrentStep,
		&affected,
		&item.BeforeDigest,
		&item.AfterDigest,
		&started,
		&completed,
		&item.ErrorCode,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Rebuild{}, false, nil
	}
	if err != nil {
		return Rebuild{}, false, newError(CodeRebuildFailed, "boundary rebuild could not be read", true, err)
	}
	item.Affected = json.RawMessage(affected)
	if parsed, parseErr := time.Parse(time.RFC3339Nano, started); parseErr == nil {
		item.StartedAt = parsed.UTC()
	}
	if completed.Valid {
		if parsed, parseErr := time.Parse(time.RFC3339Nano, completed.String); parseErr == nil {
			parsed = parsed.UTC()
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
		if err := rows.Scan(
			&item.ID,
			&item.PlanID,
			&item.Chapter,
			&item.Severity,
			&item.AffectedFact,
			&item.PreviousAssumption,
			&item.NewTruth,
			&item.ActionRequired,
			&item.Reason,
			&item.SourceVersion,
			&created,
		); err != nil {
			return nil, 0, newError(CodeStorage, "plan impact row could not be decoded", true, err)
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, newError(CodeStorage, "plan impact iteration failed", true, err)
	}
	return items, total, nil
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
		var id string
		var subjectType string
		var subjectID string
		var predicate string
		var valueHash string
		var authority string
		var fromChapter int
		var toChapter int
		if err := rows.Scan(&id, &subjectType, &subjectID, &predicate, &valueHash, &fromChapter, &toChapter, &authority); err != nil {
			return "", err
		}
		line := fmt.Sprintf("%s|%s|%s|%s|%s|%d|%d|%s", id, subjectType, subjectID, predicate, valueHash, fromChapter, toChapter, authority)
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:]), nil
}
