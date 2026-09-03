package chapterversion

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/voocel/ainovel-cli/internal/qualitygate"
	"github.com/voocel/ainovel-cli/internal/truthstore"
)

func (c *Coordinator) commitTruth(ctx context.Context, operationID string, final Version, proposal qualitygate.FactProposal, authority truthstore.Authority) (int, error) {
	if final.Type != TypeFinal {
		return 0, newError(CodeFinalizeNotAllowed, "only an immutable Final ChapterVersion may submit Truth", false, nil)
	}
	var saved string
	if err := c.Store.db.QueryRowContext(ctx, `SELECT truth_event_ids_json FROM chapter_finalize_sagas WHERE operation_id=?`, operationID).Scan(&saved); err != nil {
		return 0, newError(CodeStorage, "Truth commit evidence could not be read", true, err)
	}
	var savedIDs []string
	if json.Unmarshal([]byte(saved), &savedIDs) == nil && len(savedIDs) > 0 {
		return len(savedIDs), nil
	}

	evaluation, ok, err := c.Store.latestEvaluation(ctx, final.ParentVersionID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, newError(CodeFinalizeNotAllowed, "persisted evaluation is required before Truth commit", false, nil)
	}
	ids := []string{}
	for index, change := range proposal.AllChanges() {
		subjectType, subjectID := splitSubject(change.Subject)
		conflicts := conflictsForChange(evaluation.Conflicts, subjectType, subjectID, change.Predicate)
		if len(conflicts) == 0 {
			input := truthstore.AppendInput{}
			input.IdempotencyKey = safeTruthKey(operationID, index, "assert")
			input.Kind = truthstore.EventAssert
			input.SubjectType = subjectType
			input.SubjectID = subjectID
			input.Predicate = change.Predicate
			input.Value = append(json.RawMessage(nil), change.Object...)
			input.ValidFromChapter = change.ValidFromChapter
			input.ValidToChapter = change.ValidToChapter
			input.KnownFromChapter = change.KnownFromChapter
			input.KnownToChapter = change.KnownToChapter
			input.Authority = authority
			input.Confidence = change.Confidence
			input.Source = truthSource(final, change.Extractor, authority)
			result, appendErr := c.Truth.Append(ctx, input)
			if appendErr != nil {
				return 0, newError(CodeTruthConflict, "Truth commit failed", true, appendErr)
			}
			ids = append(ids, result.Event.ID)
			continue
		}
		if authority != truthstore.AuthorityHumanFinal {
			return 0, newError(CodeTruthConflict, "generated final conflicts with current Truth", false, nil)
		}
		for conflictIndex, conflict := range conflicts {
			input := truthstore.AppendInput{}
			input.IdempotencyKey = safeTruthKey(operationID, index, "supersede:"+strconv.Itoa(conflictIndex)+":"+conflict.ExistingEventID)
			input.Kind = truthstore.EventSupersede
			input.SubjectType = subjectType
			input.SubjectID = subjectID
			input.Predicate = change.Predicate
			input.Value = append(json.RawMessage(nil), change.Object...)
			input.ValidFromChapter = change.ValidFromChapter
			input.ValidToChapter = change.ValidToChapter
			input.KnownFromChapter = change.KnownFromChapter
			input.KnownToChapter = change.KnownToChapter
			input.Authority = authority
			input.Confidence = change.Confidence
			input.SupersedesEventID = conflict.ExistingEventID
			input.Source = truthSource(final, change.Extractor, authority)
			result, appendErr := c.Truth.Append(ctx, input)
			if appendErr != nil {
				return 0, newError(CodeTruthConflict, "Accepted Human Final could not supersede conflicting Truth", false, appendErr)
			}
			ids = append(ids, result.Event.ID)
		}
	}

	encoded, _ := json.Marshal(ids)
	_, err = c.Store.db.ExecContext(ctx, `UPDATE chapter_finalize_sagas SET truth_event_ids_json=?,updated_at=? WHERE operation_id=?`,
		string(encoded), c.Store.now().UTC().Format(time.RFC3339Nano), operationID)
	if err != nil {
		return 0, newError(CodeStorage, "Truth commit evidence could not be recorded", true, err)
	}
	return len(ids), nil
}

func conflictsForChange(conflicts []CandidateConflict, subjectType, subjectID, predicate string) []CandidateConflict {
	matches := []CandidateConflict{}
	for _, conflict := range conflicts {
		if conflict.SubjectType == subjectType && conflict.SubjectID == subjectID && conflict.Predicate == predicate {
			matches = append(matches, conflict)
		}
	}
	return matches
}

func truthSource(final Version, extractor string, authority truthstore.Authority) truthstore.Source {
	source := truthstore.Source{}
	source.Type = truthSourceType(authority)
	source.ID = final.ID
	source.Chapter = final.Chapter
	source.Version = final.ID
	source.Extractor = extractor
	source.ConfirmedBy = confirmedBy(authority)
	return source
}

func safeTruthKey(operationID string, index int, suffix string) string {
	return "p8:" + hashText(operationID + ":" + strconv.Itoa(index) + ":" + suffix)[:48]
}

func truthSourceType(authority truthstore.Authority) string {
	if authority == truthstore.AuthorityHumanFinal {
		return "chapter_human_final"
	}
	return "chapter_final"
}

func confirmedBy(authority truthstore.Authority) string {
	if authority == truthstore.AuthorityHumanFinal {
		return "human"
	}
	return ""
}
