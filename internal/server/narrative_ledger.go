package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/voocel/ainovel-cli/internal/narrativeledger"
)

const ledgerRequestLimit = 1 << 20

type ledgerErrorEnvelope struct {
	Error ledgerErrorBody `json:"error"`
}

type ledgerErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	TraceID string `json:"trace_id"`
}

type foreshadowRequest struct {
	Chapter         int                             `json:"chapter"`
	Action          string                          `json:"action"`
	Key             string                          `json:"key"`
	Title           string                          `json:"title"`
	Description     string                          `json:"description"`
	Priority        *narrativeledger.Priority       `json:"priority"`
	Status          *narrativeledger.ForeshadowStatus `json:"status"`
	PlantedChapter  *int                            `json:"planted_chapter"`
	DueChapter      *int                            `json:"due_chapter"`
	RevealChapter   *int                            `json:"reveal_chapter"`
}

type secretRequest struct {
	Chapter           int                                      `json:"chapter"`
	Action            string                                   `json:"action"`
	Key               string                                   `json:"key"`
	Title             string                                   `json:"title"`
	Description       string                                   `json:"description"`
	Status            *narrativeledger.SecretStatus            `json:"status"`
	PublicFromChapter *int                                     `json:"public_from_chapter"`
	Knowledge         []narrativeledger.SecretKnowledgeChange  `json:"knowledge"`
}

func (s *Server) handleForeshadows(w http.ResponseWriter, r *http.Request) {
	store, err := s.projects.OpenNarrativeLedger(r.Context(), r.PathValue("id"))
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	defer store.Close()
	if r.Method == http.MethodGet {
		options, err := ledgerListOptions(r)
		if err != nil {
			writeLedgerError(w, r, err)
			return
		}
		page, err := store.ListForeshadows(r.Context(), options)
		if err != nil {
			writeLedgerError(w, r, err)
			return
		}
		writeLedgerJSON(w, http.StatusOK, page)
		return
	}
	idempotency, ok := requireLedgerIdempotency(w, r)
	if !ok {
		return
	}
	var input foreshadowRequest
	if err := decodeLedgerJSON(w, r, &input); err != nil {
		writeLedgerError(w, r, err)
		return
	}
	if input.Action == "" {
		input.Action = "create"
	}
	result, err := store.ApplyHuman(r.Context(), narrativeledger.ChangeSet{
		Source: narrativeledger.Source{
			TransactionID: "human:foreshadow:" + idempotency,
			CandidateID:   "human",
			Chapter:       input.Chapter,
			Authority:     narrativeledger.AuthorityHuman,
			Provenance: map[string]string{
				"transport":       "http",
				"idempotency_key": idempotency,
			},
		},
		Foreshadows: []narrativeledger.ForeshadowChange{{
			Action:          input.Action,
			Key:             input.Key,
			Title:           input.Title,
			Description:     input.Description,
			Priority:        input.Priority,
			Status:          input.Status,
			PlantedChapter:  input.PlantedChapter,
			DueChapter:      input.DueChapter,
			RevealChapter:   input.RevealChapter,
		}},
	})
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	item, err := store.GetForeshadow(r.Context(), input.Key, input.Chapter)
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	writeLedgerJSON(w, http.StatusOK, map[string]any{"result": result, "foreshadow": item})
}

func (s *Server) handleForeshadow(w http.ResponseWriter, r *http.Request) {
	store, err := s.projects.OpenNarrativeLedger(r.Context(), r.PathValue("id"))
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	defer store.Close()
	key := r.PathValue("key")
	chapter, err := ledgerChapterQuery(r, 0)
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	if r.Method == http.MethodGet {
		item, err := store.GetForeshadow(r.Context(), key, chapter)
		if err != nil {
			writeLedgerError(w, r, err)
			return
		}
		writeLedgerJSON(w, http.StatusOK, item)
		return
	}
	idempotency, ok := requireLedgerIdempotency(w, r)
	if !ok {
		return
	}
	var input foreshadowRequest
	if err := decodeLedgerJSON(w, r, &input); err != nil {
		writeLedgerError(w, r, err)
		return
	}
	input.Key = key
	if input.Action == "" {
		input.Action = "update"
	}
	result, err := store.ApplyHuman(r.Context(), narrativeledger.ChangeSet{
		Source: narrativeledger.Source{
			TransactionID: "human:foreshadow:" + idempotency,
			CandidateID:   "human",
			Chapter:       input.Chapter,
			Authority:     narrativeledger.AuthorityHuman,
			Provenance: map[string]string{"transport": "http", "idempotency_key": idempotency},
		},
		Foreshadows: []narrativeledger.ForeshadowChange{{
			Action:          input.Action,
			Key:             input.Key,
			Title:           input.Title,
			Description:     input.Description,
			Priority:        input.Priority,
			Status:          input.Status,
			PlantedChapter:  input.PlantedChapter,
			DueChapter:      input.DueChapter,
			RevealChapter:   input.RevealChapter,
		}},
	})
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	item, err := store.GetForeshadow(r.Context(), key, input.Chapter)
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	writeLedgerJSON(w, http.StatusOK, map[string]any{"result": result, "foreshadow": item})
}

func (s *Server) handleSecrets(w http.ResponseWriter, r *http.Request) {
	store, err := s.projects.OpenNarrativeLedger(r.Context(), r.PathValue("id"))
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	defer store.Close()
	if r.Method == http.MethodGet {
		options, err := ledgerListOptions(r)
		if err != nil {
			writeLedgerError(w, r, err)
			return
		}
		page, err := store.ListSecrets(r.Context(), options)
		if err != nil {
			writeLedgerError(w, r, err)
			return
		}
		writeLedgerJSON(w, http.StatusOK, page)
		return
	}
	idempotency, ok := requireLedgerIdempotency(w, r)
	if !ok {
		return
	}
	var input secretRequest
	if err := decodeLedgerJSON(w, r, &input); err != nil {
		writeLedgerError(w, r, err)
		return
	}
	if input.Action == "" {
		input.Action = "create"
	}
	result, err := store.ApplyHuman(r.Context(), narrativeledger.ChangeSet{
		Source: narrativeledger.Source{
			TransactionID: "human:secret:" + idempotency,
			CandidateID:   "human",
			Chapter:       input.Chapter,
			Authority:     narrativeledger.AuthorityHuman,
			Provenance: map[string]string{"transport": "http", "idempotency_key": idempotency},
		},
		Secrets: []narrativeledger.SecretChange{{
			Action:            input.Action,
			Key:               input.Key,
			Title:             input.Title,
			Description:       input.Description,
			Status:            input.Status,
			PublicFromChapter: input.PublicFromChapter,
			Knowledge:         input.Knowledge,
		}},
	})
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	item, err := store.GetSecret(r.Context(), input.Key, input.Chapter)
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	writeLedgerJSON(w, http.StatusOK, map[string]any{"result": result, "secret": item})
}

func (s *Server) handleSecret(w http.ResponseWriter, r *http.Request) {
	store, err := s.projects.OpenNarrativeLedger(r.Context(), r.PathValue("id"))
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	defer store.Close()
	key := r.PathValue("key")
	chapter, err := ledgerChapterQuery(r, 0)
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	if r.Method == http.MethodGet {
		item, err := store.GetSecret(r.Context(), key, chapter)
		if err != nil {
			writeLedgerError(w, r, err)
			return
		}
		writeLedgerJSON(w, http.StatusOK, item)
		return
	}
	idempotency, ok := requireLedgerIdempotency(w, r)
	if !ok {
		return
	}
	var input secretRequest
	if err := decodeLedgerJSON(w, r, &input); err != nil {
		writeLedgerError(w, r, err)
		return
	}
	input.Key = key
	if input.Action == "" {
		input.Action = "update"
	}
	result, err := store.ApplyHuman(r.Context(), narrativeledger.ChangeSet{
		Source: narrativeledger.Source{
			TransactionID: "human:secret:" + idempotency,
			CandidateID:   "human",
			Chapter:       input.Chapter,
			Authority:     narrativeledger.AuthorityHuman,
			Provenance: map[string]string{"transport": "http", "idempotency_key": idempotency},
		},
		Secrets: []narrativeledger.SecretChange{{
			Action:            input.Action,
			Key:               input.Key,
			Title:             input.Title,
			Description:       input.Description,
			Status:            input.Status,
			PublicFromChapter: input.PublicFromChapter,
			Knowledge:         input.Knowledge,
		}},
	})
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	item, err := store.GetSecret(r.Context(), key, input.Chapter)
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	writeLedgerJSON(w, http.StatusOK, map[string]any{"result": result, "secret": item})
}

func (s *Server) handleLedgerPlannerContext(w http.ResponseWriter, r *http.Request) {
	store, err := s.projects.OpenNarrativeLedger(r.Context(), r.PathValue("id"))
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	defer store.Close()
	chapter, err := ledgerChapterQuery(r, 0)
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	contextValue, err := store.BuildPlannerContext(r.Context(), chapter, 20)
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	writeLedgerJSON(w, http.StatusOK, contextValue)
}

func (s *Server) handleLedgerDashboard(w http.ResponseWriter, r *http.Request) {
	store, err := s.projects.OpenNarrativeLedger(r.Context(), r.PathValue("id"))
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	defer store.Close()
	chapter, err := ledgerChapterQuery(r, 0)
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	dashboard, err := store.Dashboard(r.Context(), chapter)
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	writeLedgerJSON(w, http.StatusOK, dashboard)
}

func (s *Server) handleLedgerDiagnostics(w http.ResponseWriter, r *http.Request) {
	store, err := s.projects.OpenNarrativeLedger(r.Context(), r.PathValue("id"))
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	defer store.Close()
	chapter, err := ledgerChapterQuery(r, 0)
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	diagnostics, err := store.Diagnostics(r.Context(), chapter)
	if err != nil {
		writeLedgerError(w, r, err)
		return
	}
	writeLedgerJSON(w, http.StatusOK, map[string]any{"chapter": chapter, "diagnostics": diagnostics})
}

func ledgerListOptions(r *http.Request) (narrativeledger.ListOptions, error) {
	chapter, err := ledgerChapterQuery(r, 0)
	if err != nil {
		return narrativeledger.ListOptions{}, err
	}
	limit, err := ledgerQueryInt(r, "limit", 50)
	if err != nil {
		return narrativeledger.ListOptions{}, err
	}
	offset, err := ledgerQueryInt(r, "offset", 0)
	if err != nil {
		return narrativeledger.ListOptions{}, err
	}
	return narrativeledger.ListOptions{
		AsOfChapter: chapter,
		Status:      r.URL.Query().Get("status"),
		Priority:    r.URL.Query().Get("priority"),
		Query:       r.URL.Query().Get("q"),
		Limit:       limit,
		Offset:      offset,
	}, nil
}

func ledgerChapterQuery(r *http.Request, fallback int) (int, error) {
	return ledgerQueryInt(r, "chapter", fallback)
}

func ledgerQueryInt(r *http.Request, key string, fallback int) (int, error) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, &narrativeledger.Error{Code: "LEDGER_QUERY_INVALID", Message: key + " must be a non-negative integer", Cause: narrativeledger.ErrValidation}
	}
	return parsed, nil
}

func requireLedgerIdempotency(w http.ResponseWriter, r *http.Request) (string, bool) {
	value := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if value == "" || len(value) > 200 {
		writeLedgerJSON(w, http.StatusBadRequest, ledgerErrorEnvelope{Error: ledgerErrorBody{
			Code:    "IDEMPOTENCY_KEY_REQUIRED",
			Message: "Idempotency-Key is required and must be at most 200 characters",
			TraceID: ledgerTraceID(r),
		}})
		return "", false
	}
	return value, true
}

func decodeLedgerJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, ledgerRequestLimit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return &narrativeledger.Error{Code: "LEDGER_JSON_INVALID", Message: "request body must be one strict JSON object", Cause: narrativeledger.ErrValidation}
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return &narrativeledger.Error{Code: "LEDGER_JSON_INVALID", Message: "request body must contain exactly one JSON object", Cause: narrativeledger.ErrValidation}
	}
	return nil
}

func writeLedgerError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	body := ledgerErrorBody{Code: "LEDGER_INTERNAL_ERROR", Message: "narrative ledger operation failed", TraceID: ledgerTraceID(r)}
	var domain *narrativeledger.Error
	if errors.As(err, &domain) {
		body.Code = domain.Code
		body.Message = domain.Message
		switch {
		case errors.Is(domain, narrativeledger.ErrValidation), errors.Is(domain, narrativeledger.ErrAuthority):
			status = http.StatusBadRequest
		case errors.Is(domain, narrativeledger.ErrNotFound):
			status = http.StatusNotFound
		case errors.Is(domain, narrativeledger.ErrConflict):
			status = http.StatusConflict
		}
	}
	writeLedgerJSON(w, status, ledgerErrorEnvelope{Error: body})
}

func writeLedgerJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func ledgerTraceID(r *http.Request) string {
	for _, header := range []string{"X-Trace-ID", "X-Request-ID"} {
		if value := strings.TrimSpace(r.Header.Get(header)); value != "" && len(value) <= 128 {
			return value
		}
	}
	return fmt.Sprintf("ledger-%x", len(r.URL.Path))
}
