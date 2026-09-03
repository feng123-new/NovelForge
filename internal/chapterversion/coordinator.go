package chapterversion

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/contextcompiler"
	"github.com/voocel/ainovel-cli/internal/narrativeledger"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
	"github.com/voocel/ainovel-cli/internal/truthstore"
)

type FinalChapterWriter interface {
	WriteFinalChapter(context.Context, string, int, string, string) error
}

type FaultInjector interface {
	Fail(string) error
}

type CandidateConflict struct {
	SubjectType       string               `json:"subject_type"`
	SubjectID         string               `json:"subject_id"`
	Predicate         string               `json:"predicate"`
	ExistingEventID   string               `json:"existing_event_id"`
	ExistingValue     json.RawMessage      `json:"existing_value"`
	ExistingAuthority truthstore.Authority `json:"existing_authority"`
	ProposedValue     json.RawMessage      `json:"proposed_value"`
	Blocking          bool                 `json:"blocking"`
	Reason            string               `json:"reason"`
}

type Evaluation struct {
	Proposal    qualitygate.FactProposal      `json:"proposal"`
	Continuity qualitygate.ContinuityResult  `json:"continuity"`
	Review     *qualitygate.EditorReview     `json:"review,omitempty"`
	Conflicts  []CandidateConflict           `json:"conflicts"`
	EvaluatedAt time.Time                    `json:"evaluated_at"`
}

type Coordinator struct {
	Store       *Store
	Truth       truthstore.Repository
	Ledger      *narrativeledger.Store
	Librarian   qualitygate.LibrarianService
	Continuity  qualitygate.ContinuityService
	Editor      qualitygate.EditorService
	FinalWriter FinalChapterWriter
	Faults      FaultInjector
}

func (c *Coordinator) validate() error {
	if c == nil || c.Store == nil {
		return newError(CodeStorage, "chapter version coordinator is not configured", false, nil)
	}
	return nil
}

func (c *Coordinator) evaluate(ctx context.Context, version Version) (Evaluation, error) {
	if err := c.validate(); err != nil {
		return Evaluation{}, err
	}
	if c.Librarian == nil || c.Continuity == nil || c.Truth == nil {
		return Evaluation{}, newError(CodeFinalizeNotAllowed, "Librarian, Continuity and Truth services are required", false, nil)
	}
	candidate := qualitygate.Candidate{
		ID:            version.ID,
		TransactionID: "phase8:" + version.ID,
		Chapter:       version.Chapter,
		Attempt:       0,
		Text:          version.Content,
		TextSHA:       version.ContentSHA,
		SourceVersion: version.ID,
		CreatedAt:     version.CreatedAt,
	}
	proposal, err := c.Librarian.Propose(ctx, qualitygate.LibrarianRequest{
		ProjectID: c.Store.projectID, Chapter: version.Chapter, TransactionID: candidate.TransactionID, Candidate: candidate,
	})
	if err != nil {
		return Evaluation{}, newError(CodeFinalizeNotAllowed, "Librarian fact proposal failed; human revision was retained", true, err)
	}
	if err := proposal.Validate(); err != nil {
		return Evaluation{}, newError(CodeFinalizeNotAllowed, "Librarian fact proposal is invalid; human revision was retained", false, err)
	}
	continuity, err := c.Continuity.Check(ctx, qualitygate.ContinuityRequest{
		ProjectID: c.Store.projectID, Chapter: version.Chapter, TransactionID: candidate.TransactionID, Candidate: candidate, Proposal: proposal,
	})
	if err != nil {
		return Evaluation{}, newError(CodeContinuityBlocked, "Continuity check failed; human revision was retained", true, err)
	}
	if err := continuity.Validate(); err != nil {
		return Evaluation{}, newError(CodeContinuityBlocked, "Continuity result is invalid; human revision was retained", false, err)
	}
	conflicts, err := c.proposalConflicts(ctx, version.Chapter, proposal)
	if err != nil {
		return Evaluation{}, err
	}
	var review *qualitygate.EditorReview
	if c.Editor != nil && continuity.Status != qualitygate.ContinuityFail {
		value, reviewErr := c.Editor.Review(ctx, qualitygate.EditorRequest{
			ProjectID: c.Store.projectID, Chapter: version.Chapter, TransactionID: candidate.TransactionID, Candidate: candidate, Continuity: continuity,
		})
		if reviewErr != nil {
			return Evaluation{}, newError(CodeFinalizeNotAllowed, "Editor review failed; human revision was retained", true, reviewErr)
		}
		if err := value.Validate(); err != nil {
			return Evaluation{}, newError(CodeFinalizeNotAllowed, "Editor review is invalid; human revision was retained", false, err)
		}
		review = &value
	}
	return Evaluation{Proposal: proposal, Continuity: continuity, Review: review, Conflicts: conflicts, EvaluatedAt: c.Store.now().UTC()}, nil
}

func (c *Coordinator) proposalConflicts(ctx context.Context, chapter int, proposal qualitygate.FactProposal) ([]CandidateConflict, error) {
	conflicts := []CandidateConflict{}
	for _, change := range proposal.AllChanges() {
		subjectType, subjectID := splitSubject(change.Subject)
		page, err := c.Truth.State(ctx, truthstore.StateQuery{Chapter: chapter, SubjectType: subjectType, SubjectID: subjectID, Predicate: change.Predicate, Limit: 100})
		if err != nil {
			return nil, newError(CodeStorage, "Truth conflict check could not read current state", true, err)
		}
		for _, fact := range page.Facts {
			if jsonEqual(fact.Value, change.Object) {
				continue
			}
			blocking := fact.Authority == truthstore.AuthorityHumanFinal
			reason := "candidate differs from current projected fact and requires explicit supersede"
			if blocking {
				reason = "candidate differs from an existing Accepted Human Final fact"
			}
			conflicts = append(conflicts, CandidateConflict{
				SubjectType: subjectType, SubjectID: subjectID, Predicate: change.Predicate,
				ExistingEventID: fact.ID, ExistingValue: append(json.RawMessage(nil), fact.Value...), ExistingAuthority: fact.Authority,
				ProposedValue: append(json.RawMessage(nil), change.Object...), Blocking: blocking, Reason: reason,
			})
		}
	}
	sort.Slice(conflicts, func(i, j int) bool {
		left := conflicts[i].SubjectType + "\x00" + conflicts[i].SubjectID + "\x00" + conflicts[i].Predicate + "\x00" + conflicts[i].ExistingEventID
		right := conflicts[j].SubjectType + "\x00" + conflicts[j].SubjectID + "\x00" + conflicts[j].Predicate + "\x00" + conflicts[j].ExistingEventID
		return left < right
	})
	return conflicts, nil
}

func jsonEqual(left, right json.RawMessage) bool {
	var a, b any
	if json.Unmarshal(left, &a) != nil || json.Unmarshal(right, &b) != nil {
		return bytes.Equal(bytes.TrimSpace(left), bytes.TrimSpace(right))
	}
	la, _ := json.Marshal(a)
	lb, _ := json.Marshal(b)
	return bytes.Equal(la, lb)
}

func splitSubject(subject string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(subject), ":", 2)
	if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "entity", strings.TrimSpace(subject)
}

func (c *Coordinator) SyncExternal(ctx context.Context, chapter int, key, detectedSHA string) (SyncResult, error) {
	if err := c.validate(); err != nil {
		return SyncResult{}, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return SyncResult{}, newError(CodeValidation, "Idempotency-Key is required", false, nil)
	}
	status, err := c.Store.DetectExternal(ctx, chapter)
	if err != nil {
		return SyncResult{}, err
	}
	if !status.SyncRequired {
		return SyncResult{}, newError(CodeSyncRequired, "chapter file matches the active final; no sync is required", false, nil)
	}
	if detectedSHA != "" && detectedSHA != status.ObservedSHA {
		return SyncResult{}, newError(CodeSyncContentChanged, "chapter file changed after the displayed sync status", false, nil)
	}
	content, observed, err := c.Store.readExternal(chapter)
	if err != nil {
		return SyncResult{}, err
	}
	if observed != status.ObservedSHA {
		return SyncResult{}, newError(CodeSyncContentChanged, "chapter file changed while sync was starting", false, nil)
	}
	digest := requestDigest("external_sync", c.Store.projectID, strconv.Itoa(chapter), status.ActiveVersionID, status.ExpectedSHA, observed)
	op, replay, err := c.Store.BeginOperation(ctx, key, "external_sync", chapter, status.ActiveVersionID, digest)
	if err != nil {
		return SyncResult{}, err
	}
	if replay {
		if result, ok, decodeErr := decodeReplay[SyncResult](op); decodeErr != nil {
			return SyncResult{}, decodeErr
		} else if ok {
			return result, nil
		}
	}
	active, err := c.Store.ActiveFinal(ctx, chapter, false)
	if err != nil || active == nil || active.ID != status.ActiveVersionID || active.ContentSHA != status.ExpectedSHA {
		return SyncResult{}, newError(CodeSyncContentChanged, "active final changed while sync was starting", false, err)
	}
	if err := c.Store.AppendEvent(ctx, chapter, active.ID, "sync_started", "explicit external chapter synchronization started", mustJSON(map[string]string{"expected_sha": status.ExpectedSHA, "observed_sha": observed})); err != nil {
		return SyncResult{}, err
	}
	matching, err := c.Store.findMatching(ctx, chapter, active.ID, TypeHumanRevision, AuthorHuman, observed)
	if err != nil {
		return SyncResult{}, err
	}
	var human Version
	if matching != nil {
		human = *matching
	} else {
		provenance := mustJSON(map[string]any{"source": "external_file_sync", "original_sha": status.ExpectedSHA, "observed_sha": observed, "parent_final": active.ID})
		human, err = c.Store.Create(ctx, chapter, CreateInput{Content: content, Type: TypeHumanRevision, ParentVersionID: active.ID, AuthorType: AuthorHuman, Provenance: provenance})
		if err != nil {
			_ = c.Store.FailOperation(ctx, key, errorCode(err))
			return SyncResult{}, err
		}
	}
	evaluation, err := c.evaluate(ctx, human)
	if err != nil {
		_ = c.Store.FailOperation(ctx, key, errorCode(err))
		return SyncResult{Version: human, SyncRequired: true}, err
	}
	payload := mustJSON(evaluation)
	if err := c.Store.AppendEvent(ctx, chapter, human.ID, "sync_completed", "external chapter content evaluated and retained as human_revision", payload); err != nil {
		return SyncResult{}, err
	}
	proposalJSON, _ := json.Marshal(evaluation.Proposal)
	continuityJSON, _ := json.Marshal(evaluation.Continuity)
	var reviewJSON json.RawMessage
	if evaluation.Review != nil {
		reviewJSON, _ = json.Marshal(evaluation.Review)
	}
	result := SyncResult{Version: human, Proposal: proposalJSON, Continuity: continuityJSON, Review: reviewJSON, Conflicts: len(evaluation.Conflicts), SyncRequired: true}
	if err := c.Store.CompleteOperation(ctx, key, result); err != nil {
		return SyncResult{}, err
	}
	return result, nil
}

func (c *Coordinator) Accept(ctx context.Context, chapter int, key, versionID, reason string) (Version, error) {
	if err := c.validate(); err != nil {
		return Version{}, err
	}
	version, err := c.Store.Get(ctx, chapter, versionID, true)
	if err != nil {
		return Version{}, err
	}
	if version.Rejected {
		return Version{}, newError(CodeRejected, "rejected versions cannot be accepted", false, nil)
	}
	if version.Accepted {
		return version, nil
	}
	digest := requestDigest("accept", c.Store.projectID, version.ID, version.ContentSHA, strings.TrimSpace(reason))
	op, replay, err := c.Store.BeginOperation(ctx, key, "accept", chapter, version.ID, digest)
	if err != nil {
		return Version{}, err
	}
	if replay {
		if result, ok, decodeErr := decodeReplay[Version](op); decodeErr != nil {
			return Version{}, decodeErr
		} else if ok {
			return result, nil
		}
	}
	evaluation, ok, err := c.Store.latestEvaluation(ctx, version.ID)
	if err != nil {
		return Version{}, err
	}
	if !ok {
		evaluation, err = c.evaluate(ctx, version)
		if err != nil {
			_ = c.Store.FailOperation(ctx, key, errorCode(err))
			return Version{}, err
		}
	}
	if evaluation.Continuity.Status == qualitygate.ContinuityFail || evaluation.Continuity.Blocking {
		return Version{}, newError(CodeContinuityBlocked, "continuity FAIL blocks acceptance", false, nil)
	}
	// A Human Final may explicitly supersede lower or equal authority facts on
	// finalization. The conflict evidence is retained here rather than erased.
	if err := c.Store.AppendEvent(ctx, chapter, version.ID, "accept", reason, mustJSON(evaluation)); err != nil {
		return Version{}, err
	}
	version, err = c.Store.Get(ctx, chapter, version.ID, true)
	if err != nil {
		return Version{}, err
	}
	if err := c.Store.CompleteOperation(ctx, key, version); err != nil {
		return Version{}, err
	}
	return version, nil
}

func (s *Store) latestEvaluation(ctx context.Context, versionID string) (Evaluation, bool, error) {
	var payload string
	err := s.db.QueryRowContext(ctx, `SELECT payload_json FROM chapter_version_events WHERE version_id=? AND event_type IN ('accept','sync_completed') ORDER BY sequence DESC LIMIT 1`, versionID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return Evaluation{}, false, nil
	}
	if err != nil {
		return Evaluation{}, false, newError(CodeStorage, "chapter version evaluation could not be read", true, err)
	}
	var evaluation Evaluation
	if err := json.Unmarshal([]byte(payload), &evaluation); err != nil {
		return Evaluation{}, false, newError(CodeStorage, "chapter version evaluation is invalid", false, err)
	}
	if evaluation.Proposal.ProposalID == "" || evaluation.Continuity.Status == "" {
		return Evaluation{}, false, nil
	}
	return evaluation, true, nil
}

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
	operationID := "fin_" + hashText(key+"\x00"+digest)[:28]
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
	truthEvents, err := c.commitTruth(ctx, operationID, final, candidate, evaluation.Proposal, truthAuthority)
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
	if err := c.Store.AppendEvent(ctx, chapter, final.ID, "finalize", "accepted chapter version finalized", mustJSON(map[string]any{"operation_id": operationID, "authority": authority})); err != nil {
		return FinalizeResult{}, err
	}
	if err := c.Store.advanceFinalizeSaga(ctx, operationID, "completed", "completed", ""); err != nil {
		return FinalizeResult{}, err
	}
	final, err = c.Store.Get(ctx, chapter, final.ID, true)
	if err != nil {
		return FinalizeResult{}, err
	}
	result := FinalizeResult{Version: final, ActiveFinal: final, OperationID: operationID, TruthEvents: truthEvents, RebuildStatus: rebuildStatus}
	if err := c.Store.CompleteOperation(ctx, key, result); err != nil {
		return FinalizeResult{}, err
	}
	return result, nil
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
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "locked") {
			return newError(CodeActiveFinalConflict, "another finalization is already in progress for this chapter", true, err)
		}
		return newError(CodeStorage, "finalization saga could not be started", true, err)
	}
	return nil
}

func (s *Store) advanceFinalizeSaga(ctx context.Context, operationID, step, state, code string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE chapter_finalize_sagas SET current_step=?,state=?,error_code=?,updated_at=? WHERE operation_id=?`, step, state, code, s.now().UTC().Format(time.RFC3339Nano), operationID)
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
		provenance := mustJSON(map[string]any{"finalized_from": candidate.ID, "proposal_id": evaluation.Proposal.ProposalID, "evaluation_at": evaluation.EvaluatedAt})
		final, err = s.Create(ctx, candidate.Chapter, CreateInput{Content: candidate.Content, Type: TypeFinal, ParentVersionID: candidate.ID, AuthorType: author, Review: review, Continuity: continuity, Provenance: provenance})
		if err != nil {
			return Version{}, err
		}
	}
	_, err = s.db.ExecContext(ctx, `UPDATE chapter_finalize_sagas SET final_version_id=?,current_step='version_ready',updated_at=? WHERE operation_id=?`, final.ID, s.now().UTC().Format(time.RFC3339Nano), operationID)
	if err != nil {
		return Version{}, newError(CodeStorage, "finalization saga could not record final version", true, err)
	}
	return final, nil
}

func (c *Coordinator) commitTruth(ctx context.Context, operationID string, final, candidate Version, proposal qualitygate.FactProposal, authority truthstore.Authority) (int, error) {
	if final.Type != TypeFinal {
		return 0, newError(CodeFinalizeNotAllowed, "only an immutable Final ChapterVersion may submit Truth", false, nil)
	}
	ids := []string{}
	for index, change := range proposal.AllChanges() {
		subjectType, subjectID := splitSubject(change.Subject)
		state, err := c.Truth.State(ctx, truthstore.StateQuery{Chapter: final.Chapter, SubjectType: subjectType, SubjectID: subjectID, Predicate: change.Predicate, Limit: 100})
		if err != nil {
			return 0, newError(CodeStorage, "current Truth could not be read before commit", true, err)
		}
		conflicting := []truthstore.Fact{}
		for _, fact := range state.Facts {
			if !jsonEqual(fact.Value, change.Object) {
				conflicting = append(conflicting, fact)
			}
		}
		if len(conflicting) == 0 {
			result, err := c.Truth.Append(ctx, truthstore.AppendInput{
				IdempotencyKey: safeTruthKey(operationID, index, "assert"), Kind: truthstore.EventAssert,
				SubjectType: subjectType, SubjectID: subjectID, Predicate: change.Predicate, Value: append(json.RawMessage(nil), change.Object...),
				ValidFromChapter: change.ValidFromChapter, ValidToChapter: change.ValidToChapter, KnownFromChapter: change.KnownFromChapter, KnownToChapter: change.KnownToChapter,
				Authority: authority, Confidence: change.Confidence,
				Source: truthstore.Source{Type: truthSourceType(authority), ID: final.ID, Chapter: final.Chapter, Version: final.ID, Extractor: change.Extractor, ConfirmedBy: confirmedBy(authority)},
			})
			if err != nil {
				return 0, newError(CodeTruthConflict, "Truth commit failed", true, err)
			}
			ids = append(ids, result.Event.ID)
			continue
		}
		if authority != truthstore.AuthorityHumanFinal {
			return 0, newError(CodeTruthConflict, "generated final conflicts with current Truth", false, nil)
		}
		for conflictIndex, fact := range conflicting {
			result, err := c.Truth.Append(ctx, truthstore.AppendInput{
				IdempotencyKey: safeTruthKey(operationID, index, "supersede"+strconv.Itoa(conflictIndex)), Kind: truthstore.EventSupersede,
				SubjectType: subjectType, SubjectID: subjectID, Predicate: change.Predicate, Value: append(json.RawMessage(nil), change.Object...),
				ValidFromChapter: change.ValidFromChapter, ValidToChapter: change.ValidToChapter, KnownFromChapter: change.KnownFromChapter, KnownToChapter: change.KnownToChapter,
				Authority: authority, Confidence: change.Confidence, SupersedesEventID: fact.ID,
				Source: truthstore.Source{Type: "chapter_human_final", ID: final.ID, Chapter: final.Chapter, Version: final.ID, Extractor: change.Extractor, ConfirmedBy: "human"},
			})
			if err != nil {
				return 0, newError(CodeTruthConflict, "Accepted Human Final could not supersede conflicting Truth", false, err)
			}
			ids = append(ids, result.Event.ID)
		}
	}
	encoded, _ := json.Marshal(ids)
	_, err := c.Store.db.ExecContext(ctx, `UPDATE chapter_finalize_sagas SET truth_event_ids_json=?,updated_at=? WHERE operation_id=?`, string(encoded), c.Store.now().UTC().Format(time.RFC3339Nano), operationID)
	if err != nil {
		return 0, newError(CodeStorage, "Truth commit evidence could not be recorded", true, err)
	}
	return len(ids), nil
}

func safeTruthKey(operationID string, index int, suffix string) string {
	return "p8:" + hashText(operationID+":"+strconv.Itoa(index)+":"+suffix)[:48]
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

func (c *Coordinator) commitLedger(ctx context.Context, operationID string, final, candidate Version, proposal qualitygate.FactProposal, authority truthstore.Authority) error {
	changes := func(values []qualitygate.FactChange) []narrativeledger.AcceptedChange {
		out := make([]narrativeledger.AcceptedChange, 0, len(values))
		for _, change := range values {
			out = append(out, narrativeledger.AcceptedChange{
				Subject: change.Subject, Predicate: change.Predicate, Object: append(json.RawMessage(nil), change.Object...),
				SourceChapter: change.SourceChapter, SourceVersion: change.SourceVersion, SourceSHA: change.SourceSHA, Extractor: change.Extractor,
				Confidence: change.Confidence, Authority: authority, ValidFromChapter: change.ValidFromChapter, ValidToChapter: change.ValidToChapter,
				KnownFromChapter: change.KnownFromChapter, KnownToChapter: change.KnownToChapter, Reason: change.Reason,
			})
		}
		return out
	}
	_, err := c.Ledger.CommitAcceptedFinal(ctx, narrativeledger.AcceptedFinalInput{
		ProjectID: c.Store.projectID, TransactionID: operationID, ProposalID: proposal.ProposalID, CandidateID: final.ID,
		Chapter: final.Chapter, SourceVersion: candidate.ID, IdempotencyKey: operationID + ":ledger",
		ForeshadowUpdates: changes(proposal.ForeshadowUpdates), Secrets: changes(proposal.Secrets),
	})
	if err != nil {
		return newError(CodeStorage, "Narrative Ledger commit failed", true, err)
	}
	if authority == truthstore.AuthorityHumanFinal {
		_, err = c.Ledger.PromoteAcceptedFinalAuthority(ctx, narrativeledger.HumanAuthorityPromotion{
			ProjectID: c.Store.projectID, TransactionID: operationID, CandidateID: final.ID, Chapter: final.Chapter,
			SourceVersion: candidate.ID, IdempotencyKey: operationID + ":human-authority",
		})
		if err != nil {
			return newError(CodeStorage, "Narrative Ledger Human Final authority promotion failed", true, err)
		}
	}
	return nil
}

func (c *Coordinator) updateContextDocument(ctx context.Context, final Version) error {
	fts := contextcompiler.NewFTSStore(c.Store.db)
	err := fts.Upsert(ctx, contextcompiler.Document{
		ID: fmt.Sprintf("chapter-final:%04d", final.Chapter), ProjectID: c.Store.projectID, Kind: "chapter_final",
		Title: fmt.Sprintf("Chapter %d Final", final.Chapter), Content: final.Content, SourceChapter: final.Chapter,
		SourceVersion: final.ID, Priority: 100, CreatedAt: c.Store.now().UTC(),
	})
	if err != nil {
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

func mustJSON(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
