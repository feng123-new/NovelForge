package chapterversion

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/contextcompiler"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
	"github.com/voocel/ainovel-cli/internal/truthstore"
)

func (c *Coordinator) Finalize(ctx context.Context, chapter int, key, versionID string) (FinalizeResult, error) {
	if err := c.validate(); err != nil {
		return FinalizeResult{}, err
	}
	if c.Truth == nil || c.Ledger == nil || c.FinalWriter == nil {
		return FinalizeResult{}, newError(CodeFinalizeNotAllowed, "Truth, Narrative Ledger and final chapter writer are required", false, nil)
	}
	candidate, err := c.Store.Get(ctx, chapter, versionID, true)
	if err != nil {
		return FinalizeResult{}, err
	}
	if candidate.Rejected {
		return FinalizeResult{}, newError(CodeRejected, "rejected versions cannot be finalized", false, nil)
	}
	if !candidate.Accepted {
		return FinalizeResult{}, newError(CodeFinalizeNotAllowed, "chapter version must be accepted before finalization", false, nil)
	}

	evaluation, ok, err := c.Store.latestEvaluation(ctx, candidate.ID)
	if err != nil {
		return FinalizeResult{}, err
	}
	if !ok {
		return FinalizeResult{}, newError(CodeFinalizeNotAllowed, "accepted chapter version has no persisted evaluation", false, nil)
	}
	if evaluation.Continuity.Status == qualitygate.ContinuityFail || evaluation.Continuity.Blocking {
		return FinalizeResult{}, newError(CodeContinuityBlocked, "continuity FAIL blocks finalization", false, nil)
	}

	authority := AuthorityGeneratedFinal
	truthAuthority := truthstore.AuthorityGeneratedFinal
	finalAuthor := AuthorSystem
	if isHumanCandidate(ctx, c.Store, candidate) {
		authority = AuthorityHumanFinal
		truthAuthority = truthstore.AuthorityHumanFinal
		finalAuthor = AuthorHuman
	}
	if authority == AuthorityGeneratedFinal {
		for _, conflict := range evaluation.Conflicts {
			if conflict.ExistingAuthority == truthstore.AuthorityHumanFinal && !jsonEqual(conflict.ExistingValue, conflict.ProposedValue) {
				return FinalizeResult{}, newError(CodeTruthConflict, "generated final cannot supersede Accepted Human Final Truth", false, nil)
			}
		}
	}

	digest := requestDigest("finalize", c.Store.projectID, candidate.ID, candidate.ContentSHA, authority)
	op, replay, err := c.Store.BeginOperation(ctx, key, "finalize", chapter, candidate.ID, digest)
	if err != nil {
		return FinalizeResult{}, err
	}
	if replay {
		if result, done, decodeErr := decodeReplay[FinalizeResult](op); decodeErr != nil {
			return FinalizeResult{}, decodeErr
		} else if done {
			return result, nil
		}
	}

	// A different idempotency key may legitimately retry an already completed
	// candidate. Reuse it only when the saga and checkpoint prove completion.
	// An Active Final by itself is not enough: a crash may have switched the
	// pointer before rebuild/checkpoint/operation completion, and that path must
	// resume rather than silently return partial success.
	if existing, ok, existingErr := c.Store.completedFinalForCandidate(ctx, candidate); existingErr != nil {
		return FinalizeResult{}, existingErr
	} else if ok {
		if err := c.Store.CompleteOperation(ctx, key, existing); err != nil {
			return FinalizeResult{}, err
		}
		return existing, nil
	}

	operationID := "fin_" + hashText(key + "\x00" + digest)[:28]
	if err := c.Store.ensureFinalizeSaga(ctx, operationID, chapter, candidate.ID, authority, mustJSON(evaluation.Proposal)); err != nil {
		return FinalizeResult{}, err
	}
	final, err := c.Store.ensureFinalVersion(ctx, operationID, candidate, finalAuthor, evaluation)
	if err != nil {
		_ = c.Store.FailOperation(ctx, key, errorCode(err))
		return FinalizeResult{}, err
	}
	if err := c.fail("after_version_ready"); err != nil {
		return FinalizeResult{}, err
	}

	truthEvents, err := c.commitTruth(ctx, operationID, final, evaluation.Proposal, truthAuthority)
	if err != nil {
		return FinalizeResult{}, err
	}
	if err := c.Store.advanceFinalizeSaga(ctx, operationID, "truth_committed", "running", ""); err != nil {
		return FinalizeResult{}, err
	}
	if err := c.fail("after_truth_commit"); err != nil {
		return FinalizeResult{}, err
	}
	if err := c.commitLedger(ctx, operationID, final, candidate, evaluation.Proposal, truthAuthority); err != nil {
		return FinalizeResult{}, err
	}
	if err := c.Store.advanceFinalizeSaga(ctx, operationID, "ledger_committed", "running", ""); err != nil {
		return FinalizeResult{}, err
	}
	if err := c.fail("after_ledger_commit"); err != nil {
		return FinalizeResult{}, err
	}

	if err := c.FinalWriter.WriteFinalChapter(ctx, c.Store.projectID, chapter, final.Content, final.ContentSHA); err != nil {
		return FinalizeResult{}, newError(CodeStorage, "final chapter file could not be switched", true, err)
	}
	if err := c.Store.advanceFinalizeSaga(ctx, operationID, "chapter_file_written", "running", ""); err != nil {
		return FinalizeResult{}, err
	}
	if err := c.fail("after_chapter_file_write"); err != nil {
		return FinalizeResult{}, err
	}
	if err := c.Store.SwitchActiveFinal(ctx, chapter, final.ID, authority); err != nil {
		return FinalizeResult{}, err
	}
	if err := c.Store.advanceFinalizeSaga(ctx, operationID, "active_final_switched", "running", ""); err != nil {
		return FinalizeResult{}, err
	}
	if err := c.fail("after_active_final_switch"); err != nil {
		return FinalizeResult{}, err
	}
	if err := c.updateContextDocument(ctx, final); err != nil {
		return FinalizeResult{}, err
	}

	rebuildStatus := "ready"
	if authority == AuthorityHumanFinal {
		rebuild, rebuildErr := c.RebuildDerivedState(ctx, operationID+":rebuild", final, evaluation)
		if rebuildErr != nil {
			return FinalizeResult{}, rebuildErr
		}
		rebuildStatus = rebuild.State
	}
	if err := c.Store.advanceFinalizeSaga(ctx, operationID, "derived_state_ready", "running", ""); err != nil {
		return FinalizeResult{}, err
	}
	if err := c.fail("before_checkpoint"); err != nil {
		return FinalizeResult{}, err
	}
	if err := c.Store.saveCheckpoint(ctx, operationID, final); err != nil {
		return FinalizeResult{}, err
	}
	if err := c.Store.advanceFinalizeSaga(ctx, operationID, "checkpointed", "running", ""); err != nil {
		return FinalizeResult{}, err
	}
	if err := c.fail("after_checkpoint"); err != nil {
		return FinalizeResult{}, err
	}
	if authority == AuthorityHumanFinal {
		if err := c.Store.ClearSyncRequired(ctx, chapter, final); err != nil {
			return FinalizeResult{}, err
		}
	}

	finalizePayload := map[string]any{}
	finalizePayload["operation_id"] = operationID
	finalizePayload["authority"] = authority
	if err := c.Store.AppendEvent(ctx, chapter, final.ID, "finalize", "accepted chapter version finalized", mustJSON(finalizePayload)); err != nil {
		return FinalizeResult{}, err
	}
	if err := c.Store.advanceFinalizeSaga(ctx, operationID, "completed", "completed", ""); err != nil {
		return FinalizeResult{}, err
	}
	final, err = c.Store.Get(ctx, chapter, final.ID, true)
	if err != nil {
		return FinalizeResult{}, err
	}
	result := FinalizeResult{}
	result.Version = final
	result.ActiveFinal = final
	result.OperationID = operationID
	result.TruthEvents = truthEvents
	result.RebuildStatus = rebuildStatus
	if err := c.Store.CompleteOperation(ctx, key, result); err != nil {
		return FinalizeResult{}, err
	}
	return result, nil
}

func (s *Store) completedFinalForCandidate(ctx context.Context, candidate Version) (FinalizeResult, bool, error) {
	var finalID string
	var operationID string
	var truthIDsJSON string
	err := s.db.QueryRowContext(ctx, `SELECT v.id,s.operation_id,s.truth_event_ids_json
		FROM chapter_active_finals a
		JOIN chapter_versions v ON v.id=a.version_id
		JOIN chapter_finalize_sagas s ON s.final_version_id=v.id
		JOIN chapter_version_checkpoints cp ON cp.operation_id=s.operation_id AND cp.version_id=v.id
		WHERE a.project_id=? AND a.chapter=? AND v.parent_version_id=?
		  AND v.content_sha=? AND s.candidate_version_id=? AND s.state='completed'
		ORDER BY s.updated_at DESC LIMIT 1`,
		s.projectID, candidate.Chapter, candidate.ID, candidate.ContentSHA, candidate.ID).Scan(&finalID, &operationID, &truthIDsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return FinalizeResult{}, false, nil
	}
	if err != nil {
		return FinalizeResult{}, false, newError(CodeStorage, "completed finalization could not be inspected", true, err)
	}
	final, err := s.Get(ctx, candidate.Chapter, finalID, true)
	if err != nil {
		return FinalizeResult{}, false, err
	}
	truthIDs := []string{}
	if strings.TrimSpace(truthIDsJSON) != "" {
		if err := json.Unmarshal([]byte(truthIDsJSON), &truthIDs); err != nil {
			return FinalizeResult{}, false, newError(CodeStorage, "completed finalization Truth evidence is invalid", false, err)
		}
	}
	result := FinalizeResult{}
	result.Version = final
	result.ActiveFinal = final
	result.OperationID = operationID
	result.TruthEvents = len(truthIDs)
	result.RebuildStatus = "ready"
	if final.Authority == AuthorityHumanFinal || isHumanCandidate(ctx, s, candidate) {
		if rebuild, ok, rebuildErr := s.getRebuildByOperation(ctx, operationID+":rebuild"); rebuildErr != nil {
			return FinalizeResult{}, false, rebuildErr
		} else if ok {
			result.RebuildStatus = rebuild.State
		}
	}
	return result, true, nil
}

func isHumanCandidate(ctx context.Context, store *Store, candidate Version) bool {
	if candidate.AuthorType == AuthorHuman || candidate.Type == TypeHumanRevision {
		return true
	}
	parent := candidate.ParentVersionID
	for depth := 0; depth < 8 && parent != ""; depth++ {
		version, err := store.Get(ctx, candidate.Chapter, parent, false)
		if err != nil {
			return false
		}
		if version.AuthorType == AuthorHuman || version.Type == TypeHumanRevision {
			return true
		}
		parent = version.ParentVersionID
	}
	return false
}

func (s *Store) ensureFinalizeSaga(ctx context.Context, operationID string, chapter int, candidateID, authority string, proposal json.RawMessage) error {
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `INSERT INTO chapter_finalize_sagas(operation_id,project_id,chapter,candidate_version_id,authority,state,current_step,proposal_json,created_at,updated_at)
		VALUES(?,?,?,?,?,'running','pending',?,?,?) ON CONFLICT(operation_id) DO NOTHING`, operationID, s.projectID, chapter, candidateID, authority, string(proposal), now, now)
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "unique") || strings.Contains(message, "locked") {
			return newError(CodeActiveFinalConflict, "another finalization is already in progress for this chapter", true, err)
		}
		return newError(CodeStorage, "finalization saga could not be started", true, err)
	}
	return nil
}

func (s *Store) advanceFinalizeSaga(ctx context.Context, operationID, step, state, code string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE chapter_finalize_sagas SET current_step=?,state=?,error_code=?,updated_at=? WHERE operation_id=?`,
		step, state, code, s.now().UTC().Format(time.RFC3339Nano), operationID)
	if err != nil {
		return newError(CodeStorage, "finalization saga could not advance", true, err)
	}
	return nil
}

func (s *Store) ensureFinalVersion(ctx context.Context, operationID string, candidate Version, author AuthorType, evaluation Evaluation) (Version, error) {
	var finalID string
	err := s.db.QueryRowContext(ctx, `SELECT final_version_id FROM chapter_finalize_sagas WHERE operation_id=?`, operationID).Scan(&finalID)
	if err != nil {
		return Version{}, newError(CodeStorage, "finalization saga could not be read", true, err)
	}
	if finalID != "" {
		return s.Get(ctx, candidate.Chapter, finalID, true)
	}
	matching, err := s.findMatching(ctx, candidate.Chapter, candidate.ID, TypeFinal, author, candidate.ContentSHA)
	if err != nil {
		return Version{}, err
	}
	var final Version
	if matching != nil {
		final = *matching
	} else {
		review, _ := json.Marshal(evaluation.Review)
		continuity, _ := json.Marshal(evaluation.Continuity)
		provenance := map[string]any{}
		provenance["finalized_from"] = candidate.ID
		provenance["proposal_id"] = evaluation.Proposal.ProposalID
		provenance["evaluation_at"] = evaluation.EvaluatedAt
		input := CreateInput{}
		input.Content = candidate.Content
		input.Type = TypeFinal
		input.ParentVersionID = candidate.ID
		input.AuthorType = author
		input.Review = review
		input.Continuity = continuity
		input.Provenance = mustJSON(provenance)
		final, err = s.Create(ctx, candidate.Chapter, input)
		if err != nil {
			return Version{}, err
		}
	}
	_, err = s.db.ExecContext(ctx, `UPDATE chapter_finalize_sagas SET final_version_id=?,current_step='version_ready',updated_at=? WHERE operation_id=?`,
		final.ID, s.now().UTC().Format(time.RFC3339Nano), operationID)
	if err != nil {
		return Version{}, newError(CodeStorage, "finalization saga could not record final version", true, err)
	}
	return final, nil
}

func (c *Coordinator) updateContextDocument(ctx context.Context, final Version) error {
	fts := contextcompiler.NewFTSStore(c.Store.db)
	document := contextcompiler.Document{}
	document.ID = fmt.Sprintf("chapter-final:%04d", final.Chapter)
	document.ProjectID = c.Store.projectID
	document.Kind = "chapter_final"
	document.Title = fmt.Sprintf("Chapter %d Final", final.Chapter)
	document.Content = final.Content
	document.SourceChapter = final.Chapter
	document.SourceVersion = final.ID
	document.Priority = 100
	document.CreatedAt = c.Store.now().UTC()
	if err := fts.Upsert(ctx, document); err != nil {
		return newError(CodeStorage, "Final chapter context document could not be updated", true, err)
	}
	return nil
}

func (s *Store) saveCheckpoint(ctx context.Context, operationID string, final Version) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO chapter_version_checkpoints(operation_id,project_id,chapter,version_id,final_sha,created_at) VALUES(?,?,?,?,?,?)`,
		operationID, s.projectID, final.Chapter, final.ID, final.ContentSHA, s.now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return newError(CodeStorage, "chapter version checkpoint could not be persisted", true, err)
	}
	return nil
}

func (c *Coordinator) fail(point string) error {
	if c.Faults == nil {
		return nil
	}
	return c.Faults.Fail(point)
}
