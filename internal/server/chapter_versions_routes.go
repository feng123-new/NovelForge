package server

import (
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
		return "", 0, &apiFailure{
			Status:  http.StatusBadRequest,
			Code:    "CHAPTER_VERSION_VALIDATION_FAILED",
			Message: "project and positive chapter are required",
		}
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
	cleanup := func() {
		_ = store.Close()
	}
	return store, cleanup, nil
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
	coordinator := &chapterversion.Coordinator{}
	coordinator.Store = store
	coordinator.Truth = quality.Truth
	coordinator.Ledger = ledger
	coordinator.Librarian = quality.Librarian
	coordinator.Continuity = quality.Continuity
	coordinator.Editor = quality.Editor
	coordinator.FinalWriter = quality.FinalWriter
	cleanup := func() {
		qualityCleanup()
		_ = store.Close()
	}
	return coordinator, cleanup, nil
}
