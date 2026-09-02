package narrativeledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type rowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func scanForeshadow(ctx context.Context, queryer rowQueryer, key string, chapter int) (Foreshadow, bool, error) {
	chapterSQL := strconv.Itoa(chapter)
	query := `SELECT id, key, title, description, priority, status,
		CASE WHEN status IN ('planned', 'planted', 'reinforced')
			AND due_chapter IS NOT NULL AND due_chapter < ` + chapterSQL + `
			THEN 'overdue' ELSE status END AS effective_status,
		planted_chapter, due_chapter, reveal_chapter, source_transaction_id,
		updated_chapter, created_at, updated_at
	FROM foreshadows WHERE key = ?`
	var result Foreshadow
	var planted, due, reveal sql.NullInt64
	var createdAt, updatedAt string
	err := queryer.QueryRowContext(ctx, query, normalizeKey(key)).Scan(
		&result.ID,
		&result.Key,
		&result.Title,
		&result.Description,
		&result.Priority,
		&result.Status,
		&result.EffectiveStatus,
		&planted,
		&due,
		&reveal,
		&result.SourceTransaction,
		&result.UpdatedChapter,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Foreshadow{}, false, nil
	}
	if err != nil {
		return Foreshadow{}, false, newError("LEDGER_DATABASE_READ_FAILED", "foreshadow could not be read", err)
	}
	result.PlantedChapter = nullIntPointer(planted)
	result.DueChapter = nullIntPointer(due)
	result.RevealChapter = nullIntPointer(reveal)
	result.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Foreshadow{}, false, err
	}
	result.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return Foreshadow{}, false, err
	}
	return result, true, nil
}

// GetForeshadow returns one deterministic Chapter-N view by stable key.
func (s *Store) GetForeshadow(ctx context.Context, key string, asOfChapter int) (Foreshadow, error) {
	if asOfChapter < 0 {
		return Foreshadow{}, newError("LEDGER_CHAPTER_INVALID", "chapter must not be negative", ErrValidation)
	}
	if asOfChapter == 0 {
		current, err := s.currentChapter(ctx)
		if err != nil {
			return Foreshadow{}, err
		}
		asOfChapter = current
	}
	result, found, err := scanForeshadow(ctx, s.db, key, asOfChapter)
	if err != nil {
		return Foreshadow{}, err
	}
	if !found {
		return Foreshadow{}, newError("LEDGER_FORESHADOW_NOT_FOUND", "foreshadow was not found", ErrNotFound)
	}
	return result, nil
}

// ListForeshadows performs indexed, bounded and deterministic pagination.
func (s *Store) ListForeshadows(ctx context.Context, input ListOptions) (ForeshadowPage, error) {
	options, err := normalizeListOptions(input)
	if err != nil {
		return ForeshadowPage{}, err
	}
	if options.AsOfChapter == 0 {
		options.AsOfChapter, err = s.currentChapter(ctx)
		if err != nil {
			return ForeshadowPage{}, err
		}
	}
	chapterSQL := strconv.Itoa(options.AsOfChapter)
	effective := `CASE WHEN f.status IN ('planned', 'planted', 'reinforced')
		AND f.due_chapter IS NOT NULL AND f.due_chapter < ` + chapterSQL + `
		THEN 'overdue' ELSE f.status END`
	where := []string{"1 = 1"}
	args := []any{}
	if options.Status != "" {
		switch ForeshadowStatus(options.Status) {
		case ForeshadowPlanned, ForeshadowPlanted, ForeshadowReinforced,
			ForeshadowRevealed, ForeshadowAbandoned, ForeshadowOverdue:
			where = append(where, effective+" = ?")
			args = append(args, options.Status)
		default:
			return ForeshadowPage{}, newError("LEDGER_FORESHADOW_STATUS_INVALID", "foreshadow status filter is invalid", ErrValidation)
		}
	}
	if options.Priority != "" {
		where = append(where, "f.priority = ?")
		args = append(args, options.Priority)
	}
	if options.Query != "" {
		where = append(where, "(LOWER(f.key) LIKE ? OR LOWER(f.title) LIKE ? OR LOWER(f.description) LIKE ?)")
		needle := "%" + strings.ToLower(options.Query) + "%"
		args = append(args, needle, needle, needle)
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM foreshadows AS f WHERE `+clause, args...).Scan(&total); err != nil {
		return ForeshadowPage{}, newError("LEDGER_DATABASE_READ_FAILED", "foreshadow count could not be read", err)
	}
	query := `SELECT f.id, f.key, f.title, f.description, f.priority, f.status,
		` + effective + ` AS effective_status,
		f.planted_chapter, f.due_chapter, f.reveal_chapter,
		f.source_transaction_id, f.updated_chapter, f.created_at, f.updated_at
	FROM foreshadows AS f
	WHERE ` + clause + `
	ORDER BY
		CASE WHEN ` + effective + ` = 'overdue' THEN 0 ELSE 1 END,
		CASE f.priority WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END,
		COALESCE(f.due_chapter, 2147483647), f.key
	LIMIT ? OFFSET ?`
	queryArgs := append(append([]any{}, args...), options.Limit, options.Offset)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return ForeshadowPage{}, newError("LEDGER_DATABASE_READ_FAILED", "foreshadow page could not be read", err)
	}
	defer rows.Close()
	items := make([]Foreshadow, 0, options.Limit)
	for rows.Next() {
		var item Foreshadow
		var planted, due, reveal sql.NullInt64
		var createdAt, updatedAt string
		if err := rows.Scan(
			&item.ID,
			&item.Key,
			&item.Title,
			&item.Description,
			&item.Priority,
			&item.Status,
			&item.EffectiveStatus,
			&planted,
			&due,
			&reveal,
			&item.SourceTransaction,
			&item.UpdatedChapter,
			&createdAt,
			&updatedAt,
		); err != nil {
			return ForeshadowPage{}, newError("LEDGER_DATABASE_READ_FAILED", "foreshadow row could not be decoded", err)
		}
		item.PlantedChapter = nullIntPointer(planted)
		item.DueChapter = nullIntPointer(due)
		item.RevealChapter = nullIntPointer(reveal)
		item.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return ForeshadowPage{}, err
		}
		item.UpdatedAt, err = parseTime(updatedAt)
		if err != nil {
			return ForeshadowPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ForeshadowPage{}, newError("LEDGER_DATABASE_READ_FAILED", "foreshadow page could not be completed", err)
	}
	return ForeshadowPage{
		Items:      items,
		Total:      total,
		Limit:      options.Limit,
		Offset:     options.Offset,
		NextOffset: nextOffset(total, options.Offset, options.Limit),
	}, nil
}

func (s *Store) currentChapter(ctx context.Context) (int, error) {
	var chapter int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(chapter), 0) FROM narrative_ledger_commits`).Scan(&chapter); err != nil {
		return 0, newError("LEDGER_DATABASE_READ_FAILED", "current ledger chapter could not be read", err)
	}
	return chapter, nil
}

// ExplainForeshadowSchedule returns SQLite's plan for the blocking schedule query.
func (s *Store) ExplainForeshadowSchedule(ctx context.Context, chapter, limit int) ([]string, error) {
	if chapter < 0 || limit <= 0 {
		return nil, newError("LEDGER_PAGINATION_INVALID", "chapter and limit are invalid", ErrValidation)
	}
	rows, err := s.db.QueryContext(ctx, `EXPLAIN QUERY PLAN
		SELECT key FROM foreshadows INDEXED BY idx_foreshadows_status_due_priority
		WHERE status IN ('planned', 'planted', 'reinforced')
			AND due_chapter IS NOT NULL AND due_chapter < ?
		ORDER BY status, due_chapter, priority, key LIMIT ?`, chapter, limit)
	if err != nil {
		return nil, newError("LEDGER_DATABASE_READ_FAILED", "foreshadow schedule plan could not be read", err)
	}
	defer rows.Close()
	plans := []string{}
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			return nil, newError("LEDGER_DATABASE_READ_FAILED", "foreshadow schedule plan could not be decoded", err)
		}
		plans = append(plans, fmt.Sprintf("%d:%d:%s", id, parent, detail))
	}
	return plans, rows.Err()
}
