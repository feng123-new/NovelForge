package qualitygate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/voocel/ainovel-cli/internal/truthstore"
)

func (c *Coordinator) Rewrite(ctx context.Context, projectID string, chapter int, plan ChapterPlan) (Snapshot, error) {
	if err := c.validate(); err != nil {
		return Snapshot{}, err
	}
	if c.Writer == nil {
		return Snapshot{}, errors.New("writer service is not configured")
	}
	tx, err := c.Store.transactionByProjectChapter(ctx, projectID, chapter)
	if err != nil {
		return Snapshot{}, err
	}
	if tx.Attempt >= tx.MaxRewrites {
		return c.Store.Snapshot(ctx, projectID, chapter)
	}
	candidates, err := c.Store.Candidates(ctx, tx.ID)
	if err != nil || len(candidates) == 0 {
		return Snapshot{}, errors.New("rewrite requires an existing draft")
	}
	previous := candidates[len(candidates)-1]
	feedback := []string{}
	if continuity, err := c.Store.Continuity(ctx, tx.ID, previous.ID); err == nil {
		for _, issue := range continuity.Issues {
			feedback = append(feedback, issue.IssueCode+": "+issue.SuggestedAction)
		}
	}
	if review, err := c.Store.Editor(ctx, tx.ID, previous.ID); err == nil {
		feedback = append(feedback, review.Weaknesses...)
	}
	attempt := tx.Attempt + 1
	if tx.State != StateRewritePending {
		tx, err = c.Store.Transition(ctx, tx.ID, StateRewritePending, "explicit bounded rewrite requested", "policy", tx.Attempt)
		if err != nil {
			return Snapshot{}, err
		}
	}
	tx, err = c.Store.Transition(ctx, tx.ID, StateDrafting, "rewrite attempt", "writer", attempt)
	if err != nil {
		return Snapshot{}, err
	}
	result, err := c.Writer.Write(ctx, WriterRequest{ProjectID: projectID, Chapter: chapter, TransactionID: tx.ID, Attempt: attempt, Plan: plan, PreviousDraft: previous.Text, Feedback: feedback})
	if err != nil {
		_, _ = c.Store.Transition(ctx, tx.ID, StateHold, "rewrite failed; previous drafts retained", "writer", attempt)
		return c.Store.Snapshot(ctx, projectID, chapter)
	}
	candidate, err := c.Store.SaveCandidate(ctx, tx.ID, result.Text, result.SourceVersion, attempt)
	if err != nil {
		_, _ = c.Store.Transition(ctx, tx.ID, StateHold, "rewrite persistence failed; previous drafts retained", "coordinator", attempt)
		return Snapshot{}, err
	}
	if _, err := c.Store.Transition(ctx, tx.ID, StateDraftReady, "rewrite draft persisted: "+candidate.ID, "writer", attempt); err != nil {
		return Snapshot{}, err
	}
	return c.Store.Snapshot(ctx, projectID, chapter)
}

func (c *Coordinator) Finalize(ctx context.Context, projectID string, chapter int, idempotencyKey string) (Snapshot, error) {
	if err := c.validate(); err != nil {
		return Snapshot{}, err
	}
	if c.Truth == nil || c.FinalWriter == nil {
		return Snapshot{}, errors.New("truth repository and final chapter writer are required")
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return Snapshot{}, errors.New("finalize idempotency key is required")
	}
	lock := c.chapterLock(projectID, chapter)
	lock.Lock()
	defer lock.Unlock()

	tx, err := c.Store.transactionByProjectChapter(ctx, projectID, chapter)
	if err != nil {
		return Snapshot{}, err
	}
	if tx.State == StateCompleted {
		return c.Store.Snapshot(ctx, projectID, chapter)
	}
	if tx.FinalCandidateID == "" {
		best, reason, bestErr := c.Store.BestSafeCandidate(ctx, tx.ID, c.policy())
		if bestErr != nil {
			_, _ = c.Store.Transition(ctx, tx.ID, StateHold, "finalize refused: no continuity-safe candidate", "policy", tx.Attempt)
			return c.Store.Snapshot(ctx, projectID, chapter)
		}
		if err := c.Store.SelectFinal(ctx, tx.ID, best.ID, reason); err != nil {
			return Snapshot{}, err
		}
		tx.FinalCandidateID = best.ID
		if tx.State != StateFinalCandidate {
			tx, err = c.Store.Transition(ctx, tx.ID, StateFinalCandidate, reason, "policy", tx.Attempt)
			if err != nil {
				return Snapshot{}, err
			}
		}
	}
	candidate, err := c.Store.Candidate(ctx, tx.FinalCandidateID)
	if err != nil {
		return Snapshot{}, err
	}
	continuity, err := c.Store.Continuity(ctx, tx.ID, candidate.ID)
	if err != nil || continuity.Status == ContinuityFail || !c.policy().Allows(continuity) {
		_, _ = c.Store.Transition(ctx, tx.ID, StateHold, "continuity policy blocks finalization", "policy", tx.Attempt)
		return c.Store.Snapshot(ctx, projectID, chapter)
	}
	proposal, err := c.Store.Proposal(ctx, tx.ID, candidate.ID)
	if err != nil {
		return Snapshot{}, err
	}
	if tx.State == StateFinalCandidate || tx.State == StateHold {
		tx, err = c.Store.Transition(ctx, tx.ID, StateTruthCommitPending, "accepted Final candidate committing Truth", "coordinator", tx.Attempt)
		if err != nil {
			return Snapshot{}, err
		}
	}
	for index, change := range proposal.AllChanges() {
		subjectType, subjectID := splitSubject(change.Subject)
		authority := truthstore.AuthorityGeneratedFinal
		result, appendErr := c.Truth.Append(ctx, truthstore.AppendInput{
			IdempotencyKey:   fmt.Sprintf("%s:truth:%s:%d", idempotencyKey, proposal.ProposalID, index),
			Kind:             truthstore.EventAssert,
			SubjectType:      subjectType,
			SubjectID:        subjectID,
			Predicate:        change.Predicate,
			Value:            append(json.RawMessage(nil), change.Object...),
			ValidFromChapter: change.ValidFromChapter,
			ValidToChapter:   change.ValidToChapter,
			KnownFromChapter: change.KnownFromChapter,
			KnownToChapter:   change.KnownToChapter,
			Authority:        authority,
			Confidence:       change.Confidence,
			Source: truthstore.Source{
				Type:      "chapter_final",
				ID:        candidate.ID,
				Chapter:   chapter,
				Version:   candidate.SourceVersion,
				Extractor: change.Extractor,
			},
		})
		if appendErr != nil {
			return c.Store.Snapshot(ctx, projectID, chapter)
		}
		if err := c.Store.SaveTruthCommit(ctx, tx.ID, proposal.ProposalID, index, result.Event.ID); err != nil {
			return Snapshot{}, err
		}
	}
	if err := c.fail("after_truth_commit"); err != nil {
		return Snapshot{}, err
	}
	if err := c.FinalWriter.WriteFinalChapter(ctx, projectID, chapter, candidate.Text, candidate.TextSHA); err != nil {
		return c.Store.Snapshot(ctx, projectID, chapter)
	}
	if err := c.fail("after_file_switch"); err != nil {
		return Snapshot{}, err
	}
	if tx.State != StateCheckpointPending {
		tx, err = c.Store.Transition(ctx, tx.ID, StateCheckpointPending, "Truth and chapter file committed; checkpoint pending", "coordinator", tx.Attempt)
		if err != nil {
			return Snapshot{}, err
		}
	}
	if err := c.Store.SaveCheckpoint(ctx, tx.ID, candidate.ID, candidate.TextSHA); err != nil {
		return c.Store.Snapshot(ctx, projectID, chapter)
	}
	if err := c.fail("after_checkpoint"); err != nil {
		return Snapshot{}, err
	}
	if _, err := c.Store.Transition(ctx, tx.ID, StateCompleted, "Final, Truth and checkpoint committed", "coordinator", tx.Attempt); err != nil {
		return Snapshot{}, err
	}
	return c.Store.Snapshot(ctx, projectID, chapter)
}

func (c *Coordinator) fail(point string) error {
	if c.Faults == nil {
		return nil
	}
	return c.Faults.Fail(point)
}

func (c *Coordinator) chapterLock(projectID string, chapter int) *sync.Mutex {
	key := fmt.Sprintf("%s:%d", projectID, chapter)
	value, _ := c.locks.LoadOrStore(key, &sync.Mutex{})
	return value.(*sync.Mutex)
}
