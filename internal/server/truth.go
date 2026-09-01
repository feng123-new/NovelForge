package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/voocel/ainovel-cli/internal/project"
	"github.com/voocel/ainovel-cli/internal/truthstore"
)

type truthAppendRequest struct {
	ProjectID string                 `json:"project_id"`
	Event     truthstore.AppendInput `json:"event"`
}

type truthRebuildRequest struct {
	ProjectID   string `json:"project_id"`
	FromChapter int    `json:"from_chapter"`
}

type truthBatchRequest struct {
	ProjectID string                  `json:"project_id"`
	Queries   []truthstore.StateQuery `json:"queries"`
}

func (s *Server) handleTruth(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/truth")
	switch path {
	case "/events":
		switch r.Method {
		case http.MethodGet:
			s.handleTruthEvents(w, r)
		case http.MethodPost:
			s.handleTruthAppend(w, r)
		default:
			writeMethodNotAllowed(w, r, http.MethodGet, http.MethodPost)
		}
	case "/state":
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w, r, http.MethodGet)
			return
		}
		s.handleTruthState(w, r)
	case "/state:batch":
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, r, http.MethodPost)
			return
		}
		s.handleTruthBatch(w, r)
	case "/conflicts":
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w, r, http.MethodGet)
			return
		}
		s.handleTruthConflicts(w, r)
	case "/rebuild":
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, r, http.MethodPost)
			return
		}
		s.handleTruthRebuild(w, r)
	case "/verify":
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w, r, http.MethodGet)
			return
		}
		s.handleTruthVerify(w, r)
	default:
		writeAPIError(w, r, http.StatusNotFound, "API_ROUTE_NOT_FOUND", "API route not found")
	}
}

func (s *Server) handleTruthAppend(w http.ResponseWriter, r *http.Request) {
	if !requireTruthIdempotencyKey(w, r) {
		return
	}
	body, failure := readRequestBody(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	var input truthAppendRequest
	if failure := decodeJSONBody(body, &input, false); failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	s.executeIdempotentBody(w, r, "truth.event.append", input.ProjectID, body,
		func(_ []byte) (int, any, *apiFailure) {
			store, failure := s.openTruthStore(r.Context(), input.ProjectID)
			if failure != nil {
				return failure.Status, nil, failure
			}
			defer store.Close()
			input.Event.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
			result, err := store.Append(r.Context(), input.Event)
			if err != nil {
				failure := truthStoreFailure(err)
				return failure.Status, nil, failure
			}
			return http.StatusCreated, result, nil
		})
}

func (s *Server) handleTruthState(w http.ResponseWriter, r *http.Request) {
	chapter, ok := truthIntQuery(w, r, "chapter", true)
	if !ok {
		return
	}
	limit, offset, ok := truthPageQuery(w, r)
	if !ok {
		return
	}
	store, failure := s.openTruthStore(r.Context(), r.URL.Query().Get("project_id"))
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer store.Close()
	page, err := store.State(r.Context(), truthstore.StateQuery{
		Chapter: chapter, SubjectType: r.URL.Query().Get("subject_type"),
		SubjectID: r.URL.Query().Get("subject_id"), Predicate: r.URL.Query().Get("predicate"),
		Limit: limit, Offset: offset,
	})
	if err != nil {
		writeFailure(w, r, *truthStoreFailure(err))
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleTruthBatch(w http.ResponseWriter, r *http.Request) {
	if !requireTruthIdempotencyKey(w, r) {
		return
	}
	body, failure := readRequestBody(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	var input truthBatchRequest
	if failure := decodeJSONBody(body, &input, false); failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	s.executeIdempotentBody(w, r, "truth.state.batch", input.ProjectID, body,
		func(_ []byte) (int, any, *apiFailure) {
			store, failure := s.openTruthStore(r.Context(), input.ProjectID)
			if failure != nil {
				return failure.Status, nil, failure
			}
			defer store.Close()
			pages, err := store.StateMany(r.Context(), input.Queries)
			if err != nil {
				failure := truthStoreFailure(err)
				return failure.Status, nil, failure
			}
			return http.StatusOK, map[string]any{"results": pages}, nil
		})
}

func (s *Server) handleTruthEvents(w http.ResponseWriter, r *http.Request) {
	store, failure := s.openTruthStore(r.Context(), r.URL.Query().Get("project_id"))
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer store.Close()
	after, ok := truthInt64Query(w, r, "after_sequence")
	if !ok {
		return
	}
	limit, _, ok := truthPageQuery(w, r)
	if !ok {
		return
	}
	var throughChapter *int
	if r.URL.Query().Has("through_chapter") {
		parsed, valid := truthIntQuery(w, r, "through_chapter", true)
		if !valid {
			return
		}
		throughChapter = &parsed
	}
	page, err := store.Events(r.Context(), truthstore.EventQuery{
		AfterSequence: after, ThroughChapter: throughChapter,
		SubjectType: r.URL.Query().Get("subject_type"), SubjectID: r.URL.Query().Get("subject_id"),
		Predicate: r.URL.Query().Get("predicate"), Limit: limit,
	})
	if err != nil {
		writeFailure(w, r, *truthStoreFailure(err))
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleTruthConflicts(w http.ResponseWriter, r *http.Request) {
	store, failure := s.openTruthStore(r.Context(), r.URL.Query().Get("project_id"))
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer store.Close()
	var chapter *int
	if r.URL.Query().Has("chapter") {
		parsed, valid := truthIntQuery(w, r, "chapter", true)
		if !valid {
			return
		}
		chapter = &parsed
	}
	limit, offset, ok := truthPageQuery(w, r)
	if !ok {
		return
	}
	page, err := store.Conflicts(r.Context(), truthstore.ConflictQuery{
		Chapter: chapter, SubjectType: r.URL.Query().Get("subject_type"),
		SubjectID: r.URL.Query().Get("subject_id"), Predicate: r.URL.Query().Get("predicate"),
		Status: truthstore.ConflictStatus(strings.TrimSpace(r.URL.Query().Get("status"))),
		Limit:  limit, Offset: offset,
	})
	if err != nil {
		writeFailure(w, r, *truthStoreFailure(err))
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleTruthRebuild(w http.ResponseWriter, r *http.Request) {
	if !requireTruthIdempotencyKey(w, r) {
		return
	}
	body, failure := readRequestBody(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	var input truthRebuildRequest
	if failure := decodeJSONBody(body, &input, false); failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	s.executeIdempotentBody(w, r, "truth.projection.rebuild", input.ProjectID, body,
		func(_ []byte) (int, any, *apiFailure) {
			store, failure := s.openTruthStore(r.Context(), input.ProjectID)
			if failure != nil {
				return failure.Status, nil, failure
			}
			defer store.Close()
			result, err := store.Rebuild(r.Context(), input.FromChapter)
			if err != nil {
				failure := truthStoreFailure(err)
				return failure.Status, nil, failure
			}
			return http.StatusOK, result, nil
		})
}

func (s *Server) handleTruthVerify(w http.ResponseWriter, r *http.Request) {
	store, failure := s.openTruthStore(r.Context(), r.URL.Query().Get("project_id"))
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer store.Close()
	result, err := store.Verify(r.Context())
	if err != nil {
		writeFailure(w, r, *truthStoreFailure(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func requireTruthIdempotencyKey(w http.ResponseWriter, r *http.Request) bool {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		writeAPIError(w, r, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required")
		return false
	}
	if !idempotencyKeyPattern.MatchString(key) {
		writeAPIError(w, r, http.StatusBadRequest, "IDEMPOTENCY_KEY_INVALID", "Idempotency-Key must be 1-128 safe ASCII characters")
		return false
	}
	return true
}

func (s *Server) openTruthStore(ctx context.Context, projectID string) (*truthstore.Store, *apiFailure) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, &apiFailure{Status: http.StatusBadRequest, Code: "TRUTH_PROJECT_ID_REQUIRED", Message: "project_id is required"}
	}
	store, err := s.projects.OpenTruthStore(ctx, projectID)
	if err == nil {
		return store, nil
	}
	if errors.Is(err, project.ErrNotFound) || errors.Is(err, project.ErrUnsafePath) {
		return nil, &apiFailure{Status: http.StatusNotFound, Code: "TRUTH_PROJECT_NOT_FOUND", Message: "project is unavailable"}
	}
	return nil, &apiFailure{Status: http.StatusInternalServerError, Code: "TRUTH_STORE_UNAVAILABLE", Message: "truth store is unavailable"}
}

func truthStoreFailure(err error) *apiFailure {
	failure := &apiFailure{Status: http.StatusInternalServerError, Code: string(truthstore.CodeStorage), Message: "truth operation failed"}
	storeErr, ok := truthstore.AsError(err)
	if !ok {
		return failure
	}
	failure.Code = string(storeErr.Code)
	failure.Message = storeErr.Message
	failure.Retryable = storeErr.Retryable
	switch storeErr.Code {
	case truthstore.CodeValidation:
		failure.Status = http.StatusBadRequest
	case truthstore.CodeNotFound:
		failure.Status = http.StatusNotFound
	case truthstore.CodeConflict, truthstore.CodeAuthority, truthstore.CodeIdempotencyConflict:
		failure.Status = http.StatusConflict
	case truthstore.CodeBusy:
		failure.Status = http.StatusServiceUnavailable
	case truthstore.CodeCorrupt:
		failure.Status = http.StatusUnprocessableEntity
	}
	return failure
}

func truthIntQuery(w http.ResponseWriter, r *http.Request, name string, required bool) (int, bool) {
	value := strings.TrimSpace(r.URL.Query().Get(name))
	if value == "" {
		if required {
			writeAPIError(w, r, http.StatusBadRequest, "TRUTH_QUERY_INVALID", name+" is required")
			return 0, false
		}
		return 0, true
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		writeAPIError(w, r, http.StatusBadRequest, "TRUTH_QUERY_INVALID", name+" must be a non-negative integer")
		return 0, false
	}
	return parsed, true
}

func truthInt64Query(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	value := strings.TrimSpace(r.URL.Query().Get(name))
	if value == "" {
		return 0, true
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		writeAPIError(w, r, http.StatusBadRequest, "TRUTH_QUERY_INVALID", name+" must be a non-negative integer")
		return 0, false
	}
	return parsed, true
}

func truthPageQuery(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	limit := 100
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 500 {
			writeAPIError(w, r, http.StatusBadRequest, "TRUTH_PAGINATION_INVALID", "limit must be between 1 and 500")
			return 0, 0, false
		}
		limit = parsed
	}
	offset := 0
	if value := strings.TrimSpace(r.URL.Query().Get("offset")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			writeAPIError(w, r, http.StatusBadRequest, "TRUTH_PAGINATION_INVALID", "offset must not be negative")
			return 0, 0, false
		}
		offset = parsed
	}
	return limit, offset, true
}
