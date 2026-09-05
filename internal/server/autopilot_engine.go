package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/voocel/ainovel-cli/internal/autopilot"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/project"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
	"github.com/voocel/ainovel-cli/internal/server/qualityruntime"
)

// chapterJobEngine adapts the existing deterministic Chapter Engine (Phase 5)
// and immutable Final coordinator (Phase 8). It does not invoke the old host
// writing loop, which would bypass this authority boundary.
type chapterJobEngine struct{ s *Server }

func (s *Server) autopilotModel(ctx context.Context, id string, profile map[string]string) (qualitygate.ModelInvoker, error) {
	if s.cfg.QualityModel != nil {
		return s.cfg.QualityModel, nil
	}
	if !s.cfg.QualityConfigEnabled {
		return nil, autopilot.Stop("MODEL_NOT_CONFIGURED")
	}
	cfg, err := s.projects.LoadModelConfig(ctx, id, s.cfg.QualityConfigPath)
	if err != nil {
		return nil, autopilot.Stop("MODEL_NOT_CONFIGURED")
	}
	if cfg.Roles == nil {
		cfg.Roles = map[string]bootstrap.RoleConfig{}
	}
	for role, value := range profile {
		if role != "architect" && role != "planner" && role != "writer" && role != "librarian" && role != "editor" {
			return nil, autopilot.Stop("MODEL_PROFILE_INVALID")
		}
		provider, model, ok := strings.Cut(value, "/")
		if !ok || model == "" {
			return nil, autopilot.Stop("MODEL_PROFILE_INVALID")
		}
		if _, ok = cfg.Providers[provider]; !ok {
			return nil, autopilot.Stop("MODEL_PROFILE_NOT_CONFIGURED")
		}
		selection := cfg.Roles[role]
		selection.Provider = provider
		selection.Model = model
		cfg.Roles[role] = selection
	}
	model, err := qualityruntime.New(cfg)
	if err != nil {
		return nil, autopilot.Stop("MODEL_NOT_CONFIGURED")
	}
	return model, nil
}

func (e chapterJobEngine) Step(ctx context.Context, j autopilot.Job) (autopilot.Job, error) {
	if _, err := e.s.projects.Get(ctx, j.ProjectID); err != nil {
		return j, autopilot.Stop("PROJECT_UNAVAILABLE")
	}
	model, err := e.s.autopilotModel(ctx, j.ProjectID, j.Input.ModelProfile)
	if err != nil {
		return j, err
	}
	// Copy configuration, not Server ownership; this adapter never closes Server.
	local := *e.s
	local.cfg = e.s.cfg
	local.cfg.QualityModel = model
	local.cfg.QualityPolicy.MaxRewrites = j.Input.MaxRewrites
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/internal-autopilot", nil)
	quality, cleanup, failure := local.qualityCoordinator(req, j.ProjectID)
	if failure != nil {
		return j, autopilot.Stop("QUALITY_SERVICE_UNAVAILABLE")
	}
	defer cleanup()
	if writer, ok := quality.Writer.(qualitygate.ModelWriterService); ok {
		writer.Context = project.FoundationContext{Repository: e.s.projects, Foundation: j.Foundation, Style: fmt.Sprintf("%s\nLanguage: %s. Target chapter length: %d characters/words.", j.Input.Style, j.Input.Language, j.Input.WordsPerChapter)}
		quality.Writer = writer
	}
	call := func(agent, operation string, payload any) ([]byte, error) {
		if operation == "foundation" {
			selected, selectErr := e.s.projects.CompilePlanningSkills(ctx, j.ProjectID, j.ID+":foundation", j.Input.Idea)
			if selectErr != nil {
				return nil, autopilot.Stop("AUTHORING_CONTEXT_FAILED")
			}
			raw, marshalErr := json.Marshal(payload)
			if marshalErr != nil {
				return nil, marshalErr
			}
			var fields map[string]json.RawMessage
			if unmarshalErr := json.Unmarshal(raw, &fields); unmarshalErr != nil {
				return nil, unmarshalErr
			}
			fields["authoring_context"] = selected
			payload = fields
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		invoker := qualitygate.RetryingModelInvoker{Invoker: model, MaxRetries: e.s.cfg.QualityMaxRetries}
		caller := qualitygate.IdempotentModelCaller{Repository: quality.Store, Invoker: invoker}
		out, _, _, err := caller.Call(ctx, qualitygate.CallRequest{IdempotencyKey: j.CallKey(agent + ":" + operation), ProjectID: j.ProjectID, Chapter: j.Chapter, TransactionID: j.ID, Agent: agent, Operation: operation, Payload: data})
		if err != nil {
			var ce *qualitygate.ModelCallError
			if errors.As(err, &ce) && ce.Retryable {
				return nil, autopilot.Retry("PLANNING_PROVIDER_RETRY")
			}
			return nil, autopilot.Stop("PLANNING_CALL_FAILED")
		}
		return out, nil
	}
	// Before planning, do not rebrand an older unfinished quality transaction
	// with this job's new plan/fingerprint. Explicit completed human work can
	// take over even when this cursor was paused before draft generation.
	if j.Stage == "plan_context" || j.Stage == "plan" {
		complete, proofErr := e.s.projects.AutopilotFinalComplete(ctx, j.ProjectID, j.Chapter)
		if proofErr != nil {
			return j, autopilot.Stop("FINAL_PROOF_UNAVAILABLE")
		}
		if complete {
			return e.advance(ctx, j)
		}
		if _, snapshotErr := quality.Store.Snapshot(ctx, j.ProjectID, j.Chapter); snapshotErr == nil {
			return j, autopilot.Stop("EXISTING_DRAFT_REQUIRES_REVIEW")
		} else if !errors.Is(snapshotErr, qualitygate.ErrNotFound) {
			return j, autopilot.Stop("QUALITY_STATE_UNAVAILABLE")
		}
	}
	switch j.Stage {
	case "foundation":
		saved, loadErr := e.s.projects.LoadAutopilotFoundation(ctx, j.ProjectID, j.Input.FoundationID)
		if loadErr == nil {
			if saved.Validate(1000) != nil || !saved.Covers(j.Input.PlanningTarget()) {
				return j, autopilot.Stop("FOUNDATION_HORIZON_MISMATCH")
			}
			j.Foundation = saved
			j.Stage = "plan_context"
			j.ErrorCode = ""
			return j, nil
		}
		if !errors.Is(loadErr, os.ErrNotExist) {
			return j, autopilot.Stop("FOUNDATION_STORAGE_ERROR")
		}
		foundationInput := j.Input
		foundationInput.TargetChapter = j.Input.PlanningTarget()
		data, err := call("architect", "foundation", foundationInput)
		if err != nil {
			return j, err
		}
		var f autopilot.Foundation
		if err = autopilot.Decode(data, &f); err != nil {
			return j, autopilot.Stop("FOUNDATION_SCHEMA_INVALID")
		}
		if f.Validate(j.Input.PlanningTarget()) != nil || !f.Covers(j.Input.PlanningTarget()) {
			return j, autopilot.Stop("FOUNDATION_INVALID")
		}
		if err = e.s.projects.SaveAutopilotFoundation(ctx, j.ProjectID, j.Input.FoundationID, f); err != nil {
			return j, autopilot.Stop("FOUNDATION_STORAGE_ERROR")
		}
		j.Foundation = &f
		j.Stage = "plan_context"
		j.ErrorCode = ""
		return j, nil
	case "plan_context":
		if j.Foundation == nil {
			return j, autopilot.Stop("FOUNDATION_MISSING")
		}
		if err = e.checkBoundary(ctx, j, true); err != nil {
			return j, err
		}
		data, err := e.s.projects.PlanningContext(ctx, j.ProjectID, j.Chapter, j.Foundation.POV, j.ID)
		if err != nil {
			return j, autopilot.Stop("PLANNING_CONTEXT_FAILED")
		}
		fingerprint, err := e.s.projects.AutopilotFingerprint(ctx, j.ProjectID, j.Chapter)
		if err != nil {
			return j, autopilot.Stop("AUTHORITY_SNAPSHOT_FAILED")
		}
		j.AuthorityFingerprint = fingerprint
		j.PlanningContext = data
		j.Stage = "plan"
		j.ErrorCode = ""
		return j, nil
	case "plan":
		if j.Foundation == nil || len(j.PlanningContext) == 0 {
			return j, autopilot.Stop("PLANNING_CONTEXT_MISSING")
		}
		if err := e.checkAuthority(ctx, j); err != nil {
			return j, err
		}
		data, err := call("planner", "chapter", map[string]any{"chapter": j.Chapter, "target_chapter": j.Input.PlanningTarget(), "batch_target_chapter": j.Input.TargetChapter, "planning_pov": j.Foundation.POV, "style": j.Input.Style, "words_per_chapter": j.Input.WordsPerChapter, "language": j.Input.Language, "foundation": j.Foundation, "selected_context": j.PlanningContext})
		if err != nil {
			return j, err
		}
		var p qualitygate.ChapterPlan
		if autopilot.Decode(data, &p) != nil || p.Validate() != nil || p.Chapter != j.Chapter {
			return j, autopilot.Stop("CHAPTER_PLAN_INVALID")
		}
		known := false
		for _, c := range j.Foundation.Characters {
			known = known || c.ID == p.POV
		}
		if !known || p.POV != j.Foundation.POV {
			return j, autopilot.Stop("PLAN_POV_SCOPE_MISMATCH")
		}
		j.Plan = &p
		j.Stage = "generate"
		j.ErrorCode = ""
		return j, nil
	case "generate", "check", "rewrite", "finalize":
		if j.Plan == nil || j.Plan.Chapter != j.Chapter {
			return j, autopilot.Stop("CHAPTER_PLAN_MISSING")
		}
		snapshot, err := quality.Store.Snapshot(ctx, j.ProjectID, j.Chapter)
		if err != nil && !errors.Is(err, qualitygate.ErrNotFound) {
			return j, autopilot.Stop("QUALITY_STATE_UNAVAILABLE")
		}
		// Partial generated Final commits must converge before any takeover or stale-input check.
		partial := err == nil && (snapshot.Transaction.State == qualitygate.StateTruthCommitPending || snapshot.Transaction.State == qualitygate.StateCheckpointPending)
		if !partial {
			complete, proofErr := e.s.projects.AutopilotFinalComplete(ctx, j.ProjectID, j.Chapter)
			if proofErr != nil {
				return j, autopilot.Stop("FINAL_PROOF_UNAVAILABLE")
			}
			if complete {
				return e.advance(ctx, j)
			} // explicit Human Final may finish a paused chapter
			if authorityErr := e.checkAuthority(ctx, j); authorityErr != nil {
				return j, authorityErr
			}
		}
		if errors.Is(err, qualitygate.ErrNotFound) {
			if err = e.checkBoundary(ctx, j, true); err != nil {
				return j, err
			}
			snapshot, err = quality.Generate(ctx, j.ProjectID, j.Chapter, *j.Plan)
		} else {
			switch snapshot.Transaction.State {
			case qualitygate.StateCompleted:
				return e.advance(ctx, j)
			case qualitygate.StateFinalCandidate:
				due := j.Input.ReviewEvery > 0 && ((j.Chapter-j.Input.StartChapter+1)%j.Input.ReviewEvery == 0 || j.Chapter == j.Input.TargetChapter)
				if due && (!j.ReviewApproved || j.ReviewCandidateID != snapshot.Transaction.FinalCandidateID) {
					j.State = autopilot.Paused
					j.Stage = "finalize"
					j.ErrorCode = "REVIEW_REQUIRED"
					j.ReviewApproved = false
					j.ReviewCandidateID = snapshot.Transaction.FinalCandidateID
					return j, nil
				}
				if err = e.checkBoundary(ctx, j, true); err != nil {
					return j, err
				}
				snapshot, err = local.finalizeQualityPhase8(req, quality, j.ProjectID, j.Chapter, j.CallKey("final"))
				if err == nil && snapshot.Transaction.State != qualitygate.StateCompleted {
					return j, autopilot.Retry("FINALIZE_INCOMPLETE")
				}
			case qualitygate.StateTruthCommitPending, qualitygate.StateCheckpointPending:
				// An incomplete Final saga must be replayed, never treated as completed
				// merely because the Active Final pointer has already switched.
				snapshot, err = local.finalizeQualityPhase8(req, quality, j.ProjectID, j.Chapter, j.CallKey("final"))
				if err == nil && snapshot.Transaction.State != qualitygate.StateCompleted {
					return j, autopilot.Retry("FINALIZE_INCOMPLETE")
				}
			case qualitygate.StateRewritePending:
				if snapshot.Transaction.Attempt >= j.Input.MaxRewrites {
					return j, autopilot.Stop("REWRITE_BUDGET_EXHAUSTED")
				}
				snapshot, err = quality.Rewrite(ctx, j.ProjectID, j.Chapter, *j.Plan)
			case qualitygate.StatePlanned, qualitygate.StateDrafting:
				snapshot, err = quality.Generate(ctx, j.ProjectID, j.Chapter, *j.Plan)
			case qualitygate.StateHold:
				if snapshot.Continuity != nil && snapshot.Continuity.Status == qualitygate.ContinuityFail && snapshot.Transaction.Attempt >= snapshot.Transaction.MaxRewrites {
					return j, autopilot.Stop("CONTINUITY_REWRITE_EXHAUSTED")
				}
				if len(snapshot.Candidates) == 0 || snapshot.Candidates[len(snapshot.Candidates)-1].Attempt < snapshot.Transaction.Attempt {
					snapshot, err = quality.Generate(ctx, j.ProjectID, j.Chapter, *j.Plan)
				} else {
					snapshot, err = quality.Check(ctx, j.ProjectID, j.Chapter)
				}
			case qualitygate.StateFailed:
				return j, autopilot.Stop("QUALITY_FAILED")
			default:
				snapshot, err = quality.Check(ctx, j.ProjectID, j.Chapter)
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				return j, ctx.Err()
			}
			return j, autopilot.Retry("QUALITY_STEP_ERROR")
		}
		if snapshot.Transaction.State == qualitygate.StateHold || snapshot.Transaction.State == qualitygate.StateFailed {
			return j, autopilot.Stop("QUALITY_HOLD")
		}
		if snapshot.Transaction.State == qualitygate.StateCompleted {
			return e.advance(ctx, j)
		}
		switch snapshot.Transaction.State {
		case qualitygate.StateRewritePending:
			j.Stage = "rewrite"
		case qualitygate.StateFinalCandidate, qualitygate.StateTruthCommitPending, qualitygate.StateCheckpointPending:
			j.Stage = "finalize"
		default:
			j.Stage = "check"
		}
		j.ErrorCode = ""
		return j, nil
	default:
		return j, autopilot.Stop("AUTOPILOT_STAGE_INVALID")
	}
}

func (e chapterJobEngine) checkBoundary(ctx context.Context, j autopilot.Job, rejectCurrent bool) error {
	if rejectCurrent {
		pending, err := e.s.projects.ImportedChapterPending(ctx, j.ProjectID, j.Chapter)
		if err != nil {
			return autopilot.Stop("IMPORT_STATE_UNAVAILABLE")
		}
		if pending {
			return autopilot.Stop("IMPORTED_CHAPTER_REQUIRES_REVIEW")
		}
	}
	versions, err := e.s.projects.OpenChapterVersionStore(ctx, j.ProjectID)
	if err != nil {
		return autopilot.Stop("VERSION_STORE_UNAVAILABLE")
	}
	defer versions.Close()
	if j.Chapter > 1 {
		complete, proofErr := e.s.projects.AutopilotFinalComplete(ctx, j.ProjectID, j.Chapter-1)
		if proofErr != nil || !complete {
			return autopilot.Stop("PRIOR_FINAL_CHECKPOINT_REQUIRED")
		}
		active, err := versions.ActiveFinal(ctx, j.Chapter-1, false)
		if err != nil || active == nil {
			return autopilot.Stop("PRIOR_FINAL_REQUIRED")
		}
		status, err := versions.DetectExternal(ctx, j.Chapter-1)
		if err != nil || status.SyncRequired {
			return autopilot.Stop("PRIOR_CHAPTER_SYNC_REQUIRED")
		}
	}
	if rejectCurrent {
		active, err := versions.ActiveFinal(ctx, j.Chapter, false)
		if err != nil {
			return autopilot.Stop("VERSION_STORE_UNAVAILABLE")
		}
		if active != nil {
			return autopilot.Stop("EXISTING_FINAL_PROTECTED")
		}
		// Do not overwrite legacy or externally created prose that has not passed
		// the immutable version/sync workflow.
		for offset := 0; ; offset += 100 {
			page, err := e.s.projects.ListChapters(ctx, j.ProjectID, 100, offset)
			if err != nil {
				return autopilot.Stop("CHAPTER_INSPECTION_FAILED")
			}
			for _, c := range page.Chapters {
				if c.Chapter == j.Chapter {
					return autopilot.Stop("EXISTING_CHAPTER_REQUIRES_SYNC")
				}
			}
			if page.NextOffset == nil {
				break
			}
		}
	}
	return nil
}
func (e chapterJobEngine) advance(ctx context.Context, j autopilot.Job) (autopilot.Job, error) {
	if err := e.checkBoundary(ctx, j, false); err != nil {
		return j, err
	}
	complete, proofErr := e.s.projects.AutopilotFinalComplete(ctx, j.ProjectID, j.Chapter)
	if proofErr != nil || !complete {
		return j, autopilot.Stop("FINAL_CHECKPOINT_MISSING")
	}
	versions, err := e.s.projects.OpenChapterVersionStore(ctx, j.ProjectID)
	if err != nil {
		return j, autopilot.Stop("VERSION_STORE_UNAVAILABLE")
	}
	defer versions.Close()
	active, err := versions.ActiveFinal(ctx, j.Chapter, false)
	if err != nil || active == nil {
		return j, autopilot.Stop("FINAL_CHECKPOINT_MISSING")
	}
	sync, err := versions.DetectExternal(ctx, j.Chapter)
	if err != nil || sync.SyncRequired {
		return j, autopilot.Stop("CHAPTER_SYNC_REQUIRED")
	}
	j.CompletedThrough = j.Chapter
	j.Chapter++
	j.Plan = nil
	j.PlanningContext = nil
	j.AuthorityFingerprint = ""
	j.ReviewApproved = false
	j.ReviewCandidateID = ""
	j.ErrorCode = ""
	j.Stage = "plan_context"
	if j.Chapter > j.Input.TargetChapter {
		j.State = autopilot.Completed
		j.Stage = "completed"
	}
	return j, nil
}

// Conservatively reject stale plans/reviews after any authoritative edit.
// No prompt, fact or secret is exposed by this digest. A completed explicit
// Human Final may take over the chapter; it is verified before this guard.
func (e chapterJobEngine) checkAuthority(ctx context.Context, j autopilot.Job) error {
	if j.AuthorityFingerprint == "" {
		return autopilot.Stop("CONTEXT_BASELINE_REQUIRED")
	}
	fingerprint, err := e.s.projects.AutopilotFingerprint(ctx, j.ProjectID, j.Chapter)
	if err != nil {
		return autopilot.Stop("AUTHORITY_SNAPSHOT_FAILED")
	}
	if fingerprint != j.AuthorityFingerprint {
		return autopilot.Stop("CHAPTER_CONTEXT_CHANGED")
	}
	return e.checkBoundary(ctx, j, false)
}
