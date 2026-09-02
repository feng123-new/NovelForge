package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/voocel/ainovel-cli/internal/narrativeledger"
	"github.com/voocel/ainovel-cli/internal/project"
	"github.com/voocel/ainovel-cli/internal/truthstore"
)

func (s *Server) registerLedgerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/projects/{id}/foreshadows", s.handleForeshadows)
	mux.HandleFunc("/api/projects/{id}/foreshadows/{foreshadow}", s.handleForeshadow)
	mux.HandleFunc("/api/projects/{id}/secrets", s.handleSecrets)
	mux.HandleFunc("/api/projects/{id}/secrets/{secret}", s.handleSecret)
	mux.HandleFunc("/api/projects/{id}/secrets/{secret}/holders", s.handleSecretHolders)
	mux.HandleFunc("/api/projects/{id}/secrets/{secret}/holders/{holder}/close", s.handleSecretHolderClose)
	mux.HandleFunc("/api/projects/{id}/ledger/dashboard", s.handleLedgerDashboard)
	mux.HandleFunc("/api/projects/{id}/ledger/diagnostics", s.handleLedgerDiagnostics)
	mux.HandleFunc("/api/projects/{id}/ledger/planner-context", s.handleLedgerPlannerContext)
}

func (s *Server) ledgerIdentity(r *http.Request) (string, *apiFailure) {
	projectID := strings.TrimSpace(r.PathValue("id"))
	if projectID == "" {
		return "", &apiFailure{Status: http.StatusBadRequest, Code: "LEDGER_PROJECT_INVALID", Message: "project is required"}
	}
	if _, err := s.projects.Get(r.Context(), projectID); err != nil {
		return "", projectFailure(err)
	}
	return projectID, nil
}

func (s *Server) openLedger(r *http.Request, projectID string) (*narrativeledger.Store, *apiFailure) {
	store, err := s.projects.OpenNarrativeLedger(r.Context(), projectID)
	if err != nil {
		return nil, projectFailure(err)
	}
	return store, nil
}

func (s *Server) handleForeshadows(w http.ResponseWriter, r *http.Request) {
	projectID, failure := s.ledgerIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	switch r.Method {
	case http.MethodGet:
		chapter, failure := parseChapterBoundary(r, "chapter")
		if failure != nil {
			writeFailure(w, r, *failure)
			return
		}
		limit, offset, failure := parseBoundedPage(r, 50, 100)
		if failure != nil {
			writeFailure(w, r, *failure)
			return
		}
		query := narrativeledger.ForeshadowQuery{CurrentChapter: chapter, Status: narrativeledger.Status(strings.TrimSpace(r.URL.Query().Get("status"))), Importance: narrativeledger.Importance(strings.TrimSpace(r.URL.Query().Get("importance"))), Urgency: narrativeledger.Urgency(strings.TrimSpace(r.URL.Query().Get("urgency"))), Arc: r.URL.Query().Get("arc"), Entity: r.URL.Query().Get("entity"), Query: r.URL.Query().Get("query"), Limit: limit, Offset: offset}
		if value := strings.TrimSpace(r.URL.Query().Get("overdue")); value != "" {
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				writeFailure(w, r, apiFailure{Status: http.StatusBadRequest, Code: "LEDGER_FILTER_INVALID", Message: "overdue must be true or false"})
				return
			}
			query.Overdue = &parsed
		}
		store, failure := s.openLedger(r, projectID)
		if failure != nil {
			writeFailure(w, r, *failure)
			return
		}
		defer store.Close()
		page, err := store.ListForeshadows(r.Context(), projectID, query)
		if err != nil {
			writeFailure(w, r, *ledgerFailure(err))
			return
		}
		writeJSON(w, http.StatusOK, page)
	case http.MethodPost:
		s.executeIdempotent(w, r, "ledger.foreshadow.create", projectID, func(body []byte) (int, any, *apiFailure) {
			var input narrativeledger.ForeshadowInput
			if failure := decodeJSONBody(body, &input, false); failure != nil {
				return failure.Status, nil, failure
			}
			if input.Authority == "" {
				input.Authority = truthstore.AuthorityHumanFinal
			}
			store, failure := s.openLedger(r, projectID)
			if failure != nil {
				return failure.Status, nil, failure
			}
			defer store.Close()
			item, err := store.CreateForeshadow(r.Context(), projectID, strings.TrimSpace(r.Header.Get("Idempotency-Key")), input)
			if err != nil {
				failure := ledgerFailure(err)
				return failure.Status, nil, failure
			}
			return http.StatusCreated, item, nil
		})
	default:
		writeMethodNotAllowed(w, r, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleForeshadow(w http.ResponseWriter, r *http.Request) {
	projectID, failure := s.ledgerIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	id := strings.TrimSpace(r.PathValue("foreshadow"))
	if id == "" {
		writeFailure(w, r, apiFailure{Status: http.StatusBadRequest, Code: "LEDGER_RESOURCE_INVALID", Message: "foreshadow is required"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		chapter, failure := parseChapterBoundary(r, "chapter")
		if failure != nil {
			writeFailure(w, r, *failure)
			return
		}
		store, failure := s.openLedger(r, projectID)
		if failure != nil {
			writeFailure(w, r, *failure)
			return
		}
		defer store.Close()
		item, err := store.GetForeshadow(r.Context(), projectID, id, chapter)
		if err != nil {
			writeFailure(w, r, *ledgerFailure(err))
			return
		}
		writeJSON(w, http.StatusOK, item)
	case http.MethodPatch:
		s.executeIdempotent(w, r, "ledger.foreshadow.update", projectID, func(body []byte) (int, any, *apiFailure) {
			var patch narrativeledger.ForeshadowPatch
			if failure := decodeJSONBody(body, &patch, false); failure != nil {
				return failure.Status, nil, failure
			}
			store, failure := s.openLedger(r, projectID)
			if failure != nil {
				return failure.Status, nil, failure
			}
			defer store.Close()
			item, err := store.UpdateForeshadow(r.Context(), projectID, id, strings.TrimSpace(r.Header.Get("Idempotency-Key")), patch)
			if err != nil {
				failure := ledgerFailure(err)
				return failure.Status, nil, failure
			}
			return http.StatusOK, item, nil
		})
	default:
		writeMethodNotAllowed(w, r, http.MethodGet, http.MethodPatch)
	}
}

func (s *Server) handleSecrets(w http.ResponseWriter, r *http.Request) {
	projectID, failure := s.ledgerIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	switch r.Method {
	case http.MethodGet:
		chapter, failure := parseChapterBoundary(r, "chapter")
		if failure != nil {
			writeFailure(w, r, *failure)
			return
		}
		limit, offset, failure := parseBoundedPage(r, 50, 100)
		if failure != nil {
			writeFailure(w, r, *failure)
			return
		}
		includeTruth := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_truth")), "true")
		query := narrativeledger.SecretQuery{CurrentChapter: chapter, PublicStatus: narrativeledger.PublicStatus(strings.TrimSpace(r.URL.Query().Get("public_status"))), Holder: r.URL.Query().Get("holder"), Query: r.URL.Query().Get("query"), IncludeTruth: includeTruth, Limit: limit, Offset: offset}
		store, failure := s.openLedger(r, projectID)
		if failure != nil {
			writeFailure(w, r, *failure)
			return
		}
		defer store.Close()
		page, err := store.ListSecrets(r.Context(), projectID, query)
		if err != nil {
			writeFailure(w, r, *ledgerFailure(err))
			return
		}
		writeJSON(w, http.StatusOK, page)
	case http.MethodPost:
		s.executeIdempotent(w, r, "ledger.secret.create", projectID, func(body []byte) (int, any, *apiFailure) {
			var input narrativeledger.SecretInput
			if failure := decodeJSONBody(body, &input, false); failure != nil {
				return failure.Status, nil, failure
			}
			if input.Authority == "" {
				input.Authority = truthstore.AuthorityHumanFinal
			}
			store, failure := s.openLedger(r, projectID)
			if failure != nil {
				return failure.Status, nil, failure
			}
			defer store.Close()
			item, err := store.CreateSecret(r.Context(), projectID, strings.TrimSpace(r.Header.Get("Idempotency-Key")), input)
			if err != nil {
				failure := ledgerFailure(err)
				return failure.Status, nil, failure
			}
			return http.StatusCreated, item, nil
		})
	default:
		writeMethodNotAllowed(w, r, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleSecret(w http.ResponseWriter, r *http.Request) {
	projectID, failure := s.ledgerIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	id := strings.TrimSpace(r.PathValue("secret"))
	if id == "" {
		writeFailure(w, r, apiFailure{Status: http.StatusBadRequest, Code: "LEDGER_RESOURCE_INVALID", Message: "secret is required"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		chapter, failure := parseChapterBoundary(r, "chapter")
		if failure != nil {
			writeFailure(w, r, *failure)
			return
		}
		includeTruth := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_truth")), "true")
		store, failure := s.openLedger(r, projectID)
		if failure != nil {
			writeFailure(w, r, *failure)
			return
		}
		defer store.Close()
		item, err := store.GetSecret(r.Context(), projectID, id, chapter, includeTruth)
		if err != nil {
			writeFailure(w, r, *ledgerFailure(err))
			return
		}
		holders, err := store.SecretHolders(r.Context(), id, chapter)
		if err != nil {
			writeFailure(w, r, *ledgerFailure(err))
			return
		}
		item.Holders = holders
		writeJSON(w, http.StatusOK, item)
	case http.MethodPatch:
		s.executeIdempotent(w, r, "ledger.secret.update", projectID, func(body []byte) (int, any, *apiFailure) {
			var patch narrativeledger.SecretPatch
			if failure := decodeJSONBody(body, &patch, false); failure != nil {
				return failure.Status, nil, failure
			}
			store, failure := s.openLedger(r, projectID)
			if failure != nil {
				return failure.Status, nil, failure
			}
			defer store.Close()
			item, err := store.UpdateSecret(r.Context(), projectID, id, strings.TrimSpace(r.Header.Get("Idempotency-Key")), patch)
			if err != nil {
				failure := ledgerFailure(err)
				return failure.Status, nil, failure
			}
			return http.StatusOK, item, nil
		})
	default:
		writeMethodNotAllowed(w, r, http.MethodGet, http.MethodPatch)
	}
}

func (s *Server) handleSecretHolders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	projectID, failure := s.ledgerIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	id := strings.TrimSpace(r.PathValue("secret"))
	s.executeIdempotent(w, r, "ledger.secret.holder.add", projectID, func(body []byte) (int, any, *apiFailure) {
		var input narrativeledger.HolderInput
		if failure := decodeJSONBody(body, &input, false); failure != nil {
			return failure.Status, nil, failure
		}
		if input.Authority == "" {
			input.Authority = truthstore.AuthorityHumanFinal
		}
		store, failure := s.openLedger(r, projectID)
		if failure != nil {
			return failure.Status, nil, failure
		}
		defer store.Close()
		item, err := store.AddHolder(r.Context(), projectID, id, strings.TrimSpace(r.Header.Get("Idempotency-Key")), input)
		if err != nil {
			failure := ledgerFailure(err)
			return failure.Status, nil, failure
		}
		return http.StatusCreated, item, nil
	})
}

type holderCloseRequest struct {
	ValidToChapter int    `json:"valid_to_chapter"`
	SourceVersion  string `json:"source_version"`
}

func (s *Server) handleSecretHolderClose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	projectID, failure := s.ledgerIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	secretID, holderID := strings.TrimSpace(r.PathValue("secret")), strings.TrimSpace(r.PathValue("holder"))
	s.executeIdempotent(w, r, "ledger.secret.holder.close", projectID, func(body []byte) (int, any, *apiFailure) {
		var input holderCloseRequest
		if failure := decodeJSONBody(body, &input, false); failure != nil {
			return failure.Status, nil, failure
		}
		store, failure := s.openLedger(r, projectID)
		if failure != nil {
			return failure.Status, nil, failure
		}
		defer store.Close()
		item, err := store.CloseHolder(r.Context(), projectID, secretID, holderID, strings.TrimSpace(r.Header.Get("Idempotency-Key")), input.ValidToChapter, input.SourceVersion, truthstore.AuthorityHumanFinal)
		if err != nil {
			failure := ledgerFailure(err)
			return failure.Status, nil, failure
		}
		return http.StatusOK, item, nil
	})
}

func (s *Server) handleLedgerDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	projectID, failure := s.ledgerIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	chapter, failure := parseChapterBoundary(r, "chapter")
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	store, failure := s.openLedger(r, projectID)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer store.Close()
	result, err := store.Dashboard(r.Context(), projectID, chapter)
	if err != nil {
		writeFailure(w, r, *ledgerFailure(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) handleLedgerDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	projectID, failure := s.ledgerIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	chapter, failure := parseChapterBoundary(r, "chapter")
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	store, failure := s.openLedger(r, projectID)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer store.Close()
	result, err := store.Diagnostics(r.Context(), projectID, chapter)
	if err != nil {
		writeFailure(w, r, *ledgerFailure(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"diagnostics": result, "total": len(result)})
}
func (s *Server) handleLedgerPlannerContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	projectID, failure := s.ledgerIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	chapter, failure := parseChapterBoundary(r, "chapter")
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	store, failure := s.openLedger(r, projectID)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer store.Close()
	result, err := store.PlannerContext(r.Context(), projectID, chapter, r.URL.Query().Get("pov"), r.URL.Query().Get("arc"), 3)
	if err != nil {
		writeFailure(w, r, *ledgerFailure(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func parseChapterBoundary(r *http.Request, name string) (int, *apiFailure) {
	value := strings.TrimSpace(r.URL.Query().Get(name))
	if value == "" {
		return 0, nil
	}
	chapter, err := strconv.Atoi(value)
	if err != nil || chapter < 0 {
		return 0, &apiFailure{Status: http.StatusBadRequest, Code: "CHAPTER_BOUNDARY_INVALID", Message: "chapter boundary must be a non-negative integer"}
	}
	return chapter, nil
}

func ledgerFailure(err error) *apiFailure {
	var projectErr *project.Error
	if errors.As(err, &projectErr) {
		return projectFailure(err)
	}
	switch {
	case errors.Is(err, narrativeledger.ErrNotFound):
		return &apiFailure{Status: http.StatusNotFound, Code: "LEDGER_NOT_FOUND", Message: "narrative ledger resource not found"}
	case errors.Is(err, narrativeledger.ErrValidation):
		return &apiFailure{Status: http.StatusBadRequest, Code: "LEDGER_VALIDATION_FAILED", Message: "narrative ledger request is invalid"}
	case errors.Is(err, narrativeledger.ErrIdempotencyConflict):
		return &apiFailure{Status: http.StatusConflict, Code: "LEDGER_IDEMPOTENCY_CONFLICT", Message: "idempotency key conflicts with another narrative ledger request"}
	case errors.Is(err, narrativeledger.ErrStateConflict):
		return &apiFailure{Status: http.StatusConflict, Code: "LEDGER_STATE_CONFLICT", Message: "narrative ledger state does not permit this operation"}
	default:
		return ptrFailure(internalFailure())
	}
}
