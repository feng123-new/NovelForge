package server

import (
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/voocel/ainovel-cli/internal/models"
	"github.com/voocel/ainovel-cli/internal/project"
)

// ModelList is a stable, bounded page of the runtime model registry.
type ModelList struct {
	Models     []models.ModelEntry `json:"models"`
	Total      int                 `json:"total"`
	Limit      int                 `json:"limit"`
	Offset     int                 `json:"offset"`
	NextOffset *int                `json:"next_offset,omitempty"`
}

// WorkspaceSettings is intentionally secret-free and path-free.
type WorkspaceSettings struct {
	Product       string         `json:"product"`
	Version       string         `json:"version"`
	APIVersion    string         `json:"api_version"`
	Workspace     string         `json:"workspace"`
	ListenHost    string         `json:"listen_host"`
	ListenPort    int            `json:"listen_port"`
	LoopbackOnly  bool           `json:"loopback_only"`
	ThemeStorage  string         `json:"theme_storage"`
	RequestLimits map[string]int `json:"request_limits"`
	Capabilities  map[string]any `json:"capabilities"`
}

func (s *Server) registerWorkspaceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/models", s.handleModels)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/projects/{id}/chapters", s.handleChapterCollection)
	mux.HandleFunc("/api/projects/{id}/foundation", s.handleFoundationRequest)
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	limit, offset, failure := parseBoundedPage(r, 50, 100)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	entries := models.DefaultRegistry().List(strings.TrimSpace(r.URL.Query().Get("query")))
	sort.Slice(entries, func(i, j int) bool {
		left := strings.ToLower(entries[i].Provider + "/" + entries[i].ID)
		right := strings.ToLower(entries[j].Provider + "/" + entries[j].ID)
		return left < right
	})
	result := ModelList{Models: []models.ModelEntry{}, Total: len(entries), Limit: limit, Offset: offset}
	if offset < len(entries) {
		end := offset + limit
		if end > len(entries) {
			end = len(entries)
		}
		result.Models = append(result.Models, entries[offset:end]...)
		if end < len(entries) {
			next := end
			result.NextOffset = &next
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, WorkspaceSettings{
		Product:      productName,
		Version:      s.cfg.Version,
		APIVersion:   apiVersion,
		Workspace:    s.workspaceLabel,
		ListenHost:   s.cfg.Host,
		ListenPort:   s.cfg.Port,
		LoopbackOnly: isLoopbackAddress(s.cfg.Host),
		ThemeStorage: "browser-preference-only",
		RequestLimits: map[string]int{
			"body_bytes":     maxRequestBody,
			"collection_max": 100,
		},
		Capabilities: map[string]any{
			"project_lifecycle":             true,
			"durable_events":                true,
			"formal_web_workspace":          true,
			"foundation_request_storage":    true,
			"foundation_worker_available":   false,
			"autopilot_worker_available":    false,
			"credentials_exposed_to_web":    false,
			"authoritative_state_is_server": true,
		},
	})
}

func (s *Server) handleChapterCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	projectID := strings.TrimSpace(r.PathValue("id"))
	limit, offset, failure := parseBoundedPage(r, 50, 100)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	result, err := s.projects.ListChapters(r.Context(), projectID, limit, offset)
	if err != nil {
		writeFailure(w, r, *projectFailure(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFoundationRequest(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("id"))
	switch r.Method {
	case http.MethodGet:
		request, err := s.projects.GetFoundationRequest(r.Context(), projectID)
		if err != nil {
			writeFailure(w, r, *projectFailure(err))
			return
		}
		writeJSON(w, http.StatusOK, request)
	case http.MethodPost:
		s.executeIdempotent(
			w,
			r,
			"project.foundation.request",
			projectID,
			func(body []byte) (int, any, *apiFailure) {
				var input project.FoundationRequestInput
				if failure := decodeJSONBody(body, &input, false); failure != nil {
					return failure.Status, nil, failure
				}
				request, err := s.projects.SaveFoundationRequest(r.Context(), projectID, input)
				if err != nil {
					failure := projectFailure(err)
					return failure.Status, nil, failure
				}
				if _, err := s.events.PublishContext(
					r.Context(),
					"foundation.requested",
					projectID,
					map[string]any{
						"id":               request.ID,
						"status":           request.Status,
						"worker_available": request.Automation.WorkerAvailable,
					},
				); err != nil {
					failure := ptrFailure(internalFailure())
					return failure.Status, nil, failure
				}
				return http.StatusAccepted, request, nil
			},
		)
	default:
		writeMethodNotAllowed(w, r, http.MethodGet, http.MethodPost)
	}
}

func parseBoundedPage(r *http.Request, defaultLimit int, maxLimit int) (int, int, *apiFailure) {
	limit := defaultLimit
	offset := 0
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 || parsed > maxLimit {
			return 0, 0, &apiFailure{
				Status:  http.StatusBadRequest,
				Code:    "PAGINATION_INVALID",
				Message: "limit is outside the allowed range",
			}
		}
		limit = parsed
	}
	if value := strings.TrimSpace(r.URL.Query().Get("offset")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			return 0, 0, &apiFailure{
				Status:  http.StatusBadRequest,
				Code:    "PAGINATION_INVALID",
				Message: "offset must not be negative",
			}
		}
		offset = parsed
	}
	return limit, offset, nil
}

func isLoopbackAddress(host string) bool {
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
