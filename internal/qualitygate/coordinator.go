package qualitygate

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/voocel/ainovel-cli/internal/narrativeledger"
	"github.com/voocel/ainovel-cli/internal/truthstore"
)

type FinalChapterWriter interface {
	WriteFinalChapter(context.Context, string, int, string, string) error
}

type FaultInjector interface {
	Fail(string) error
}

type Coordinator struct {
	Store       *Store
	Truth       truthstore.Repository
	Ledger      narrativeledger.AcceptedFinalCommitter
	Writer      WriterService
	Librarian   LibrarianService
	Continuity  ContinuityService
	Editor      EditorService
	Policy      Policy
	FinalWriter FinalChapterWriter
	Faults      FaultInjector

	locks sync.Map
}

func (c *Coordinator) policy() Policy {
	p := c.Policy
	if p.MaxRewrites == 0 && p.QualityThreshold == 0 {
		p = DefaultPolicy()
	}
	return p
}

func (c *Coordinator) validate() error {
	if c.Store == nil {
		return errors.New("quality store is required")
	}
	if err := c.policy().Validate(); err != nil {
		return err
	}
	return nil
}

func (c *Coordinator) Generate(ctx context.Context, projectID string, chapter int, plan ChapterPlan) (Snapshot, error) {
	if err := c.validate(); err != nil {
		return Snapshot{}, err
	}
	if c.Writer == nil {
		return Snapshot{}, errors.New("writer service is not configured")
	}
	tx, _, err := c.Store.Begin(ctx, projectID, chapter, c.policy())
	if err != nil {
		return Snapshot{}, err
	}
	if tx.State == StateCompleted || tx.State == StateFinalCandidate || tx.State == StateTruthCommitPending || tx.State == StateCheckpointPending {
		return c.Store.Snapshot(ctx, projectID, chapter)
	}
	if tx.State == StateHold {
		if candidates, candidateErr := c.Store.Candidates(ctx, tx.ID); candidateErr == nil && len(candidates) > 0 && candidates[len(candidates)-1].Attempt >= tx.Attempt {
			return c.Store.Snapshot(ctx, projectID, chapter)
		}
	}
	if tx.State != StateDrafting {
		tx, err = c.Store.Transition(ctx, tx.ID, StateDrafting, "writer requested", "coordinator", tx.Attempt)
		if err != nil {
			return Snapshot{}, err
		}
	}
	request := WriterRequest{ProjectID: projectID, Chapter: chapter, TransactionID: tx.ID, Attempt: tx.Attempt, Plan: plan}
	if tx.Attempt > 0 {
		candidates, readErr := c.Store.Candidates(ctx, tx.ID)
		if readErr != nil {
			return Snapshot{}, readErr
		}
		var previous *Candidate
		for i := range candidates {
			if candidates[i].Attempt < tx.Attempt && (previous == nil || candidates[i].Attempt > previous.Attempt) {
				previous = &candidates[i]
			}
		}
		if previous == nil {
			return Snapshot{}, errors.New("rewrite source candidate is missing")
		}
		request.PreviousDraft = previous.Text
		request.Feedback = []string{}
		if continuity, e := c.Store.Continuity(ctx, tx.ID, previous.ID); e == nil {
			for _, issue := range continuity.Issues {
				request.Feedback = append(request.Feedback, issue.IssueCode+": "+issue.SuggestedAction)
			}
		}
		if review, e := c.Store.Editor(ctx, tx.ID, previous.ID); e == nil {
			request.Feedback = append(request.Feedback, review.Weaknesses...)
		}
	}
	result, err := c.Writer.Write(ctx, request)
	if err != nil {
		_, _ = c.Store.Transition(ctx, tx.ID, StateHold, "writer failed", "writer", tx.Attempt)
		return c.Store.Snapshot(ctx, projectID, chapter)
	}
	candidate, err := c.Store.SaveCandidate(ctx, tx.ID, result.Text, result.SourceVersion, tx.Attempt)
	if err != nil {
		_, _ = c.Store.Transition(ctx, tx.ID, StateHold, "draft persistence failed", "coordinator", tx.Attempt)
		return Snapshot{}, err
	}
	if err := c.fail("after_draft_persist"); err != nil {
		_, _ = c.Store.Transition(ctx, tx.ID, StateHold, "failure after draft persistence; draft retained", "coordinator", tx.Attempt)
		return c.Store.Snapshot(ctx, projectID, chapter)
	}
	if _, err := c.Store.Transition(ctx, tx.ID, StateDraftReady, "draft persisted: "+candidate.ID, "writer", tx.Attempt); err != nil {
		return Snapshot{}, err
	}
	return c.Store.Snapshot(ctx, projectID, chapter)
}

func (c *Coordinator) Check(ctx context.Context, projectID string, chapter int) (Snapshot, error) {
	if err := c.validate(); err != nil {
		return Snapshot{}, err
	}
	if c.Librarian == nil || c.Continuity == nil || c.Editor == nil {
		return Snapshot{}, errors.New("quality services are not configured")
	}
	tx, err := c.Store.transactionByProjectChapter(ctx, projectID, chapter)
	if err != nil {
		return Snapshot{}, err
	}
	candidates, err := c.Store.Candidates(ctx, tx.ID)
	if err != nil || len(candidates) == 0 {
		return Snapshot{}, fmt.Errorf("draft candidate is required: %w", err)
	}
	candidate := candidates[len(candidates)-1]

	if tx.State == StateHold {
		if _, reviewErr := c.Store.Editor(ctx, tx.ID, candidate.ID); reviewErr == nil {
			tx, err = c.Store.Transition(ctx, tx.ID, StateReviewed, "resume from persisted editor review", "coordinator", tx.Attempt)
		} else if savedContinuity, continuityErr := c.Store.Continuity(ctx, tx.ID, candidate.ID); continuityErr == nil {
			next := StateContinuityPass
			switch savedContinuity.Status {
			case ContinuityWarn:
				next = StateContinuityWarn
			case ContinuityFail:
				next = StateContinuityFail
			}
			tx, err = c.Store.Transition(ctx, tx.ID, next, "resume from persisted continuity result", "coordinator", tx.Attempt)
		} else if _, proposalErr := c.Store.Proposal(ctx, tx.ID, candidate.ID); proposalErr == nil {
			tx, err = c.Store.Transition(ctx, tx.ID, StateFactsProposed, "resume from persisted fact proposal", "coordinator", tx.Attempt)
		} else {
			tx, err = c.Store.Transition(ctx, tx.ID, StateLibrarianPending, "resume fact extraction", "coordinator", tx.Attempt)
		}
		if err != nil {
			return Snapshot{}, err
		}
	}
	if tx.State == StateDraftReady {
		tx, err = c.Store.Transition(ctx, tx.ID, StateLibrarianPending, "extract fact proposal", "coordinator", tx.Attempt)
		if err != nil {
			return Snapshot{}, err
		}
	}
	if tx.State == StateLibrarianPending {
		proposal, callErr := c.Librarian.Propose(ctx, LibrarianRequest{ProjectID: projectID, Chapter: chapter, TransactionID: tx.ID, Candidate: candidate})
		if callErr != nil {
			_, _ = c.Store.Transition(ctx, tx.ID, StateHold, "librarian failed; draft retained", "librarian", tx.Attempt)
			return c.Store.Snapshot(ctx, projectID, chapter)
		}
		if err := c.Store.SaveProposal(ctx, tx.ID, candidate.ID, proposal); err != nil {
			_, _ = c.Store.Transition(ctx, tx.ID, StateHold, "fact proposal validation or persistence failed", "librarian", tx.Attempt)
			return Snapshot{}, err
		}
		tx, err = c.Store.Transition(ctx, tx.ID, StateFactsProposed, "fact proposal persisted", "librarian", tx.Attempt)
		if err != nil {
			return Snapshot{}, err
		}
	}
	proposal, err := c.Store.Proposal(ctx, tx.ID, candidate.ID)
	if err != nil {
		return Snapshot{}, err
	}
	if tx.State == StateFactsProposed {
		tx, err = c.Store.Transition(ctx, tx.ID, StateContinuityPending, "deterministic Chapter-N continuity check", "coordinator", tx.Attempt)
		if err != nil {
			return Snapshot{}, err
		}
	}
	var continuity ContinuityResult
	if tx.State == StateContinuityPending {
		continuity, err = c.Continuity.Check(ctx, ContinuityRequest{ProjectID: projectID, Chapter: chapter, TransactionID: tx.ID, Candidate: candidate, Proposal: proposal})
		if err != nil {
			_, _ = c.Store.Transition(ctx, tx.ID, StateHold, "continuity service failed; proposal retained", "continuity", tx.Attempt)
			return c.Store.Snapshot(ctx, projectID, chapter)
		}
		if err := c.Store.SaveContinuity(ctx, tx.ID, candidate.ID, continuity); err != nil {
			return Snapshot{}, err
		}
		next := StateContinuityPass
		switch continuity.Status {
		case ContinuityWarn:
			next = StateContinuityWarn
		case ContinuityFail:
			next = StateContinuityFail
		}
		tx, err = c.Store.Transition(ctx, tx.ID, next, "continuity result "+string(continuity.Status), "continuity", tx.Attempt)
		if err != nil {
			return Snapshot{}, err
		}
	} else {
		continuity, _ = c.Store.Continuity(ctx, tx.ID, candidate.ID)
	}

	if continuity.Status == ContinuityFail || !c.policy().Allows(continuity) {
		if tx.Attempt < tx.MaxRewrites {
			if tx.State != StateRewritePending {
				_, _ = c.Store.Transition(ctx, tx.ID, StateRewritePending, "continuity blocks finalization", "policy", tx.Attempt)
			}
		} else {
			_, _ = c.Store.Transition(ctx, tx.ID, StateHold, "continuity FAIL with rewrite budget exhausted", "policy", tx.Attempt)
		}
		return c.Store.Snapshot(ctx, projectID, chapter)
	}

	if tx.State == StateContinuityPass || tx.State == StateContinuityWarn {
		tx, err = c.Store.Transition(ctx, tx.ID, StateEditorPending, "literary review", "coordinator", tx.Attempt)
		if err != nil {
			return Snapshot{}, err
		}
	}
	if tx.State == StateEditorPending {
		review, reviewErr := c.Editor.Review(ctx, EditorRequest{ProjectID: projectID, Chapter: chapter, TransactionID: tx.ID, Candidate: candidate, Continuity: continuity})
		if reviewErr != nil {
			_, _ = c.Store.Transition(ctx, tx.ID, StateHold, "editor failed; draft, proposal and continuity retained", "editor", tx.Attempt)
			return c.Store.Snapshot(ctx, projectID, chapter)
		}
		if err := c.Store.SaveEditor(ctx, tx.ID, candidate.ID, review); err != nil {
			return Snapshot{}, err
		}
		tx, err = c.Store.Transition(ctx, tx.ID, StateReviewed, "editor review persisted", "editor", tx.Attempt)
		if err != nil {
			return Snapshot{}, err
		}
	}
	review, err := c.Store.Editor(ctx, tx.ID, candidate.ID)
	if err != nil {
		return Snapshot{}, err
	}
	if review.Score >= tx.QualityThreshold && c.policy().Allows(continuity) {
		reason := "continuity accepted and editor score met threshold"
		if continuity.Status == ContinuityWarn {
			reason += "; WARN allowed by deterministic policy"
		}
		if err := c.Store.SelectFinal(ctx, tx.ID, candidate.ID, reason); err != nil {
			return Snapshot{}, err
		}
		_, err = c.Store.Transition(ctx, tx.ID, StateFinalCandidate, reason, "policy", tx.Attempt)
		if err != nil {
			return Snapshot{}, err
		}
		return c.Store.Snapshot(ctx, projectID, chapter)
	}
	if tx.Attempt < tx.MaxRewrites {
		_, _ = c.Store.Transition(ctx, tx.ID, StateRewritePending, "editor score below threshold", "policy", tx.Attempt)
		return c.Store.Snapshot(ctx, projectID, chapter)
	}
	best, reason, err := c.Store.BestSafeCandidate(ctx, tx.ID, c.policy())
	if err != nil {
		_, _ = c.Store.Transition(ctx, tx.ID, StateHold, "no continuity-safe candidate after rewrite limit", "policy", tx.Attempt)
		return c.Store.Snapshot(ctx, projectID, chapter)
	}
	if err := c.Store.SelectFinal(ctx, tx.ID, best.ID, reason+" after rewrite limit"); err != nil {
		return Snapshot{}, err
	}
	_, err = c.Store.Transition(ctx, tx.ID, StateFinalCandidate, reason+" after rewrite limit", "policy", tx.Attempt)
	if err != nil {
		return Snapshot{}, err
	}
	return c.Store.Snapshot(ctx, projectID, chapter)
}
