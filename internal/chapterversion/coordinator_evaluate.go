package chapterversion

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/voocel/ainovel-cli/internal/qualitygate"
	"github.com/voocel/ainovel-cli/internal/truthstore"
)

func (c *Coordinator) evaluate(ctx context.Context, version Version) (Evaluation, error) {
	if err := c.validate(); err != nil {
		return Evaluation{}, err
	}
	if c.Librarian == nil || c.Continuity == nil || c.Truth == nil {
		return Evaluation{}, newError(CodeFinalizeNotAllowed, "Librarian, Continuity and Truth services are required", false, nil)
	}
	candidate := qualitygate.Candidate{}
	candidate.ID = version.ID
	candidate.TransactionID = "phase8:" + version.ID
	candidate.Chapter = version.Chapter
	candidate.Text = version.Content
	candidate.TextSHA = version.ContentSHA
	candidate.SourceVersion = version.ID
	candidate.CreatedAt = version.CreatedAt

	request := qualitygate.LibrarianRequest{}
	request.ProjectID = c.Store.projectID
	request.Chapter = version.Chapter
	request.TransactionID = candidate.TransactionID
	request.Candidate = candidate
	proposal, err := c.Librarian.Propose(ctx, request)
	if err != nil {
		return Evaluation{}, newError(CodeFinalizeNotAllowed, "Librarian fact proposal failed; human revision was retained", true, err)
	}
	if err := proposal.Validate(); err != nil {
		return Evaluation{}, newError(CodeFinalizeNotAllowed, "Librarian fact proposal is invalid; human revision was retained", false, err)
	}

	continuityRequest := qualitygate.ContinuityRequest{}
	continuityRequest.ProjectID = c.Store.projectID
	continuityRequest.Chapter = version.Chapter
	continuityRequest.TransactionID = candidate.TransactionID
	continuityRequest.Candidate = candidate
	continuityRequest.Proposal = proposal
	continuity, err := c.Continuity.Check(ctx, continuityRequest)
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
		editorRequest := qualitygate.EditorRequest{}
		editorRequest.ProjectID = c.Store.projectID
		editorRequest.Chapter = version.Chapter
		editorRequest.TransactionID = candidate.TransactionID
		editorRequest.Candidate = candidate
		editorRequest.Continuity = continuity
		value, reviewErr := c.Editor.Review(ctx, editorRequest)
		if reviewErr != nil {
			return Evaluation{}, newError(CodeFinalizeNotAllowed, "Editor review failed; human revision was retained", true, reviewErr)
		}
		if err := value.Validate(); err != nil {
			return Evaluation{}, newError(CodeFinalizeNotAllowed, "Editor review is invalid; human revision was retained", false, err)
		}
		review = &value
	}

	result := Evaluation{}
	result.Proposal = proposal
	result.Continuity = continuity
	result.Review = review
	result.Conflicts = conflicts
	result.EvaluatedAt = c.Store.now().UTC()
	return result, nil
}

func (c *Coordinator) proposalConflicts(ctx context.Context, chapter int, proposal qualitygate.FactProposal) ([]CandidateConflict, error) {
	conflicts := []CandidateConflict{}
	for _, change := range proposal.AllChanges() {
		subjectType, subjectID := splitSubject(change.Subject)
		query := truthstore.StateQuery{}
		query.Chapter = chapter
		query.SubjectType = subjectType
		query.SubjectID = subjectID
		query.Predicate = change.Predicate
		query.Limit = 100
		page, err := c.Truth.State(ctx, query)
		if err != nil {
			return nil, newError(CodeStorage, "Truth conflict check could not read current state", true, err)
		}
		for _, fact := range page.Facts {
			if jsonEqual(fact.Value, change.Object) {
				continue
			}
			conflict := CandidateConflict{}
			conflict.SubjectType = subjectType
			conflict.SubjectID = subjectID
			conflict.Predicate = change.Predicate
			conflict.ExistingEventID = fact.ID
			conflict.ExistingValue = append(json.RawMessage(nil), fact.Value...)
			conflict.ExistingAuthority = fact.Authority
			conflict.ProposedValue = append(json.RawMessage(nil), change.Object...)
			conflict.Blocking = fact.Authority == truthstore.AuthorityHumanFinal
			conflict.Reason = "candidate differs from current projected fact and requires explicit supersede"
			if conflict.Blocking {
				conflict.Reason = "candidate differs from an existing Accepted Human Final fact"
			}
			conflicts = append(conflicts, conflict)
		}
	}
	sort.Slice(conflicts, func(i, j int) bool {
		left := conflicts[i].SubjectType + "\x00" + conflicts[i].SubjectID + "\x00" + conflicts[i].Predicate + "\x00" + conflicts[i].ExistingEventID
		right := conflicts[j].SubjectType + "\x00" + conflicts[j].SubjectID + "\x00" + conflicts[j].Predicate + "\x00" + conflicts[j].ExistingEventID
		return left < right
	})
	return conflicts, nil
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
	err := s.db.QueryRowContext(ctx, `SELECT payload_json FROM chapter_version_events WHERE version_id=? AND event_type IN ('accept','sync_completed','evaluation_completed') ORDER BY sequence DESC LIMIT 1`, versionID).Scan(&payload)
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
