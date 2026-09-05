package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/voocel/ainovel-cli/internal/authoring"
)

func (s *Server) registerAuthoringRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/projects/{id}/authoring", s.handleAuthoring)
	mux.HandleFunc("/api/projects/{id}/authoring/search", s.handleAuthoringSearch)
	mux.HandleFunc("/api/projects/{id}/authoring/lint", s.handleAuthoringLint)
}
func authoringFailure(err error) *apiFailure {
	switch {
	case errors.Is(err, authoring.ErrValidation):
		return &apiFailure{Status: 400, Code: "AUTHORING_INPUT_INVALID", Message: "authoring type, size, range or rule is invalid"}
	case errors.Is(err, authoring.ErrConflict):
		return &apiFailure{Status: 409, Code: "AUTHORING_REVISION_CONFLICT", Message: "refresh the library before editing; the revision or request key has changed"}
	case errors.Is(err, authoring.ErrNotFound):
		return &apiFailure{Status: 404, Code: "AUTHORING_NOT_FOUND", Message: "authoring entry not found"}
	default:
		return &apiFailure{Status: 500, Code: "AUTHORING_STORAGE_ERROR", Message: "authoring operation could not be completed"}
	}
}
func (s *Server) handleAuthoring(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.projects.Get(r.Context(), id)
	if err != nil {
		writeFailure(w, r, *projectFailure(err))
		return
	}
	store, err := s.projects.OpenAuthoring(r.Context(), id)
	if err != nil {
		writeFailure(w, r, *authoringFailure(err))
		return
	}
	defer store.Close()
	switch r.Method {
	case http.MethodGet:
		limit, offset, f := parseBoundedPage(r, 50, 100)
		if f != nil {
			writeFailure(w, r, *f)
			return
		}
		state, err := store.State(r.Context(), r.URL.Query().Get("kind"), limit, offset)
		if err != nil {
			writeFailure(w, r, *authoringFailure(err))
			return
		}
		writeJSON(w, 200, state)
	case http.MethodPost:
		if p.Archived {
			writeFailure(w, r, apiFailure{Status: 409, Code: "PROJECT_ARCHIVED", Message: "restore project before editing authoring resources"})
			return
		}
		s.executeIdempotent(w, r, "authoring.mutate", id, func(body []byte) (int, any, *apiFailure) {
			var m authoring.Mutation
			if f := decodeJSONBody(body, &m, false); f != nil {
				return f.Status, nil, f
			}
			result, err := store.Mutate(r.Context(), r.Header.Get("Idempotency-Key"), m)
			if err != nil {
				f := authoringFailure(err)
				return f.Status, nil, f
			}
			return 200, result, nil
		})
	default:
		writeMethodNotAllowed(w, r, http.MethodGet, http.MethodPost)
	}
}
func (s *Server) handleAuthoringSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	id := r.PathValue("id")
	if _, err := s.projects.Get(r.Context(), id); err != nil {
		writeFailure(w, r, *projectFailure(err))
		return
	}
	chapter, err := strconv.Atoi(r.URL.Query().Get("chapter"))
	if err != nil {
		writeFailure(w, r, *authoringFailure(authoring.ErrValidation))
		return
	}
	limit, offset, f := parseBoundedPage(r, 20, 100)
	if f != nil {
		writeFailure(w, r, *f)
		return
	}
	store, err := s.projects.OpenAuthoring(r.Context(), id)
	if err != nil {
		writeFailure(w, r, *authoringFailure(err))
		return
	}
	defer store.Close()
	entries, err := store.Search(r.Context(), r.URL.Query().Get("kind"), r.URL.Query().Get("q"), chapter, r.URL.Query().Get("pov"), limit, offset)
	if err != nil {
		writeFailure(w, r, *authoringFailure(err))
		return
	}
	writeJSON(w, 200, map[string]any{"entries": entries, "limit": limit, "offset": offset})
}
func (s *Server) handleAuthoringLint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	id := r.PathValue("id")
	if _, err := s.projects.Get(r.Context(), id); err != nil {
		writeFailure(w, r, *projectFailure(err))
		return
	}
	s.executeIdempotent(w, r, "authoring.lint", id, func(body []byte) (int, any, *apiFailure) {
		var in struct {
			Chapter int    `json:"chapter"`
			Text    string `json:"text"`
		}
		if f := decodeJSONBody(body, &in, false); f != nil {
			return f.Status, nil, f
		}
		store, err := s.projects.OpenAuthoring(r.Context(), id)
		if err != nil {
			f := authoringFailure(err)
			return f.Status, nil, f
		}
		defer store.Close()
		state, err := store.State(r.Context(), "", 1, 0)
		if err != nil {
			f := authoringFailure(err)
			return f.Status, nil, f
		}
		report, err := s.projects.AuthoringLint(r.Context(), id, in.Chapter, in.Text, state.Rules)
		if err != nil {
			f := authoringFailure(err)
			return f.Status, nil, f
		}
		return 200, map[string]any{"revision": state.Revision, "report": report}, nil
	})
}
