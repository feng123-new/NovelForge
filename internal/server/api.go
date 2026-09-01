package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/project"
	"github.com/voocel/ainovel-cli/internal/server/idempotency"
)

const maxRequestBody = 1 << 20

var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

type traceContextKey struct{}

// APIErrorEnvelope is the only REST error shape.
type APIErrorEnvelope struct {
	Error APIError `json:"error"`
}

// APIError is safe to serialize. It never carries raw paths, SQL or secrets.
type APIError struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details"`
	Retryable bool           `json:"retryable"`
	TraceID   string         `json:"trace_id"`
}

type apiFailure struct {
	Status    int
	Code      string
	Message   string
	Details   map[string]any
	Retryable bool
}

func traceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := newTraceID()
		w.Header().Set("X-Trace-ID", traceID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), traceContextKey{}, traceID)))
	})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				writeFailure(w, r, apiFailure{
					Status:    http.StatusInternalServerError,
					Code:      "INTERNAL_ERROR",
					Message:   "an internal error occurred",
					Retryable: false,
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func newTraceID() string {
	buffer := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, buffer); err == nil {
		return hex.EncodeToString(buffer)
	}
	return fmt.Sprintf("%x", time.Now().UTC().UnixNano())
}

func traceIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	value, _ := r.Context().Value(traceContextKey{}).(string)
	return value
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	body, err := encodeJSON(value)
	if err != nil {
		http.Error(w, "internal encoding error", http.StatusInternalServerError)
		return
	}
	writeJSONBytes(w, status, body)
}

func encodeJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(true)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func writeJSONBytes(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeFailure(w http.ResponseWriter, r *http.Request, failure apiFailure) {
	if failure.Status == 0 {
		failure.Status = http.StatusInternalServerError
	}
	if failure.Code == "" {
		failure.Code = "INTERNAL_ERROR"
	}
	if failure.Message == "" {
		failure.Message = "an internal error occurred"
	}
	if failure.Details == nil {
		failure.Details = map[string]any{}
	}
	writeJSON(w, failure.Status, APIErrorEnvelope{
		Error: APIError{
			Code:      failure.Code,
			Message:   failure.Message,
			Details:   failure.Details,
			Retryable: failure.Retryable,
			TraceID:   traceIDFromRequest(r),
		},
	})
}

func writeAPIError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeFailure(w, r, apiFailure{
		Status:  status,
		Code:    code,
		Message: message,
	})
}

func writeMethodNotAllowed(w http.ResponseWriter, r *http.Request, allowed ...string) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeAPIError(
		w,
		r,
		http.StatusMethodNotAllowed,
		"METHOD_NOT_ALLOWED",
		"method not allowed",
	)
}

func readRequestBody(r *http.Request) ([]byte, *apiFailure) {
	if r.Body == nil {
		return nil, nil
	}
	reader := io.LimitReader(r.Body, maxRequestBody+1)
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, &apiFailure{
			Status:  http.StatusBadRequest,
			Code:    "REQUEST_BODY_INVALID",
			Message: "request body could not be read",
		}
	}
	if len(body) > maxRequestBody {
		return nil, &apiFailure{
			Status:  http.StatusRequestEntityTooLarge,
			Code:    "REQUEST_BODY_TOO_LARGE",
			Message: "request body exceeds the one MiB limit",
		}
	}
	return body, nil
}

func decodeJSONBody(body []byte, target any, allowEmpty bool) *apiFailure {
	if len(bytes.TrimSpace(body)) == 0 {
		if allowEmpty {
			body = []byte("{}")
		} else {
			return &apiFailure{
				Status:  http.StatusBadRequest,
				Code:    "REQUEST_BODY_REQUIRED",
				Message: "a JSON request body is required",
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return &apiFailure{
			Status:  http.StatusBadRequest,
			Code:    "REQUEST_BODY_INVALID",
			Message: "request body is not valid for this operation",
		}
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return &apiFailure{
			Status:  http.StatusBadRequest,
			Code:    "REQUEST_BODY_INVALID",
			Message: "request body must contain exactly one JSON object",
		}
	}
	return nil
}

type idempotentOperation func(body []byte) (status int, response any, failure *apiFailure)

func (s *Server) executeIdempotent(
	w http.ResponseWriter,
	r *http.Request,
	operation string,
	projectID string,
	handler idempotentOperation,
) {
	body, bodyFailure := readRequestBody(r)
	if bodyFailure != nil {
		writeFailure(w, r, *bodyFailure)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		writeAPIError(
			w,
			r,
			http.StatusBadRequest,
			"IDEMPOTENCY_KEY_REQUIRED",
			"Idempotency-Key header is required",
		)
		return
	}
	if !idempotencyKeyPattern.MatchString(key) {
		writeAPIError(
			w,
			r,
			http.StatusBadRequest,
			"IDEMPOTENCY_KEY_INVALID",
			"Idempotency-Key must be 1-128 safe ASCII characters",
		)
		return
	}
	requestHash := idempotency.RequestHash(r.Method, r.URL.RequestURI(), body)
	begin, err := s.idempotency.Begin(
		r.Context(),
		key,
		operation,
		projectID,
		requestHash,
	)
	switch {
	case errors.Is(err, idempotency.ErrConflict):
		writeAPIError(
			w,
			r,
			http.StatusConflict,
			"IDEMPOTENCY_KEY_CONFLICT",
			"Idempotency-Key is already bound to a different request",
		)
		return
	case errors.Is(err, idempotency.ErrInProgress):
		writeFailure(w, r, apiFailure{
			Status:    http.StatusConflict,
			Code:      "IDEMPOTENCY_REQUEST_IN_PROGRESS",
			Message:   "an identical request is already in progress",
			Retryable: true,
		})
		return
	case err != nil:
		writeFailure(w, r, internalFailure())
		return
	case !begin.Execute:
		w.Header().Set("Idempotency-Replayed", "true")
		writeJSONBytes(w, begin.ResponseStatus, begin.ResponseBody)
		return
	}

	status, response, failure := handler(body)
	var responseBody []byte
	if failure != nil {
		envelope := APIErrorEnvelope{
			Error: APIError{
				Code:      failure.Code,
				Message:   failure.Message,
				Details:   safeDetails(failure.Details),
				Retryable: failure.Retryable,
				TraceID:   traceIDFromRequest(r),
			},
		}
		responseBody, err = encodeJSON(envelope)
		status = failure.Status
	} else {
		responseBody, err = encodeJSON(response)
	}
	if err != nil {
		failure = ptrFailure(internalFailure())
		status = failure.Status
		responseBody, _ = encodeJSON(APIErrorEnvelope{
			Error: APIError{
				Code:      failure.Code,
				Message:   failure.Message,
				Details:   map[string]any{},
				Retryable: failure.Retryable,
				TraceID:   traceIDFromRequest(r),
			},
		})
	}
	if err := s.idempotency.Complete(
		r.Context(),
		key,
		requestHash,
		status,
		responseBody,
	); err != nil {
		writeFailure(w, r, internalFailure())
		return
	}
	writeJSONBytes(w, status, responseBody)
}

func projectFailure(err error) *apiFailure {
	var projectError *project.Error
	if errors.As(err, &projectError) {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, project.ErrNotFound):
			status = http.StatusNotFound
		case errors.Is(err, project.ErrConflict):
			status = http.StatusConflict
		case errors.Is(err, project.ErrValidation),
			errors.Is(err, project.ErrConfirmation):
			status = http.StatusBadRequest
		case errors.Is(err, project.ErrUnsafePath),
			errors.Is(err, project.ErrWorkspaceRoot):
			status = http.StatusForbidden
		}
		return &apiFailure{
			Status:  status,
			Code:    projectError.Code,
			Message: projectError.Message,
		}
	}
	return ptrFailure(internalFailure())
}

func internalFailure() apiFailure {
	return apiFailure{
		Status:    http.StatusInternalServerError,
		Code:      "INTERNAL_ERROR",
		Message:   "an internal error occurred",
		Retryable: false,
	}
}

func ptrFailure(value apiFailure) *apiFailure { return &value }

func safeDetails(details map[string]any) map[string]any {
	if details == nil {
		return map[string]any{}
	}
	// Details are built only from server-owned constants and scalar validation
	// data. Make a shallow copy so callers cannot mutate a response after write.
	result := make(map[string]any, len(details))
	for key, value := range details {
		result[key] = value
	}
	return result
}
