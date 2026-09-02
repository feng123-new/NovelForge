package narrativeledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/truthstore"
)

type scanner interface{ Scan(...any) error }

func (s *Store) GetForeshadow(ctx context.Context, projectID, id string, currentChapter int) (Foreshadow, error) {
	return getForeshadow(ctx, s.db, strings.TrimSpace(projectID), strings.TrimSpace(id), currentChapter)
}

func getForeshadow(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, projectID, id string, currentChapter int) (Foreshadow, error) {
	row := q.QueryRowContext(ctx, `SELECT id,project_id,title,description,importance,planted_chapter,expected_payoff_min,expected_payoff_max,actual_payoff,status,related_entities_json,related_arcs_json,last_progress_chapter,urgency,source_version,authority,created_at,updated_at FROM foreshadows WHERE id=? AND project_id=?`, id, projectID)
	return scanForeshadow(row, currentChapter)
}

func scanForeshadow(row scanner, currentChapter int) (Foreshadow, error) {
	var item Foreshadow
	var actual sql.NullInt64
	var entitiesJSON, arcsJSON, created, updated string
	if err := row.Scan(&item.ID, &item.ProjectID, &item.Title, &item.Description, &item.Importance, &item.PlantedChapter, &item.ExpectedPayoffMin, &item.ExpectedPayoffMax, &actual, &item.Status, &entitiesJSON, &arcsJSON, &item.LastProgressChapter, &item.Urgency, &item.SourceVersion, &item.Authority, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Foreshadow{}, ErrNotFound
		}
		return Foreshadow{}, err
	}
	item.ActualPayoff = intPointer(actual)
	if err := json.Unmarshal([]byte(entitiesJSON), &item.RelatedEntities); err != nil {
		return Foreshadow{}, err
	}
	if err := json.Unmarshal([]byte(arcsJSON), &item.RelatedArcs); err != nil {
		return Foreshadow{}, err
	}
	var err error
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Foreshadow{}, err
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return Foreshadow{}, err
	}
	item.Overdue = currentChapter > item.ExpectedPayoffMax && item.Status != StatusResolved && item.Status != StatusAbandoned
	if item.Overdue {
		item.OverdueByChapters = currentChapter - item.ExpectedPayoffMax
	}
	return item, nil
}

func (s *Store) ListForeshadows(ctx context.Context, projectID string, query ForeshadowQuery) (ForeshadowPage, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" || query.CurrentChapter < 0 {
		return ForeshadowPage{}, fmt.Errorf("%w: project and non-negative chapter are required", ErrValidation)
	}
	limit, offset, err := normalizePage(query.Limit, query.Offset)
	if err != nil {
		return ForeshadowPage{}, err
	}
	where := []string{"f.project_id=?"}
	args := []any{projectID}
	if query.Status != "" {
		if !validStatus(query.Status) {
			return ForeshadowPage{}, fmt.Errorf("%w: invalid status", ErrValidation)
		}
		where = append(where, "f.status=?")
		args = append(args, query.Status)
	}
	if query.Importance != "" {
		if !validImportance(query.Importance) {
			return ForeshadowPage{}, fmt.Errorf("%w: invalid importance", ErrValidation)
		}
		where = append(where, "f.importance=?")
		args = append(args, query.Importance)
	}
	if query.Urgency != "" {
		if !validUrgency(query.Urgency) {
			return ForeshadowPage{}, fmt.Errorf("%w: invalid urgency", ErrValidation)
		}
		where = append(where, "f.urgency=?")
		args = append(args, query.Urgency)
	}
	if query.Overdue != nil {
		if *query.Overdue {
			where = append(where, "f.expected_payoff_max < ? AND f.status NOT IN ('resolved','abandoned')")
			args = append(args, query.CurrentChapter)
		} else {
			where = append(where, "NOT (f.expected_payoff_max < ? AND f.status NOT IN ('resolved','abandoned'))")
			args = append(args, query.CurrentChapter)
		}
	}
	if value := strings.TrimSpace(query.Arc); value != "" {
		where = append(where, "EXISTS(SELECT 1 FROM foreshadow_arcs fa WHERE fa.foreshadow_id=f.id AND fa.arc_id=?)")
		args = append(args, value)
	}
	if value := strings.TrimSpace(query.Entity); value != "" {
		where = append(where, "EXISTS(SELECT 1 FROM foreshadow_entities fe WHERE fe.foreshadow_id=f.id AND fe.entity_id=?)")
		args = append(args, value)
	}
	if value := strings.TrimSpace(query.Query); value != "" {
		if len([]rune(value)) > 200 {
			return ForeshadowPage{}, fmt.Errorf("%w: query too long", ErrValidation)
		}
		where = append(where, "(LOWER(f.title) LIKE ? OR LOWER(f.description) LIKE ?)")
		needle := "%" + strings.ToLower(value) + "%"
		args = append(args, needle, needle)
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM foreshadows f WHERE "+clause, args...).Scan(&total); err != nil {
		return ForeshadowPage{}, err
	}
	statement := `SELECT f.id,f.project_id,f.title,f.description,f.importance,f.planted_chapter,f.expected_payoff_min,f.expected_payoff_max,f.actual_payoff,f.status,f.related_entities_json,f.related_arcs_json,f.last_progress_chapter,f.urgency,f.source_version,f.authority,f.created_at,f.updated_at FROM foreshadows f WHERE ` + clause + ` ORDER BY CASE WHEN f.expected_payoff_max < ? AND f.status NOT IN ('resolved','abandoned') THEN 0 ELSE 1 END ASC, CASE f.importance WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END ASC, CASE f.urgency WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END ASC, f.expected_payoff_max ASC, f.id ASC LIMIT ? OFFSET ?`
	readArgs := append(append([]any{}, args...), query.CurrentChapter, limit, offset)
	rows, err := s.db.QueryContext(ctx, statement, readArgs...)
	if err != nil {
		return ForeshadowPage{}, err
	}
	defer rows.Close()
	page := ForeshadowPage{Foreshadows: []Foreshadow{}, Total: total, Limit: limit, Offset: offset}
	for rows.Next() {
		item, err := scanForeshadow(rows, query.CurrentChapter)
		if err != nil {
			return ForeshadowPage{}, err
		}
		page.Foreshadows = append(page.Foreshadows, item)
	}
	if err := rows.Err(); err != nil {
		return ForeshadowPage{}, err
	}
	if offset+len(page.Foreshadows) < total {
		next := offset + len(page.Foreshadows)
		page.NextOffset = &next
	}
	return page, nil
}

func (s *Store) ForeshadowQueryPlan(ctx context.Context, projectID string, currentChapter int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `EXPLAIN QUERY PLAN SELECT id FROM foreshadows WHERE project_id=? AND status NOT IN ('resolved','abandoned') AND expected_payoff_max < ? ORDER BY expected_payoff_max,id LIMIT 100`, projectID, currentChapter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			return nil, err
		}
		out = append(out, detail)
	}
	return out, rows.Err()
}

func (s *Store) GetSecret(ctx context.Context, projectID, id string, currentChapter int, includeTruth bool) (Secret, error) {
	return getSecret(ctx, s.db, strings.TrimSpace(projectID), strings.TrimSpace(id), currentChapter, includeTruth)
}

func getSecret(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, projectID, id string, currentChapter int, includeTruth bool) (Secret, error) {
	item, err := scanSecret(q.QueryRowContext(ctx, `SELECT id,project_id,description,truth,created_chapter,revealed_chapter,public_status,related_foreshadow,source_version,authority,created_at,updated_at FROM secrets WHERE id=? AND project_id=? AND created_chapter<=?`, id, projectID, currentChapter), currentChapter, includeTruth)
	if err != nil {
		return Secret{}, err
	}
	return item, nil
}

func scanSecret(row scanner, currentChapter int, includeTruth bool) (Secret, error) {
	var item Secret
	var revealed sql.NullInt64
	var created, updated string
	if err := row.Scan(&item.ID, &item.ProjectID, &item.Description, &item.Truth, &item.CreatedChapter, &revealed, &item.PublicStatus, &item.RelatedForeshadow, &item.SourceVersion, &item.Authority, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Secret{}, ErrNotFound
		}
		return Secret{}, err
	}
	item.RevealedChapter = intPointer(revealed)
	item.PublicAtChapter = item.RevealedChapter != nil && *item.RevealedChapter <= currentChapter && item.PublicStatus == PublicPublic
	if !includeTruth {
		item.Truth = ""
	}
	var err error
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Secret{}, err
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return Secret{}, err
	}
	return item, nil
}

func (s *Store) ListSecrets(ctx context.Context, projectID string, query SecretQuery) (SecretPage, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" || query.CurrentChapter < 0 {
		return SecretPage{}, fmt.Errorf("%w: project and non-negative chapter are required", ErrValidation)
	}
	limit, offset, err := normalizePage(query.Limit, query.Offset)
	if err != nil {
		return SecretPage{}, err
	}
	where := []string{"s.project_id=?", "s.created_chapter<=?"}
	args := []any{projectID, query.CurrentChapter}
	if query.PublicStatus != "" {
		if query.PublicStatus != PublicPrivate && query.PublicStatus != PublicPublic {
			return SecretPage{}, fmt.Errorf("%w: invalid public status", ErrValidation)
		}
		where = append(where, "s.public_status=?")
		args = append(args, query.PublicStatus)
	}
	if holder := strings.TrimSpace(query.Holder); holder != "" {
		where = append(where, "EXISTS(SELECT 1 FROM secret_holders h WHERE h.secret_id=s.id AND h.entity_id=? AND h.valid_from_chapter<=? AND (h.valid_to_chapter IS NULL OR h.valid_to_chapter>=?))")
		args = append(args, holder, query.CurrentChapter, query.CurrentChapter)
	}
	if value := strings.TrimSpace(query.Query); value != "" {
		if len([]rune(value)) > 200 {
			return SecretPage{}, fmt.Errorf("%w: query too long", ErrValidation)
		}
		where = append(where, "LOWER(s.description) LIKE ?")
		args = append(args, "%"+strings.ToLower(value)+"%")
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM secrets s WHERE "+clause, args...).Scan(&total); err != nil {
		return SecretPage{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT s.id,s.project_id,s.description,s.truth,s.created_chapter,s.revealed_chapter,s.public_status,s.related_foreshadow,s.source_version,s.authority,s.created_at,s.updated_at FROM secrets s WHERE `+clause+` ORDER BY s.created_chapter ASC,s.id ASC LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return SecretPage{}, err
	}
	defer rows.Close()
	page := SecretPage{Secrets: []Secret{}, Total: total, Limit: limit, Offset: offset}
	for rows.Next() {
		item, err := scanSecret(rows, query.CurrentChapter, query.IncludeTruth)
		if err != nil {
			return SecretPage{}, err
		}
		page.Secrets = append(page.Secrets, item)
	}
	if err := rows.Err(); err != nil {
		return SecretPage{}, err
	}
	if err := rows.Close(); err != nil {
		return SecretPage{}, err
	}
	for index := range page.Secrets {
		holders, err := s.SecretHolders(ctx, page.Secrets[index].ID, query.CurrentChapter)
		if err != nil {
			return SecretPage{}, err
		}
		page.Secrets[index].Holders = holders
	}
	if offset+len(page.Secrets) < total {
		next := offset + len(page.Secrets)
		page.NextOffset = &next
	}
	return page, nil
}

func (s *Store) SecretHolders(ctx context.Context, secretID string, chapter int) ([]KnowledgeHolder, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT secret_id,entity_id,valid_from_chapter,valid_to_chapter,source_version,authority,provenance_json FROM secret_holders WHERE secret_id=? AND valid_from_chapter<=? AND (valid_to_chapter IS NULL OR valid_to_chapter>=?) ORDER BY entity_id ASC,valid_from_chapter ASC`, secretID, chapter, chapter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []KnowledgeHolder{}
	for rows.Next() {
		var holder KnowledgeHolder
		var validTo sql.NullInt64
		var provenance string
		if err := rows.Scan(&holder.SecretID, &holder.EntityID, &holder.ValidFromChapter, &validTo, &holder.SourceVersion, &holder.Authority, &provenance); err != nil {
			return nil, err
		}
		holder.ValidToChapter = intPointer(validTo)
		if err := json.Unmarshal([]byte(provenance), &holder.Provenance); err != nil {
			return nil, err
		}
		out = append(out, holder)
	}
	return out, rows.Err()
}

func (s *Store) Dashboard(ctx context.Context, projectID string, chapter int) (Dashboard, error) {
	if chapter < 0 {
		return Dashboard{}, fmt.Errorf("%w: chapter must not be negative", ErrValidation)
	}
	var out Dashboard
	out.Chapter = chapter
	statement := `SELECT
		SUM(CASE WHEN status NOT IN ('resolved','abandoned') THEN 1 ELSE 0 END),
		SUM(CASE WHEN status NOT IN ('resolved','abandoned') AND expected_payoff_max < ? THEN 1 ELSE 0 END),
		SUM(CASE WHEN importance='critical' AND status NOT IN ('resolved','abandoned') AND expected_payoff_max < ? THEN 1 ELSE 0 END),
		SUM(CASE WHEN status NOT IN ('resolved','abandoned') AND expected_payoff_min BETWEEN ? AND ? THEN 1 ELSE 0 END)
		FROM foreshadows WHERE project_id=?`
	var active, overdue, critical, upcoming sql.NullInt64
	if err := s.db.QueryRowContext(ctx, statement, chapter, chapter, chapter, chapter+3, projectID).Scan(&active, &overdue, &critical, &upcoming); err != nil {
		return Dashboard{}, err
	}
	out.ActiveForeshadows = int(active.Int64)
	out.OverdueCount = int(overdue.Int64)
	out.CriticalOverdue = int(critical.Int64)
	out.UpcomingPayoffs = int(upcoming.Int64)
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM secrets WHERE project_id=? AND created_chapter<=? AND (revealed_chapter IS NULL OR revealed_chapter>?)`, projectID, chapter, chapter).Scan(&out.UnrevealedSecrets); err != nil {
		return Dashboard{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM secret_holders h JOIN secrets s ON s.id=h.secret_id WHERE s.project_id=? AND (h.valid_to_chapter IS NOT NULL AND h.valid_to_chapter<h.valid_from_chapter)`, projectID).Scan(&out.KnowledgeBoundaryWarnings); err != nil {
		return Dashboard{}, err
	}
	return out, nil
}

func (s *Store) Diagnostics(ctx context.Context, projectID string, chapter int) ([]Diagnostic, error) {
	page, err := s.ListForeshadows(ctx, projectID, ForeshadowQuery{CurrentChapter: chapter, Limit: 100})
	if err != nil {
		return nil, err
	}
	out := []Diagnostic{}
	for _, item := range page.Foreshadows {
		if item.Overdue {
			out = append(out, newDiagnostic("OVERDUE_FORESHADOW", "warning", projectID, chapter, item.ID, "foreshadow payoff window has passed", map[string]any{"overdue_by_chapters": item.OverdueByChapters, "expected_payoff_min": item.ExpectedPayoffMin, "expected_payoff_max": item.ExpectedPayoffMax, "importance": item.Importance, "urgency": item.Urgency}))
		}
		if item.Status == StatusContradicted {
			out = append(out, newDiagnostic("CONTRADICTED_FORESHADOW", "error", projectID, chapter, item.ID, "foreshadow is marked contradicted", map[string]any{"status": item.Status}))
		}
		if item.Status != StatusResolved && item.Status != StatusAbandoned && chapter-item.LastProgressChapter > 20 {
			out = append(out, newDiagnostic("STALE_FORESHADOW_PROGRESS", "warning", projectID, chapter, item.ID, "foreshadow has not progressed recently", map[string]any{"last_progress_chapter": item.LastProgressChapter}))
		}
		if item.ActualPayoff != nil && *item.ActualPayoff < item.PlantedChapter {
			out = append(out, newDiagnostic("PAYOFF_BEFORE_PLANT", "error", projectID, chapter, item.ID, "actual payoff precedes plant chapter", nil))
		}
		if item.ExpectedPayoffMax < item.ExpectedPayoffMin {
			out = append(out, newDiagnostic("INVALID_PAYOFF_RANGE", "error", projectID, chapter, item.ID, "payoff range is invalid", nil))
		}
	}
	secrets, err := s.ListSecrets(ctx, projectID, SecretQuery{CurrentChapter: chapter, IncludeTruth: false, Limit: 100})
	if err != nil {
		return nil, err
	}
	for _, secret := range secrets.Secrets {
		if secret.RevealedChapter != nil && *secret.RevealedChapter < secret.CreatedChapter {
			out = append(out, newDiagnostic("SECRET_REVEAL_BEFORE_CREATE", "error", projectID, chapter, secret.ID, "secret reveal precedes creation", nil))
		}
	}
	var lastChapter int
	if err := s.db.QueryRowContext(ctx, `SELECT last_commit_chapter FROM narrative_ledger_meta WHERE id=1`).Scan(&lastChapter); err == nil && lastChapter > chapter {
		out = append(out, newDiagnostic("LEDGER_PROJECTION_STALE", "warning", projectID, chapter, "ledger", "ledger projection is ahead of requested chapter boundary", map[string]any{"last_commit_chapter": lastChapter}))
	}
	return out, nil
}

func newDiagnostic(code, severity, projectID string, chapter int, entity, message string, evidence any) Diagnostic {
	data := json.RawMessage(`{}`)
	if evidence != nil {
		data, _ = json.Marshal(evidence)
	}
	return Diagnostic{ID: stableID("diag", code, projectID, fmt.Sprint(chapter), entity), Code: code, Severity: severity, ProjectID: projectID, Chapter: chapter, Entity: entity, Message: message, Retryable: false, Evidence: data}
}

func normalizePage(limit, offset int) (int, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		return 0, 0, fmt.Errorf("%w: limit exceeds 100", ErrValidation)
	}
	if offset < 0 {
		return 0, 0, fmt.Errorf("%w: offset must not be negative", ErrValidation)
	}
	return limit, offset, nil
}

var _ truthstore.Authority
