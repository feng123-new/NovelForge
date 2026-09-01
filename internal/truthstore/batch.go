package truthstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// StateMany executes the whole selector batch as one SQLite statement. This
// avoids the per-selector count/read pattern that would otherwise become an
// N+1 query path for Context Compiler and continuity checks.
func (s *Store) StateMany(ctx context.Context, queries []StateQuery) ([]StatePage, error) {
	if len(queries) == 0 {
		return []StatePage{}, nil
	}
	if len(queries) > 100 {
		return nil, newError(CodeValidation, "at most 100 truth queries are allowed per batch", false, nil)
	}

	pages := make([]StatePage, len(queries))
	values := make([]string, 0, len(queries))
	args := make([]any, 0, len(queries)*7)
	for index, query := range queries {
		if query.Chapter < 0 {
			return nil, newError(CodeValidation, "chapter must not be negative", false, nil)
		}
		limit, offset, err := normalizePage(query.Limit, query.Offset)
		if err != nil {
			return nil, err
		}
		subjectType, err := trimFilter(query.SubjectType, true)
		if err != nil {
			return nil, err
		}
		subjectID, err := trimFilter(query.SubjectID, false)
		if err != nil {
			return nil, err
		}
		predicate, err := trimFilter(query.Predicate, true)
		if err != nil {
			return nil, err
		}
		pages[index] = StatePage{Facts: []Fact{}, Limit: limit, Offset: offset}
		values = append(values, "(?, ?, ?, ?, ?, ?, ?)")
		args = append(args, index, query.Chapter, subjectType, subjectID, predicate, limit, offset)
	}

	statement := `WITH selectors(selector_index, chapter, subject_type, subject_id, predicate, page_limit, page_offset) AS (
		VALUES ` + strings.Join(values, ",") + `
	), ranked AS (
		SELECT s.selector_index, s.page_limit, s.page_offset,
			COUNT(*) OVER (PARTITION BY s.selector_index) AS total,
			ROW_NUMBER() OVER (PARTITION BY s.selector_index ORDER BY f.authority_rank DESC, f.sequence DESC) AS row_number,
			` + prefixedEventColumns("e") + `,
			f.effective_from_chapter, f.effective_to_chapter, f.superseded_by_event_id,
			EXISTS(SELECT 1 FROM truth_conflicts c WHERE
				(c.left_event_id=f.event_id OR c.right_event_id=f.event_id) AND
				c.status='unresolved' AND c.from_chapter <= s.chapter AND
				(c.to_chapter IS NULL OR c.to_chapter >= s.chapter)) AS conflicted
		FROM selectors s
		JOIN truth_facts f ON
			f.effective_from_chapter <= s.chapter AND
			(f.effective_to_chapter IS NULL OR f.effective_to_chapter >= s.chapter) AND
			(s.subject_type = '' OR f.subject_type = s.subject_type) AND
			(s.subject_id = '' OR f.subject_id = s.subject_id) AND
			(s.predicate = '' OR f.predicate = s.predicate)
		JOIN truth_events e ON e.id=f.event_id
	)
	SELECT selector_index, total, ` + eventColumns + `,
		effective_from_chapter, effective_to_chapter, superseded_by_event_id, conflicted
	FROM ranked
	WHERE row_number > page_offset AND row_number <= page_offset + page_limit
	ORDER BY selector_index, row_number`

	rows, err := s.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, classifyStorageError("truth state batch could not be queried", err)
	}
	defer rows.Close()
	for rows.Next() {
		index, total, fact, err := scanBatchFact(rows)
		if err != nil {
			return nil, classifyStorageError("truth state batch row could not be decoded", err)
		}
		if index < 0 || index >= len(pages) {
			return nil, newError(CodeCorrupt, fmt.Sprintf("truth state batch returned invalid selector %d", index), false, nil)
		}
		pages[index].Total = total
		if fact.Conflicted {
			pages[index].Conflicts++
		}
		pages[index].Facts = append(pages[index].Facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyStorageError("truth state batch iteration failed", err)
	}
	for index := range pages {
		if pages[index].Offset+len(pages[index].Facts) < pages[index].Total {
			next := pages[index].Offset + len(pages[index].Facts)
			pages[index].NextOffset = &next
		}
	}
	return pages, nil
}

func scanBatchFact(row scanner) (int, int, Fact, error) {
	var index, total int
	var fact Fact
	var value, created string
	var validTo, knownTo, effectiveTo sql.NullInt64
	var supersedes, supersededBy sql.NullString
	var conflicted int
	if err := row.Scan(&index, &total,
		&fact.Sequence, &fact.ID, &fact.IdempotencyKey, &fact.RequestHash,
		&fact.Kind, &fact.SubjectType, &fact.SubjectID, &fact.Predicate, &value,
		&fact.ValidFromChapter, &validTo, &fact.KnownFromChapter, &knownTo,
		&fact.Authority, &fact.Confidence, &fact.Source.Type, &fact.Source.ID,
		&fact.Source.Chapter, &fact.Source.Version, &fact.Source.Extractor,
		&fact.Source.ConfirmedBy, &fact.Source.Excerpt, &supersedes, &created,
		&fact.Checksum, &fact.EffectiveFromChapter, &effectiveTo, &supersededBy,
		&conflicted); err != nil {
		return 0, 0, Fact{}, err
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
		return 0, 0, Fact{}, err
	}
	fact.CreatedAt = parsed
	fact.Conflicted = conflicted != 0
	return index, total, fact, nil
}
