package truthstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type projectedFact struct {
	Event
	ValueHash            string
	EffectiveFromChapter int
	EffectiveToChapter   *int
	SupersededByEventID  string
}

func (s *Store) Rebuild(ctx context.Context, fromChapter int) (RebuildResult, error) {
	if fromChapter < 0 {
		return RebuildResult{}, newError(CodeValidation, "from_chapter must not be negative", false, nil)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RebuildResult{}, classifyStorageError("truth rebuild transaction could not start", err)
	}
	defer tx.Rollback()
	events, err := allEventsTx(ctx, tx)
	if err != nil {
		return RebuildResult{}, err
	}
	facts, conflicts, err := projectEvents(events)
	if err != nil {
		return RebuildResult{}, err
	}
	if fromChapter == 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM truth_conflicts; DELETE FROM truth_facts`); err != nil {
			return RebuildResult{}, classifyStorageError("truth projection could not be cleared", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `DELETE FROM truth_conflicts WHERE to_chapter IS NULL OR to_chapter >= ?`, fromChapter); err != nil {
			return RebuildResult{}, classifyStorageError("truth conflicts could not be cleared from boundary", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM truth_facts WHERE effective_to_chapter IS NULL OR effective_to_chapter >= ?`, fromChapter); err != nil {
			return RebuildResult{}, classifyStorageError("truth facts could not be cleared from boundary", err)
		}
	}
	factsWritten := 0
	for _, fact := range facts {
		if fromChapter > 0 && fact.EffectiveToChapter != nil && *fact.EffectiveToChapter < fromChapter {
			continue
		}
		rank, _ := fact.Authority.rank()
		if _, err := tx.ExecContext(ctx, `INSERT INTO truth_facts(event_id, sequence, subject_type,
			subject_id, predicate, value_json, value_hash, valid_from_chapter,
			valid_to_chapter, known_from_chapter, known_to_chapter,
			effective_from_chapter, effective_to_chapter, authority, authority_rank,
			confidence, superseded_by_event_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			fact.ID, fact.Sequence, fact.SubjectType, fact.SubjectID, fact.Predicate,
			string(fact.Value), fact.ValueHash, fact.ValidFromChapter,
			nullableInt(fact.ValidToChapter), fact.KnownFromChapter,
			nullableInt(fact.KnownToChapter), fact.EffectiveFromChapter,
			nullableInt(fact.EffectiveToChapter), fact.Authority, rank,
			fact.Confidence, nullableString(fact.SupersededByEventID)); err != nil {
			return RebuildResult{}, classifyStorageError("truth fact could not be rebuilt", err)
		}
		factsWritten++
	}
	conflictsWritten := 0
	for _, conflict := range conflicts {
		if fromChapter > 0 && conflict.ToChapter != nil && *conflict.ToChapter < fromChapter {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO truth_conflicts(id, subject_type, subject_id,
			predicate, left_event_id, right_event_id, from_chapter, to_chapter, status, reason)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, conflict.ID, conflict.SubjectType,
			conflict.SubjectID, conflict.Predicate, conflict.LeftEventID,
			conflict.RightEventID, conflict.FromChapter, nullableInt(conflict.ToChapter),
			conflict.Status, conflict.Reason); err != nil {
			return RebuildResult{}, classifyStorageError("truth conflict could not be rebuilt", err)
		}
		conflictsWritten++
	}
	lastSequence := int64(0)
	if len(events) > 0 {
		lastSequence = events[len(events)-1].Sequence
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE truth_projection_meta SET last_sequence=?, last_rebuild_from_chapter=?, updated_at=? WHERE id=1`, lastSequence, fromChapter, now); err != nil {
		return RebuildResult{}, classifyStorageError("truth projection metadata could not be rebuilt", err)
	}
	if err := tx.Commit(); err != nil {
		return RebuildResult{}, classifyStorageError("truth rebuild could not commit", err)
	}
	digest := projectionDigest(facts, conflicts)
	return RebuildResult{FromChapter: fromChapter, EventsReplayed: len(events), FactsProjected: factsWritten,
		ConflictsProjected: conflictsWritten, ProjectionDigest: digest}, nil
}

func (s *Store) Verify(ctx context.Context) (VerifyResult, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return VerifyResult{}, classifyStorageError("truth verification could not start", err)
	}
	defer tx.Rollback()
	events, err := allEventsTx(ctx, tx)
	if err != nil {
		return VerifyResult{}, err
	}
	expectedFacts, expectedConflicts, projectionErr := projectEvents(events)
	result := VerifyResult{Events: len(events), Violations: []string{}}
	if projectionErr != nil {
		result.Violations = append(result.Violations, projectionErr.Error())
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM truth_facts`).Scan(&result.Facts); err != nil {
		return VerifyResult{}, classifyStorageError("truth fact count could not be verified", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM truth_conflicts`).Scan(&result.Conflicts); err != nil {
		return VerifyResult{}, classifyStorageError("truth conflict count could not be verified", err)
	}
	if projectionErr == nil {
		if result.Facts != len(expectedFacts) {
			result.Violations = append(result.Violations, fmt.Sprintf("fact projection count mismatch: have %d want %d", result.Facts, len(expectedFacts)))
		}
		if result.Conflicts != len(expectedConflicts) {
			result.Violations = append(result.Violations, fmt.Sprintf("conflict projection count mismatch: have %d want %d", result.Conflicts, len(expectedConflicts)))
		}
		expectedDigest := projectionDigest(expectedFacts, expectedConflicts)
		actualDigest, err := actualProjectionDigest(ctx, tx)
		if err != nil {
			return VerifyResult{}, err
		}
		result.ProjectionDigest = actualDigest
		if expectedDigest != actualDigest {
			result.Violations = append(result.Violations, "projection digest mismatch")
		}
	}
	result.Valid = len(result.Violations) == 0
	if err := tx.Commit(); err != nil {
		return VerifyResult{}, classifyStorageError("truth verification could not finish", err)
	}
	return result, nil
}

func allEventsTx(ctx context.Context, tx *sql.Tx) ([]Event, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+eventColumns+` FROM truth_events ORDER BY sequence`)
	if err != nil {
		return nil, classifyStorageError("truth events could not be read for rebuild", err)
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, classifyStorageError("truth event could not be decoded for rebuild", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyStorageError("truth rebuild event iteration failed", err)
	}
	return events, nil
}

func projectEvents(events []Event) ([]projectedFact, []Conflict, error) {
	facts := make([]projectedFact, 0, len(events))
	byID := make(map[string]int, len(events))
	for _, event := range events {
		if event.Checksum == "" || eventChecksum(event) != event.Checksum {
			return nil, nil, newError(CodeCorrupt, fmt.Sprintf("truth event %s failed checksum verification", event.ID), false, nil)
		}
		if event.SupersedesEventID != "" {
			targetIndex, exists := byID[event.SupersedesEventID]
			if !exists {
				return nil, nil, newError(CodeCorrupt, fmt.Sprintf("truth event %s has an invalid supersede target", event.ID), false, nil)
			}
			target := &facts[targetIndex]
			if target.Kind == EventRetract || !sameKey(target.Event, event) {
				return nil, nil, newError(CodeCorrupt, fmt.Sprintf("truth event %s has an invalid supersede target", event.ID), false, nil)
			}
			newRank, newOK := event.Authority.rank()
			targetRank, targetOK := target.Authority.rank()
			if !newOK || !targetOK || newRank < targetRank {
				return nil, nil, newError(CodeCorrupt, fmt.Sprintf("truth event %s violates authority ordering", event.ID), false, nil)
			}
			if target.SupersededByEventID != "" {
				return nil, nil, newError(CodeCorrupt, fmt.Sprintf("truth event %s supersedes an already closed fact", event.ID), false, nil)
			}
			effectiveFrom, _ := effectiveBounds(event.ValidFromChapter, event.ValidToChapter, event.KnownFromChapter, event.KnownToChapter)
			if effectiveFrom < target.EffectiveFromChapter {
				return nil, nil, newError(CodeCorrupt, fmt.Sprintf("truth event %s begins before its supersede target", event.ID), false, nil)
			}
			end := effectiveFrom - 1
			if target.EffectiveToChapter == nil || end < *target.EffectiveToChapter {
				target.EffectiveToChapter = &end
			}
			target.SupersededByEventID = event.ID
		}
		if event.Kind == EventRetract {
			continue
		}
		_, valueHash, err := canonicalizeValue(event.Kind, event.Value)
		if err != nil {
			return nil, nil, newError(CodeCorrupt, fmt.Sprintf("truth event %s has invalid value JSON", event.ID), false, err)
		}
		from, to := effectiveBounds(event.ValidFromChapter, event.ValidToChapter, event.KnownFromChapter, event.KnownToChapter)
		fact := projectedFact{Event: event, ValueHash: valueHash, EffectiveFromChapter: from, EffectiveToChapter: to}
		facts = append(facts, fact)
		byID[event.ID] = len(facts) - 1
	}
	groups := map[string][]projectedFact{}
	for _, fact := range facts {
		key := strings.Join([]string{fact.SubjectType, fact.SubjectID, fact.Predicate}, "\x00")
		groups[key] = append(groups[key], fact)
	}
	conflicts := []Conflict{}
	for _, group := range groups {
		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				left, right := group[i], group[j]
				if left.ValueHash == right.ValueHash {
					continue
				}
				from, to, ok := intervalIntersection(left.EffectiveFromChapter, left.EffectiveToChapter, right.EffectiveFromChapter, right.EffectiveToChapter)
				if !ok {
					continue
				}
				conflicts = append(conflicts, newConflict(left.SubjectType, left.SubjectID, left.Predicate, left.ID, right.ID, from, to))
			}
		}
	}
	sort.Slice(conflicts, func(i, j int) bool { return conflicts[i].ID < conflicts[j].ID })
	return facts, conflicts, nil
}

func projectionDigest(facts []projectedFact, conflicts []Conflict) string {
	factRows := make([]string, 0, len(facts))
	for _, fact := range facts {
		factRows = append(factRows, projectionFactRow(fact))
	}
	sort.Strings(factRows)
	conflictRows := make([]string, 0, len(conflicts))
	for _, conflict := range conflicts {
		conflictRows = append(conflictRows, projectionConflictRow(conflict))
	}
	sort.Strings(conflictRows)
	return sha256Text(strings.Join(append(factRows, conflictRows...), "\n"))
}

func actualProjectionDigest(ctx context.Context, tx *sql.Tx) (string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT event_id, sequence, subject_type, subject_id,
		predicate, value_hash, valid_from_chapter, valid_to_chapter, known_from_chapter,
		known_to_chapter, effective_from_chapter, effective_to_chapter, authority,
		authority_rank, confidence, superseded_by_event_id FROM truth_facts ORDER BY event_id`)
	if err != nil {
		return "", classifyStorageError("truth projection digest could not be read", err)
	}
	factRows := []string{}
	for rows.Next() {
		var fact projectedFact
		var validTo, knownTo, effectiveTo sql.NullInt64
		var authorityRank int
		var supersededBy sql.NullString
		if err := rows.Scan(&fact.ID, &fact.Sequence, &fact.SubjectType, &fact.SubjectID,
			&fact.Predicate, &fact.ValueHash, &fact.ValidFromChapter, &validTo,
			&fact.KnownFromChapter, &knownTo, &fact.EffectiveFromChapter, &effectiveTo,
			&fact.Authority, &authorityRank, &fact.Confidence, &supersededBy); err != nil {
			_ = rows.Close()
			return "", classifyStorageError("truth projection digest row could not be decoded", err)
		}
		fact.ValidToChapter = intPointer(validTo)
		fact.KnownToChapter = intPointer(knownTo)
		fact.EffectiveToChapter = intPointer(effectiveTo)
		fact.SupersededByEventID = supersededBy.String
		expectedRank, ok := fact.Authority.rank()
		if !ok || authorityRank != expectedRank {
			_ = rows.Close()
			return "", newError(CodeCorrupt, "truth projection authority rank is invalid", false, nil)
		}
		factRows = append(factRows, projectionFactRow(fact))
	}
	if err := rows.Close(); err != nil {
		return "", classifyStorageError("truth projection digest rows could not be closed", err)
	}
	rows, err = tx.QueryContext(ctx, `SELECT id, subject_type, subject_id, predicate, left_event_id,
		right_event_id, from_chapter, to_chapter, status, reason FROM truth_conflicts ORDER BY id`)
	if err != nil {
		return "", classifyStorageError("truth conflict digest could not be read", err)
	}
	conflictRows := []string{}
	for rows.Next() {
		var conflict Conflict
		var to sql.NullInt64
		if err := rows.Scan(&conflict.ID, &conflict.SubjectType, &conflict.SubjectID, &conflict.Predicate,
			&conflict.LeftEventID, &conflict.RightEventID, &conflict.FromChapter, &to, &conflict.Status, &conflict.Reason); err != nil {
			_ = rows.Close()
			return "", classifyStorageError("truth conflict digest row could not be decoded", err)
		}
		conflict.ToChapter = intPointer(to)
		conflictRows = append(conflictRows, projectionConflictRow(conflict))
	}
	if err := rows.Close(); err != nil {
		return "", classifyStorageError("truth conflict digest rows could not be closed", err)
	}
	return sha256Text(strings.Join(append(factRows, conflictRows...), "\n")), nil
}

func projectionFactRow(fact projectedFact) string {
	validTo := chapterBound(fact.ValidToChapter)
	knownTo := chapterBound(fact.KnownToChapter)
	effectiveTo := chapterBound(fact.EffectiveToChapter)
	rank, _ := fact.Authority.rank()
	return strings.Join([]string{
		"fact", fact.ID, fmt.Sprint(fact.Sequence), fact.SubjectType, fact.SubjectID,
		fact.Predicate, fact.ValueHash, fmt.Sprint(fact.ValidFromChapter), validTo,
		fmt.Sprint(fact.KnownFromChapter), knownTo, fmt.Sprint(fact.EffectiveFromChapter),
		effectiveTo, string(fact.Authority), fmt.Sprint(rank),
		strconv.FormatFloat(fact.Confidence, 'g', -1, 64), fact.SupersededByEventID,
	}, "|")
}

func chapterBound(value *int) string {
	if value == nil {
		return "open"
	}
	return fmt.Sprint(*value)
}

func projectionConflictRow(conflict Conflict) string {
	toValue := "open"
	if conflict.ToChapter != nil {
		toValue = fmt.Sprint(*conflict.ToChapter)
	}
	return strings.Join([]string{"conflict", conflict.ID, conflict.LeftEventID, conflict.RightEventID,
		fmt.Sprint(conflict.FromChapter), toValue, string(conflict.Status), conflict.Reason}, "|")
}

var _ = json.Valid
var _ = errors.Is
