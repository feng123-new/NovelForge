package chapterversion

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/voocel/ainovel-cli/internal/qualitygate"
)

// FinalizeQualityCandidate bridges the pre-Phase-8 quality workflow into the
// immutable ChapterVersion boundary without re-running model calls. Production
// HTTP finalization uses this method so Truth is first submitted only after an
// immutable generated candidate has been accepted and a Final version has been
// created by the recoverable Phase 8 saga.
func (c *Coordinator) FinalizeQualityCandidate(
	ctx context.Context,
	key string,
	transactionID string,
	candidate qualitygate.Candidate,
	proposal qualitygate.FactProposal,
	continuity qualitygate.ContinuityResult,
	review *qualitygate.EditorReview,
) (FinalizeResult, error) {
	if err := c.validate(); err != nil {
		return FinalizeResult{}, err
	}
	version, err := c.Store.generatedCandidate(ctx, transactionID, candidate)
	if err != nil {
		return FinalizeResult{}, err
	}
	conflicts, err := c.proposalConflicts(ctx, version.Chapter, proposal)
	if err != nil {
		return FinalizeResult{}, err
	}
	evaluation := Evaluation{Proposal: proposal, Continuity: continuity, Review: review, Conflicts: conflicts, EvaluatedAt: c.Store.now().UTC()}
	if !version.Accepted {
		if err := c.Store.AppendEvent(ctx, version.Chapter, version.ID, "evaluation_completed", "Phase 5 quality evidence imported without another model call", mustJSON(evaluation)); err != nil {
			return FinalizeResult{}, err
		}
		if continuity.Status == qualitygate.ContinuityFail || continuity.Blocking {
			return FinalizeResult{}, newError(CodeContinuityBlocked, "continuity FAIL blocks generated finalization", false, nil)
		}
		for _, conflict := range conflicts {
			if conflict.ExistingAuthority == "human_final" {
				return FinalizeResult{}, newError(CodeTruthConflict, "generated candidate cannot overwrite Accepted Human Final Truth", false, nil)
			}
		}
		if err := c.Store.AppendEvent(ctx, version.Chapter, version.ID, "accept", "quality gate selected continuity-safe generated candidate", mustJSON(evaluation)); err != nil {
			return FinalizeResult{}, err
		}
		version, err = c.Store.Get(ctx, version.Chapter, version.ID, true)
		if err != nil {
			return FinalizeResult{}, err
		}
	}
	return c.Finalize(ctx, version.Chapter, key+":phase8", version.ID)
}

func (s *Store) generatedCandidate(ctx context.Context, transactionID string, candidate qualitygate.Candidate) (Version, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM chapter_versions WHERE project_id=? AND chapter=? AND content_sha=? AND author_type='writer'
		AND json_extract(provenance_json,'$.quality_candidate_id')=? ORDER BY version_number ASC LIMIT 1`,
		s.projectID, candidate.Chapter, candidate.TextSHA, candidate.ID).Scan(&id)
	if err == nil {
		return s.Get(ctx, candidate.Chapter, id, true)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Version{}, newError(CodeStorage, "quality candidate ChapterVersion could not be read", true, err)
	}
	provenance, _ := json.Marshal(map[string]any{
		"source":               "quality_gate_candidate",
		"quality_transaction":  transactionID,
		"quality_candidate_id": candidate.ID,
		"quality_attempt":      candidate.Attempt,
		"source_version":       candidate.SourceVersion,
		"text_sha":             candidate.TextSHA,
	})
	return s.Create(ctx, candidate.Chapter, CreateInput{
		Content: candidate.Text, Type: TypeDraft, AuthorType: AuthorWriter,
		Provenance: provenance,
	})
}
