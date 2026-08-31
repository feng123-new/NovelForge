package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/voocel/ainovel-cli/internal/project"
)

// Aliases preserve the original server package API for downstream callers.
type ProjectSummary = project.Summary
type ProjectDetail = project.Project

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listProjects(w, r)
	case http.MethodPost:
		s.createProject(w, r)
	default:
		writeMethodNotAllowed(w, r, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleProject(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/projects/")
	segments := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(segments) == 0 || segments[0] == "" || len(segments) > 2 {
		writeAPIError(w, r, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found")
		return
	}
	projectID := segments[0]
	if len(segments) == 1 {
		switch r.Method {
		case http.MethodGet:
			s.getProject(w, r, projectID)
		case http.MethodPatch:
			s.updateProject(w, r, projectID)
		case http.MethodDelete:
			s.deleteProject(w, r, projectID)
		default:
			writeMethodNotAllowed(w, r, http.MethodGet, http.MethodPatch, http.MethodDelete)
		}
		return
	}
	switch segments[1] {
	case "archive":
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, r, http.MethodPost)
			return
		}
		s.archiveProject(w, r, projectID, true)
	case "unarchive":
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, r, http.MethodPost)
			return
		}
		s.archiveProject(w, r, projectID, false)
	case "duplicate":
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, r, http.MethodPost)
			return
		}
		s.duplicateProject(w, r, projectID)
	default:
		writeAPIError(w, r, http.StatusNotFound, "API_ROUTE_NOT_FOUND", "API route not found")
	}
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	options := project.ListOptions{
		Limit:  50,
		Offset: 0,
		Query:  r.URL.Query().Get("query"),
	}
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil || limit <= 0 || limit > 100 {
			writeAPIError(
				w,
				r,
				http.StatusBadRequest,
				"PAGINATION_INVALID",
				"limit must be between 1 and 100",
			)
			return
		}
		options.Limit = limit
	}
	if value := strings.TrimSpace(r.URL.Query().Get("offset")); value != "" {
		offset, err := strconv.Atoi(value)
		if err != nil || offset < 0 {
			writeAPIError(
				w,
				r,
				http.StatusBadRequest,
				"PAGINATION_INVALID",
				"offset must not be negative",
			)
			return
		}
		options.Offset = offset
	}
	if value := strings.TrimSpace(r.URL.Query().Get("archived")); value != "" {
		switch value {
		case "true":
			archived := true
			options.Archived = &archived
		case "false":
			archived := false
			options.Archived = &archived
		case "all":
		default:
			writeAPIError(
				w,
				r,
				http.StatusBadRequest,
				"PROJECT_FILTER_INVALID",
				"archived must be true, false or all",
			)
			return
		}
	}
	result, err := s.projects.List(r.Context(), options)
	if err != nil {
		writeFailure(w, r, *projectFailure(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	s.executeIdempotent(
		w,
		r,
		"project.create",
		"",
		func(body []byte) (int, any, *apiFailure) {
			var input project.CreateInput
			if failure := decodeJSONBody(body, &input, false); failure != nil {
				return failure.Status, nil, failure
			}
			created, err := s.projects.Create(r.Context(), input)
			if err != nil {
				failure := projectFailure(err)
				return failure.Status, nil, failure
			}
			if _, err := s.events.PublishContext(
				r.Context(),
				"project.created",
				created.ID,
				projectEventData(created),
			); err != nil {
				failure := ptrFailure(internalFailure())
				return failure.Status, nil, failure
			}
			return http.StatusCreated, created, nil
		},
	)
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request, projectID string) {
	projectDetail, err := s.projects.Get(r.Context(), projectID)
	if err != nil {
		writeFailure(w, r, *projectFailure(err))
		return
	}
	writeJSON(w, http.StatusOK, projectDetail)
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request, projectID string) {
	s.executeIdempotent(
		w,
		r,
		"project.update",
		projectID,
		func(body []byte) (int, any, *apiFailure) {
			var input project.UpdateInput
			if failure := decodeJSONBody(body, &input, false); failure != nil {
				return failure.Status, nil, failure
			}
			updated, err := s.projects.Update(r.Context(), projectID, input)
			if err != nil {
				failure := projectFailure(err)
				return failure.Status, nil, failure
			}
			if _, err := s.events.PublishContext(
				r.Context(),
				"project.updated",
				updated.ID,
				projectEventData(updated),
			); err != nil {
				failure := ptrFailure(internalFailure())
				return failure.Status, nil, failure
			}
			return http.StatusOK, updated, nil
		},
	)
}

func (s *Server) archiveProject(
	w http.ResponseWriter,
	r *http.Request,
	projectID string,
	archived bool,
) {
	operation := "project.unarchive"
	eventType := "project.unarchived"
	if archived {
		operation = "project.archive"
		eventType = "project.archived"
	}
	s.executeIdempotent(
		w,
		r,
		operation,
		projectID,
		func(body []byte) (int, any, *apiFailure) {
			var empty struct{}
			if failure := decodeJSONBody(body, &empty, true); failure != nil {
				return failure.Status, nil, failure
			}
			updated, err := s.projects.SetArchived(r.Context(), projectID, archived)
			if err != nil {
				failure := projectFailure(err)
				return failure.Status, nil, failure
			}
			if _, err := s.events.PublishContext(
				r.Context(),
				eventType,
				updated.ID,
				projectEventData(updated),
			); err != nil {
				failure := ptrFailure(internalFailure())
				return failure.Status, nil, failure
			}
			return http.StatusOK, updated, nil
		},
	)
}

func (s *Server) duplicateProject(w http.ResponseWriter, r *http.Request, projectID string) {
	s.executeIdempotent(
		w,
		r,
		"project.duplicate",
		projectID,
		func(body []byte) (int, any, *apiFailure) {
			var input project.DuplicateInput
			if failure := decodeJSONBody(body, &input, true); failure != nil {
				return failure.Status, nil, failure
			}
			duplicated, err := s.projects.Duplicate(r.Context(), projectID, input)
			if err != nil {
				failure := projectFailure(err)
				return failure.Status, nil, failure
			}
			if _, err := s.events.PublishContext(
				r.Context(),
				"project.duplicated",
				duplicated.ID,
				map[string]any{
					"id":        duplicated.ID,
					"title":     duplicated.Title,
					"source_id": projectID,
				},
			); err != nil {
				failure := ptrFailure(internalFailure())
				return failure.Status, nil, failure
			}
			return http.StatusCreated, duplicated, nil
		},
	)
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request, projectID string) {
	s.executeIdempotent(
		w,
		r,
		"project.delete",
		projectID,
		func(body []byte) (int, any, *apiFailure) {
			var input project.DeleteInput
			if failure := decodeJSONBody(body, &input, false); failure != nil {
				return failure.Status, nil, failure
			}
			result, err := s.projects.Delete(r.Context(), projectID, input)
			if err != nil {
				failure := projectFailure(err)
				return failure.Status, nil, failure
			}
			if _, err := s.events.PublishContext(
				r.Context(),
				"audit.project.deleted",
				projectID,
				map[string]any{
					"id":        projectID,
					"deleted":   true,
					"permanent": result.Permanent,
				},
			); err != nil {
				failure := ptrFailure(internalFailure())
				return failure.Status, nil, failure
			}
			return http.StatusOK, result, nil
		},
	)
}

func projectEventData(value project.Project) map[string]any {
	return map[string]any{
		"id":       value.ID,
		"title":    value.Title,
		"status":   value.Status,
		"archived": value.Archived,
	}
}
