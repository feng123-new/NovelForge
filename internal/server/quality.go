package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/voocel/ainovel-cli/internal/project"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
)

type qualityView struct {
	Snapshot qualitygate.Snapshot `json:"snapshot"`
	Actions  qualityActions       `json:"actions"`
}

type qualityActions struct {
	Generate bool `json:"generate"`
	Check    bool `json:"check"`
	Rewrite  bool `json:"rewrite"`
	Finalize bool `json:"finalize"`
}

type qualityCandidatesView struct {
	Candidates []qualitygate.Candidate `json:"candidates"`
	Total      int                     `json:"total"`
}

func (s *Server) registerQualityRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}/quality", s.handleQualityState)
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}/candidates", s.handleQualityCandidates)
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}/generate", s.handleQualityGenerate)
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}/check", s.handleQualityCheck)
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}/rewrite", s.handleQualityRewrite)
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}/finalize", s.handleQualityFinalize)
}

func (s *Server) qualityIdentity(r *http.Request) (string, int, *apiFailure) {
	projectID := strings.TrimSpace(r.PathValue("id"))
	chapter, err := strconv.Atoi(strings.TrimSpace(r.PathValue("chapter")))
	if projectID == "" || err != nil || chapter <= 0 {
		return "", 0, &apiFailure{Status: http.StatusBadRequest, Code: "QUALITY_CHAPTER_INVALID", Message: "project and positive chapter are required"}
	}
	if _, err := s.projects.Get(r.Context(), projectID); err != nil {
		return "", 0, projectFailure(err)
	}
	return projectID, chapter, nil
}

func (s *Server) qualityConfigured() bool {
	return (s.cfg.QualityWriter != nil && s.cfg.QualityLibrarian != nil && s.cfg.QualityEditor != nil) || s.cfg.QualityModel != nil
}

func (s *Server) qualityCoordinator(r *http.Request, projectID string) (*qualitygate.Coordinator, func(), *apiFailure) {
	store, err := s.projects.OpenQualityStore(r.Context(), projectID)
	if err != nil {
		return nil, func() {}, projectFailure(err)
	}
	truth, err := s.projects.OpenTruthStore(r.Context(), projectID)
	if err != nil {
		_ = store.Close()
		return nil, func() {}, projectFailure(err)
	}
	cleanup := func() {
		_ = truth.Close()
		_ = store.Close()
	}
	writer, librarian, editor := s.cfg.QualityWriter, s.cfg.QualityLibrarian, s.cfg.QualityEditor
	if s.cfg.QualityModel != nil {
		invoker := qualitygate.ModelInvoker(s.cfg.QualityModel)
		if s.cfg.QualityMaxRetries > 0 {
			invoker = qualitygate.RetryingModelInvoker{Invoker: invoker, MaxRetries: s.cfg.QualityMaxRetries}
		}
		caller := &qualitygate.IdempotentModelCaller{Repository: store, Invoker: invoker}
		decoder := qualitygate.StrictDecoder{MaxRepairs: s.cfg.QualityMaxRepairs, Repairer: s.cfg.QualityRepairer}
		if writer == nil {
			writer = qualitygate.ModelWriterService{Caller: caller}
		}
		if librarian == nil {
			librarian = qualitygate.ModelLibrarianService{Caller: caller, Decoder: decoder}
		}
		if editor == nil {
			editor = qualitygate.ModelEditorService{Caller: caller, Decoder: decoder}
		}
	}
	coordinator := &qualitygate.Coordinator{
		Store: store, Truth: truth, Writer: writer, Librarian: librarian,
		Continuity: qualitygate.TruthContinuityService{Truth: truth}, Editor: editor,
		Policy: s.cfg.QualityPolicy, FinalWriter: s.projects,
	}
	return coordinator, cleanup, nil
}

func (s *Server) qualitySnapshot(r *http.Request, projectID string, chapter int) (qualityView, *apiFailure) {
	store, err := s.projects.OpenQualityStore(r.Context(), projectID)
	if err != nil {
		return qualityView{}, projectFailure(err)
	}
	defer store.Close()
	snapshot, err := store.Snapshot(r.Context(), projectID, chapter)
	if errors.Is(err, qualitygate.ErrNotFound) {
		return qualityView{Snapshot: qualitygate.Snapshot{}, Actions: qualityActions{Generate: s.qualityConfigured()}}, nil
	}
	if err != nil {
		return qualityView{}, qualityFailure(err)
	}
	return qualityView{Snapshot: snapshot, Actions: s.qualityActions(snapshot)}, nil
}

func (s *Server) qualityActions(snapshot qualitygate.Snapshot) qualityActions {
	configured := s.qualityConfigured()
	tx := snapshot.Transaction
	if tx.ID == "" {
		return qualityActions{Generate: configured}
	}
	terminal := tx.State == qualitygate.StateCompleted || tx.State == qualitygate.StateFailed
	return qualityActions{
		Generate: configured && !terminal && len(snapshot.Candidates) == 0,
		Check:    configured && !terminal && len(snapshot.Candidates) > 0 && tx.State != qualitygate.StateFinalCandidate && tx.State != qualitygate.StateTruthCommitPending && tx.State != qualitygate.StateCheckpointPending,
		Rewrite:  configured && !terminal && len(snapshot.Candidates) > 0 && tx.Attempt < tx.MaxRewrites && (tx.State == qualitygate.StateRewritePending || tx.State == qualitygate.StateContinuityFail || tx.State == qualitygate.StateReviewed),
		Finalize: !terminal && tx.FinalCandidateID != "" && snapshot.Continuity != nil && snapshot.Continuity.Status != qualitygate.ContinuityFail,
	}
}

func (s *Server) handleQualityState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	projectID, chapter, failure := s.qualityIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	view, failure := s.qualitySnapshot(r, projectID, chapter)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleQualityCandidates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	projectID, chapter, failure := s.qualityIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	store, err := s.projects.OpenQualityStore(r.Context(), projectID)
	if err != nil {
		writeFailure(w, r, *projectFailure(err))
		return
	}
	defer store.Close()
	tx, err := store.Snapshot(r.Context(), projectID, chapter)
	if errors.Is(err, qualitygate.ErrNotFound) {
		writeJSON(w, http.StatusOK, qualityCandidatesView{Candidates: []qualitygate.Candidate{}, Total: 0})
		return
	}
	if err != nil {
		writeFailure(w, r, *qualityFailure(err))
		return
	}
	writeJSON(w, http.StatusOK, qualityCandidatesView{Candidates: tx.Candidates, Total: len(tx.Candidates)})
}

func (s *Server) handleQualityGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	projectID, chapter, failure := s.qualityIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	s.executeIdempotent(w, r, "chapter.quality.generate", projectID, func(body []byte) (int, any, *apiFailure) {
		var plan qualitygate.ChapterPlan
		if failure := decodeJSONBody(body, &plan, false); failure != nil {
			return failure.Status, nil, failure
		}
		if plan.Chapter != chapter {
			return http.StatusBadRequest, nil, &apiFailure{Status: http.StatusBadRequest, Code: "QUALITY_CHAPTER_MISMATCH", Message: "chapter plan does not match route chapter"}
		}
		if err := plan.Validate(); err != nil {
			return http.StatusBadRequest, nil, &apiFailure{Status: http.StatusBadRequest, Code: "QUALITY_PLAN_INVALID", Message: "chapter plan is invalid"}
		}
		if !s.qualityConfigured() {
			failure := qualityUnavailable()
			return failure.Status, nil, &failure
		}
		coordinator, cleanup, failure := s.qualityCoordinator(r, projectID)
		if failure != nil {
			return failure.Status, nil, failure
		}
		defer cleanup()
		snapshot, err := coordinator.Generate(r.Context(), projectID, chapter, plan)
		if err != nil {
			failure := qualityFailure(err)
			return failure.Status, nil, failure
		}
		return http.StatusAccepted, qualityView{Snapshot: snapshot, Actions: s.qualityActions(snapshot)}, nil
	})
}

func (s *Server) handleQualityCheck(w http.ResponseWriter, r *http.Request) {
	s.handleQualityEmptyAction(w, r, "chapter.quality.check", true, func(c *qualitygate.Coordinator, projectID string, chapter int, _ string) (qualitygate.Snapshot, error) {
		return c.Check(r.Context(), projectID, chapter)
	})
}

func (s *Server) handleQualityRewrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	projectID, chapter, failure := s.qualityIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	s.executeIdempotent(w, r, "chapter.quality.rewrite", projectID, func(body []byte) (int, any, *apiFailure) {
		var plan qualitygate.ChapterPlan
		if failure := decodeJSONBody(body, &plan, false); failure != nil {
			return failure.Status, nil, failure
		}
		if plan.Chapter != chapter || plan.Validate() != nil {
			return http.StatusBadRequest, nil, &apiFailure{Status: http.StatusBadRequest, Code: "QUALITY_PLAN_INVALID", Message: "chapter plan is invalid for this route"}
		}
		if !s.qualityConfigured() {
			failure := qualityUnavailable()
			return failure.Status, nil, &failure
		}
		coordinator, cleanup, failure := s.qualityCoordinator(r, projectID)
		if failure != nil {
			return failure.Status, nil, failure
		}
		defer cleanup()
		snapshot, err := coordinator.Rewrite(r.Context(), projectID, chapter, plan)
		if err != nil {
			failure := qualityFailure(err)
			return failure.Status, nil, failure
		}
		return http.StatusAccepted, qualityView{Snapshot: snapshot, Actions: s.qualityActions(snapshot)}, nil
	})
}

func (s *Server) handleQualityFinalize(w http.ResponseWriter, r *http.Request) {
	s.handleQualityEmptyAction(w, r, "chapter.quality.finalize", false, func(c *qualitygate.Coordinator, projectID string, chapter int, key string) (qualitygate.Snapshot, error) {
		return c.Finalize(r.Context(), projectID, chapter, key)
	})
}

type qualityEmptyAction func(*qualitygate.Coordinator, string, int, string) (qualitygate.Snapshot, error)

func (s *Server) handleQualityEmptyAction(w http.ResponseWriter, r *http.Request, operation string, requiresAgents bool, action qualityEmptyAction) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	projectID, chapter, failure := s.qualityIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	s.executeIdempotent(w, r, operation, projectID, func(body []byte) (int, any, *apiFailure) {
		var empty struct{}
		if failure := decodeJSONBody(body, &empty, true); failure != nil {
			return failure.Status, nil, failure
		}
		if requiresAgents && !s.qualityConfigured() {
			failure := qualityUnavailable()
			return failure.Status, nil, &failure
		}
		coordinator, cleanup, failure := s.qualityCoordinator(r, projectID)
		if failure != nil {
			return failure.Status, nil, failure
		}
		defer cleanup()
		snapshot, err := action(coordinator, projectID, chapter, strings.TrimSpace(r.Header.Get("Idempotency-Key")))
		if err != nil {
			failure := qualityFailure(err)
			return failure.Status, nil, failure
		}
		return http.StatusAccepted, qualityView{Snapshot: snapshot, Actions: s.qualityActions(snapshot)}, nil
	})
}

func qualityUnavailable() apiFailure {
	return apiFailure{Status: http.StatusServiceUnavailable, Code: "QUALITY_SERVICE_UNAVAILABLE", Message: "chapter quality model services are not configured", Retryable: false}
}

func qualityFailure(err error) *apiFailure {
	var projectErr *project.Error
	if errors.As(err, &projectErr) {
		return projectFailure(err)
	}
	switch {
	case errors.Is(err, qualitygate.ErrNotFound):
		return &apiFailure{Status: http.StatusNotFound, Code: "QUALITY_NOT_FOUND", Message: "chapter quality transaction not found"}
	case errors.Is(err, qualitygate.ErrIdempotencyConflict):
		return &apiFailure{Status: http.StatusConflict, Code: "QUALITY_IDEMPOTENCY_CONFLICT", Message: "quality model idempotency key conflicts with another request"}
	case errors.Is(err, qualitygate.ErrRewriteLimit):
		return &apiFailure{Status: http.StatusConflict, Code: "QUALITY_REWRITE_LIMIT", Message: "rewrite limit has been reached"}
	case errors.Is(err, qualitygate.ErrNoSafeCandidate):
		return &apiFailure{Status: http.StatusConflict, Code: "QUALITY_NO_SAFE_CANDIDATE", Message: "no continuity-safe candidate is available"}
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "not configured") || strings.Contains(message, "is required") {
		failure := qualityUnavailable()
		return &failure
	}
	if strings.Contains(message, "illegal chapter transaction transition") || strings.Contains(message, "attempt cannot") {
		return &apiFailure{Status: http.StatusConflict, Code: "QUALITY_STATE_CONFLICT", Message: "chapter quality transaction is not in a state that permits this operation"}
	}
	return ptrFailure(internalFailure())
}
