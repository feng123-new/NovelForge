package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/voocel/ainovel-cli/internal/chapterversion"
)

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
	syncStatus := chapterversion.SyncStatus{}
	syncStatus.ProjectID = projectID
	syncStatus.Chapter = chapter
	if active != nil {
		syncStatus, err = store.DetectExternal(r.Context(), chapter)
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
	view := chapterversion.ChapterView{}
	view.ProjectID = projectID
	view.Chapter = chapter
	view.ActiveFinal = active
	view.Latest = latest
	view.VersionCount = versions.Total
	view.Sync = syncStatus
	view.DerivedState = derived
	writeJSON(w, http.StatusOK, view)
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
			value, parseErr := strconv.ParseBool(raw)
			if parseErr != nil {
				writeFailure(w, r, apiFailure{
					Status:  http.StatusBadRequest,
					Code:    "CHAPTER_VERSION_VALIDATION_FAILED",
					Message: "include_content must be a boolean",
				})
				return
			}
			includeContent = value
		}
		options := chapterversion.ListOptions{}
		options.Limit = limit
		options.Offset = offset
		options.Status = strings.TrimSpace(r.URL.Query().Get("status"))
		options.Type = chapterversion.VersionType(strings.TrimSpace(r.URL.Query().Get("type")))
		options.AuthorType = chapterversion.AuthorType(strings.TrimSpace(r.URL.Query().Get("author_type")))
		options.IncludeContent = includeContent
		result, err := store.List(r.Context(), chapter, options)
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
			if bodyFailure := decodeJSONBody(body, &input, false); bodyFailure != nil {
				return bodyFailure.Status, nil, bodyFailure
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
	store, cleanup, failure := s.chapterVersionStore(r, projectID)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer cleanup()
	version, err := store.Get(r.Context(), chapter, strings.TrimSpace(r.PathValue("version")), true)
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
		writeFailure(w, r, apiFailure{
			Status:  http.StatusBadRequest,
			Code:    "CHAPTER_VERSION_VALIDATION_FAILED",
			Message: "from_version and to_version are required",
		})
		return
	}
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 500 {
			writeFailure(w, r, apiFailure{
				Status:  http.StatusBadRequest,
				Code:    "CHAPTER_VERSION_VALIDATION_FAILED",
				Message: "diff limit must be between 1 and 500",
			})
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
	mode := chapterversion.DiffMode(strings.TrimSpace(r.URL.Query().Get("mode")))
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	result, err := store.Diff(r.Context(), chapter, fromID, toID, mode, cursor, limit)
	if err != nil {
		writeFailure(w, r, *chapterVersionFailure(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
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
	view := chapterPlanImpactView{}
	view.Impacts = items
	view.Total = total
	view.Limit = limit
	view.Offset = offset
	if offset+len(items) < total {
		next := offset + len(items)
		view.NextOffset = &next
	}
	writeJSON(w, http.StatusOK, view)
}
