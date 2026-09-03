package server

import (
	"errors"
	"net/http"

	"github.com/voocel/ainovel-cli/internal/chapterversion"
	"github.com/voocel/ainovel-cli/internal/narrativeledger"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
)

// finalizeQualityPhase8 is the production adapter for the existing Generate /
// Check / Rewrite quality workflow. It deliberately does not call the legacy
// Coordinator.Finalize path, because Phase 8 requires an immutable Final
// ChapterVersion to exist before the first Truth append.
func (s *Server) finalizeQualityPhase8(r *http.Request, quality *qualitygate.Coordinator, projectID string, chapter int, key string) (qualitygate.Snapshot, error) {
	snapshot, err := quality.Store.Snapshot(r.Context(), projectID, chapter)
	if err != nil {
		return qualitygate.Snapshot{}, err
	}
	tx := snapshot.Transaction
	if tx.State == qualitygate.StateCompleted {
		return snapshot, nil
	}
	if tx.FinalCandidateID == "" {
		best, reason, bestErr := quality.Store.BestSafeCandidate(r.Context(), tx.ID, s.cfg.QualityPolicy)
		if bestErr != nil {
			_, _ = quality.Store.Transition(r.Context(), tx.ID, qualitygate.StateHold, "finalize refused: no continuity-safe candidate", "policy", tx.Attempt)
			return quality.Store.Snapshot(r.Context(), projectID, chapter)
		}
		if err := quality.Store.SelectFinal(r.Context(), tx.ID, best.ID, reason); err != nil {
			return qualitygate.Snapshot{}, err
		}
		tx.FinalCandidateID = best.ID
		if tx.State != qualitygate.StateFinalCandidate {
			tx, err = quality.Store.Transition(r.Context(), tx.ID, qualitygate.StateFinalCandidate, reason, "policy", tx.Attempt)
			if err != nil {
				return qualitygate.Snapshot{}, err
			}
		}
	}
	candidate, err := quality.Store.Candidate(r.Context(), tx.FinalCandidateID)
	if err != nil {
		return qualitygate.Snapshot{}, err
	}
	continuity, err := quality.Store.Continuity(r.Context(), tx.ID, candidate.ID)
	if err != nil || continuity.Status == qualitygate.ContinuityFail || !s.cfg.QualityPolicy.Allows(continuity) {
		_, _ = quality.Store.Transition(r.Context(), tx.ID, qualitygate.StateHold, "continuity policy blocks finalization", "policy", tx.Attempt)
		if err != nil {
			return qualitygate.Snapshot{}, err
		}
		return quality.Store.Snapshot(r.Context(), projectID, chapter)
	}
	proposal, err := quality.Store.Proposal(r.Context(), tx.ID, candidate.ID)
	if err != nil {
		return qualitygate.Snapshot{}, err
	}
	var review *qualitygate.EditorReview
	if value, reviewErr := quality.Store.Editor(r.Context(), tx.ID, candidate.ID); reviewErr == nil {
		review = &value
	} else if !errors.Is(reviewErr, qualitygate.ErrNotFound) {
		return qualitygate.Snapshot{}, reviewErr
	}
	if tx.State == qualitygate.StateFinalCandidate || tx.State == qualitygate.StateHold {
		tx, err = quality.Store.Transition(r.Context(), tx.ID, qualitygate.StateTruthCommitPending, "immutable ChapterVersion Final committing Truth", "phase8", tx.Attempt)
		if err != nil {
			return qualitygate.Snapshot{}, err
		}
	}
	versionStore, err := s.projects.OpenChapterVersionStore(r.Context(), projectID)
	if err != nil {
		return qualitygate.Snapshot{}, err
	}
	defer versionStore.Close()
	ledger, ok := quality.Ledger.(*narrativeledger.Store)
	if !ok {
		return qualitygate.Snapshot{}, errors.New("Narrative Ledger store is required for Phase 8 finalization")
	}
	coordinator := &chapterversion.Coordinator{
		Store: versionStore, Truth: quality.Truth, Ledger: ledger,
		FinalWriter: quality.FinalWriter,
	}
	if _, err := coordinator.FinalizeQualityCandidate(r.Context(), key, tx.ID, candidate, proposal, continuity, review); err != nil {
		return quality.Store.Snapshot(r.Context(), projectID, chapter)
	}
	if tx.State != qualitygate.StateCheckpointPending {
		tx, err = quality.Store.Transition(r.Context(), tx.ID, qualitygate.StateCheckpointPending, "Phase 8 Final/Truth/Ledger/file committed; legacy quality checkpoint pending", "phase8", tx.Attempt)
		if err != nil {
			return qualitygate.Snapshot{}, err
		}
	}
	if err := quality.Store.SaveCheckpoint(r.Context(), tx.ID, candidate.ID, candidate.TextSHA); err != nil {
		return quality.Store.Snapshot(r.Context(), projectID, chapter)
	}
	if _, err := quality.Store.Transition(r.Context(), tx.ID, qualitygate.StateCompleted, "immutable Final ChapterVersion, Truth and checkpoints committed", "phase8", tx.Attempt); err != nil {
		return qualitygate.Snapshot{}, err
	}
	return quality.Store.Snapshot(r.Context(), projectID, chapter)
}
