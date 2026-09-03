package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/voocel/ainovel-cli/internal/chapterversion"
	"github.com/voocel/ainovel-cli/internal/narrativeledger"
)

type chapterPlanImpactView struct {
	Impacts    []chapterversion.PlanImpact `json:"impacts"`
	Total      int                         `json:"total"`
	Limit      int                         `json:"limit"`
	Offset     int                         `json:"offset"`
	NextOffset *int                        `json:"next_offset,omitempty"`
}

type chapterVersionCheckView struct {
	Evaluation chapterversion.Evaluation `json:"evaluation"`
}

func (s *Server) registerChapterVersionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}", s.handleChapterVersionState)
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}/versions", s.handleChapterVersions)
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}/versions/{version}", s.handleChapterVersion)
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}/diff", s.handleChapterDiff)
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}/versions/{version}/check", s.handleChapterVersionCheck)
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}/versions/{version}/restore", s.handleChapterVersionRestore)
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}/versions/{version}/accept", s.handleChapterVersionAccept)
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}/versions/{version}/reject", s.handleChapterVersionReject)
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}/versions/{version}/finalize", s.handleChapterVersionFinalize)
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}/sync-status", s.handleChapterSyncStatus)
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}/sync", s.handleChapterSync)
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}/rebuild", s.handleChapterRebuild)
	mux.HandleFunc("/api/projects/{id}/chapters/{chapter}/plan-impact", s.handleChapterPlanImpact)
}

func (s *Server) chapterVersionIdentity(r *http.Request) (string, int, *apiFailure) {
	projectID := strings.TrimSpace(r.PathValue("id"))
	chapter, err := strconv.Atoi(strings.TrimSpace(r.PathValue("chapter")))
	if projectID == "" || err != nil || chapter <= 0 {
		return "", 0, &apiFailure{Status: http.StatusBadRequest, Code: "CHAPTER_VERSION_VALIDATION_FAILED", Message: "project and positive chapter are required"}
	}
	if _, err := s.projects.Get(r.Context(), projectID); err != nil {
		return "", 0, projectFailure(err)
	}
	return projectID, chapter, nil
}

func (s *Server) chapterVersionStore(r *http.Request, projectID string) (*chapterversion.Store, func(), *apiFailure) {
	store, err := s.projects.OpenChapterVersionStore(r.Context(), projectID)
	if err != nil {
		return nil, func() {}, projectFailure(err)
	}
	return store, func() { _ = store.Close() }, nil
}

func (s *Server) chapterVersionCoordinator(r *http.Request, projectID string) (*chapterversion.Coordinator, func(), *apiFailure) {
	store, err := s.projects.OpenChapterVersionStore(r.Context(), projectID)
	if err != nil {
		return nil, func() {}, projectFailure(err)
	}
	quality, qualityCleanup, failure := s.qualityCoordinator(r, projectID)
	if failure != nil {
		_ = store.Close()
		return nil, func() {}, failure
	}
	ledger, ok := quality.Ledger.(*narrativeledger.Store)
	if !ok {
		qualityCleanup()
		_ = store.Close()
		failure := internalFailure()
		return nil, func() {}, &failure
	}
	cleanup := func() {
		qualityCleanup()
		_ = store.Close()
	}
	return &chapterversion.Coordinator{
		Store: store, Truth: quality.Truth, Ledger: ledger, Librarian: quality.Librarian,
		Continuity: quality.Continuity, Editor: quality.Editor, FinalWriter: quality.FinalWriter,
	}, cleanup, nil
}

func (s *Server) handleChapterVersionState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	projectID, chapter, failure := s.chapterVersionIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	store, cleanup, failure := s.chapterVersionStore(r, projectID)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer cleanup()
	if _, err := store.BootstrapLegacyFinal(r.Context(), chapter); err != nil {
		writeFailure(w, r, *chapterVersionFailure(err))
		return
	}
	active, err := store.ActiveFinal(r.Context(), chapter, true)
	if err != nil {
		writeFailure(w, r, *chapterVersionFailure(err))
		return
	}
	latest, err := store.Latest(r.Context(), chapter, true)
	if err != nil {
		writeFailure(w, r, *chapterVersionFailure(err))
		return
	}
	versions, err := store.List(r.Context(), chapter, chapterversion.ListOptions{Limit: 1})
	if err != nil {
		writeFailure(w, r, *chapterVersionFailure(err))
		return
	}
	sync := chapterversion.SyncStatus{ProjectID: projectID, Chapter: chapter}
	if active != nil {
		sync, err = store.DetectExternal(r.Context(), chapter)
		if err != nil {
			writeFailure(w, r, *chapterVersionFailure(err))
			return
		}
	}
	derived := "ready"
	if rebuild, ok, rebuildErr := store.LatestRebuild(r.Context(), chapter); rebuildErr != nil {
		writeFailure(w, r, *chapterVersionFailure(rebuildErr))
		return
	} else if ok {
		derived = rebuild.State
	}
	writeJSON(w, http.StatusOK, chapterversion.ChapterView{ProjectID: projectID, Chapter: chapter, ActiveFinal: active, Latest: latest, VersionCount: versions.Total, Sync: sync, DerivedState: derived})
}

func (s *Server) handleChapterVersions(w http.ResponseWriter, r *http.Request) {
	projectID, chapter, failure := s.chapterVersionIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	store, cleanup, failure := s.chapterVersionStore(r, projectID)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer cleanup()
	_, _ = store.BootstrapLegacyFinal(r.Context(), chapter)
	switch r.Method {
	case http.MethodGet:
		limit, offset, pageFailure := parseBoundedPage(r, 50, 100)
		if pageFailure != nil {
			writeFailure(w, r, *pageFailure)
			return
		}
		includeContent := false
		if raw := strings.TrimSpace(r.URL.Query().Get("include_content")); raw != "" {
			value, err := strconv.ParseBool(raw)
			if err != nil {
				writeFailure(w, r, apiFailure{Status: http.StatusBadRequest, Code: "CHAPTER_VERSION_VALIDATION_FAILED", Message: "include_content must be a boolean"})
				return
			}
			includeContent = value
		}
		result, err := store.List(r.Context(), chapter, chapterversion.ListOptions{
			Limit: limit, Offset: offset, Status: strings.TrimSpace(r.URL.Query().Get("status")),
			Type: chapterversion.VersionType(strings.TrimSpace(r.URL.Query().Get("type"))),
			AuthorType: chapterversion.AuthorType(strings.TrimSpace(r.URL.Query().Get("author_type"))), IncludeContent: includeContent,
		})
		if err != nil {
			writeFailure(w, r, *chapterVersionFailure(err))
			return
		}
		writeJSON(w, http.StatusOK, result)
	case http.MethodPost:
		s.executeIdempotent(w, r, "chapter.version.save_human", projectID, func(body []byte) (int, any, *apiFailure) {
			var input struct {
				Content string `json:"content"`
			}
			if failure := decodeJSONBody(body, &input, false); failure != nil {
				return failure.Status, nil, failure
			}
			service := chapterversion.Service{Store: store}
			version, err := service.SaveHuman(r.Context(), chapter, strings.TrimSpace(r.Header.Get("Idempotency-Key")), input.Content)
			if err != nil {
				failure := chapterVersionFailure(err)
				return failure.Status, nil, failure
			}
			return http.StatusCreated, version, nil
		})
	default:
		writeMethodNotAllowed(w, r, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleChapterVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	projectID, chapter, failure := s.chapterVersionIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	versionID := strings.TrimSpace(r.PathValue("version"))
	store, cleanup, failure := s.chapterVersionStore(r, projectID)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer cleanup()
	version, err := store.Get(r.Context(), chapter, versionID, true)
	if err != nil {
		writeFailure(w, r, *chapterVersionFailure(err))
		return
	}
	writeJSON(w, http.StatusOK, version)
}

func (s *Server) handleChapterDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	projectID, chapter, failure := s.chapterVersionIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	fromID := strings.TrimSpace(r.URL.Query().Get("from_version"))
	toID := strings.TrimSpace(r.URL.Query().Get("to_version"))
	if fromID == "" || toID == "" {
		writeFailure(w, r, apiFailure{Status: http.StatusBadRequest, Code: "CHAPTER_VERSION_VALIDATION_FAILED", Message: "from_version and to_version are required"})
		return
	}
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 500 {
			writeFailure(w, r, apiFailure{Status: http.StatusBadRequest, Code: "CHAPTER_VERSION_VALIDATION_FAILED", Message: "diff limit must be between 1 and 500"})
			return
		}
		limit = value
	}
	store, cleanup, failure := s.chapterVersionStore(r, projectID)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer cleanup()
	result, err := store.Diff(r.Context(), chapter, fromID, toID, chapterversion.DiffMode(strings.TrimSpace(r.URL.Query().Get("mode"))), strings.TrimSpace(r.URL.Query().Get("cursor")), limit)
	if err != nil {
		writeFailure(w, r, *chapterVersionFailure(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleChapterVersionCheck(w http.ResponseWriter, r *http.Request) {
	s.handleChapterCoordinatorAction(w, r, "chapter.version.check", func(coordinator *chapterversion.Coordinator, chapter int, key, versionID string, _ []byte) (int, any, error) {
		evaluation, err := coordinator.Check(r.Context(), chapter, key, versionID)
		return http.StatusOK, chapterVersionCheckView{Evaluation: evaluation}, err
	})
}

func (s *Server) handleChapterVersionRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	projectID, chapter, failure := s.chapterVersionIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	versionID := strings.TrimSpace(r.PathValue("version"))
	store, cleanup, failure := s.chapterVersionStore(r, projectID)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer cleanup()
	s.executeIdempotent(w, r, "chapter.version.restore", projectID, func(body []byte) (int, any, *apiFailure) {
		var empty struct{}
		if failure := decodeJSONBody(body, &empty, true); failure != nil {
			return failure.Status, nil, failure
		}
		service := chapterversion.Service{Store: store}
		version, err := service.Restore(r.Context(), chapter, strings.TrimSpace(r.Header.Get("Idempotency-Key")), versionID)
		if err != nil {
			failure := chapterVersionFailure(err)
			return failure.Status, nil, failure
		}
		return http.StatusCreated, chapterversion.RestoreResult{Version: version}, nil
	})
}

func (s *Server) handleChapterVersionAccept(w http.ResponseWriter, r *http.Request) {
	s.handleChapterCoordinatorAction(w, r, "chapter.version.accept", func(coordinator *chapterversion.Coordinator, chapter int, key, versionID string, body []byte) (int, any, error) {
		var input struct {
			Reason string `json:"reason,omitempty"`
		}
		if failure := decodeJSONBody(body, &input, true); failure != nil {
			return failure.Status, nil, apiFailureError(*failure)
		}
		version, err := coordinator.Accept(r.Context(), chapter, key, versionID, strings.TrimSpace(input.Reason))
		return http.StatusOK, chapterversion.AcceptResult{Version: version}, err
	})
}

func (s *Server) handleChapterVersionReject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	projectID, chapter, failure := s.chapterVersionIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	versionID := strings.TrimSpace(r.PathValue("version"))
	store, cleanup, failure := s.chapterVersionStore(r, projectID)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer cleanup()
	s.executeIdempotent(w, r, "chapter.version.reject", projectID, func(body []byte) (int, any, *apiFailure) {
		var input struct {
			Reason string `json:"reason"`
		}
		if failure := decodeJSONBody(body, &input, false); failure != nil {
			return failure.Status, nil, failure
		}
		service := chapterversion.Service{Store: store}
		version, err := service.Reject(r.Context(), chapter, strings.TrimSpace(r.Header.Get("Idempotency-Key")), versionID, input.Reason)
		if err != nil {
			failure := chapterVersionFailure(err)
			return failure.Status, nil, failure
		}
		return http.StatusOK, chapterversion.AcceptResult{Version: version}, nil
	})
}

func (s *Server) handleChapterVersionFinalize(w http.ResponseWriter, r *http.Request) {
	s.handleChapterCoordinatorAction(w, r, "chapter.version.finalize", func(coordinator *chapterversion.Coordinator, chapter int, key, versionID string, body []byte) (int, any, error) {
		var empty struct{}
		if failure := decodeJSONBody(body, &empty, true); failure != nil {
			return failure.Status, nil, apiFailureError(*failure)
		}
		result, err := coordinator.Finalize(r.Context(), chapter, key, versionID)
		return http.StatusOK, result, err
	})
}

type chapterCoordinatorAction func(*chapterversion.Coordinator, int, string, string, []byte) (int, any, error)

func (s *Server) handleChapterCoordinatorAction(w http.ResponseWriter, r *http.Request, operation string, action chapterCoordinatorAction) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	projectID, chapter, failure := s.chapterVersionIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	versionID := strings.TrimSpace(r.PathValue("version"))
	if versionID == "" {
		writeFailure(w, r, apiFailure{Status: http.StatusBadRequest, Code: "CHAPTER_VERSION_VALIDATION_FAILED", Message: "version is required"})
		return
	}
	s.executeIdempotent(w, r, operation, projectID, func(body []byte) (int, any, *apiFailure) {
		coordinator, cleanup, failure := s.chapterVersionCoordinator(r, projectID)
		if failure != nil {
			return failure.Status, nil, failure
		}
		defer cleanup()
		status, result, err := action(coordinator, chapter, strings.TrimSpace(r.Header.Get("Idempotency-Key")), versionID, body)
		if err != nil {
			var wrapped *apiFailureAsError
			if errors.As(err, &wrapped) {
				return wrapped.Failure.Status, nil, &wrapped.Failure
			}
			failure := chapterVersionFailure(err)
			return failure.Status, nil, failure
		}
		return status, result, nil
	})
}

func (s *Server) handleChapterSyncStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	projectID, chapter, failure := s.chapterVersionIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	store, cleanup, failure := s.chapterVersionStore(r, projectID)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer cleanup()
	_, _ = store.BootstrapLegacyFinal(r.Context(), chapter)
	status, err := store.DetectExternal(r.Context(), chapter)
	if err != nil {
		writeFailure(w, r, *chapterVersionFailure(err))
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleChapterSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	projectID, chapter, failure := s.chapterVersionIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	s.executeIdempotent(w, r, "chapter.version.external_sync", projectID, func(body []byte) (int, any, *apiFailure) {
		var input struct {
			ObservedSHA string `json:"observed_sha,omitempty"`
		}
		if failure := decodeJSONBody(body, &input, true); failure != nil {
			return failure.Status, nil, failure
		}
		coordinator, cleanup, failure := s.chapterVersionCoordinator(r, projectID)
		if failure != nil {
			return failure.Status, nil, failure
		}
		defer cleanup()
		result, err := coordinator.SyncExternal(r.Context(), chapter, strings.TrimSpace(r.Header.Get("Idempotency-Key")), strings.TrimSpace(input.ObservedSHA))
		if err != nil {
			failure := chapterVersionFailure(err)
			return failure.Status, nil, failure
		}
		return http.StatusOK, result, nil
	})
}

func (s *Server) handleChapterRebuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	projectID, chapter, failure := s.chapterVersionIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	store, cleanup, failure := s.chapterVersionStore(r, projectID)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer cleanup()
	rebuild, ok, err := store.LatestRebuild(r.Context(), chapter)
	if err != nil {
		writeFailure(w, r, *chapterVersionFailure(err))
		return
	}
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "boundary_chapter": chapter})
		return
	}
	writeJSON(w, http.StatusOK, rebuild)
}

func (s *Server) handleChapterPlanImpact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	projectID, chapter, failure := s.chapterVersionIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	limit, offset, failure := parseBoundedPage(r, 50, 100)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	store, cleanup, failure := s.chapterVersionStore(r, projectID)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer cleanup()
	items, total, err := store.ListPlanImpacts(r.Context(), chapter, limit, offset)
	if err != nil {
		writeFailure(w, r, *chapterVersionFailure(err))
		return
	}
	view := chapterPlanImpactView{Impacts: items, Total: total, Limit: limit, Offset: offset}
	if offset+len(items) < total {
		next := offset + len(items)
		view.NextOffset = &next
	}
	writeJSON(w, http.StatusOK, view)
}

func chapterVersionFailure(err error) *apiFailure {
	var versionErr *chapterversion.Error
	if errors.As(err, &versionErr) {
		status := http.StatusInternalServerError
		switch versionErr.Code {
		case chapterversion.CodeValidation, chapterversion.CodeDiffCursorInvalid:
			status = http.StatusBadRequest
		case chapterversion.CodeNotFound:
			status = http.StatusNotFound
		case chapterversion.CodeDiffTooLarge, chapterversion.CodeExternalTooLarge:
			status = http.StatusRequestEntityTooLarge
		case chapterversion.CodeUnsafePath, chapterversion.CodeExternalEncoding:
			status = http.StatusUnprocessableEntity
		case chapterversion.CodeConflict, chapterversion.CodeImmutable, chapterversion.CodeRejected,
			chapterversion.CodeActiveFinalConflict, chapterversion.CodeFinalizeNotAllowed, chapterversion.CodeContinuityBlocked,
			chapterversion.CodeTruthConflict, chapterversion.CodeSHAMismatch, chapterversion.CodeExternalChange,
			chapterversion.CodeSyncRequired, chapterversion.CodeSyncContentChanged, chapterversion.CodeRebuildInProgress,
			chapterversion.CodeRebuildFailed, chapterversion.CodeStaleDerivedState, chapterversion.CodeIdempotencyConflict:
			status = http.StatusConflict
		}
		return &apiFailure{Status: status, Code: versionErr.Code, Message: versionErr.Message, Retryable: versionErr.Retryable}
	}
	return ptrFailure(internalFailure())
}

type apiFailureAsError struct{ Failure apiFailure }
func (e *apiFailureAsError) Error() string { return e.Failure.Message }
func apiFailureError(failure apiFailure) error { return &apiFailureAsError{Failure: failure} }
