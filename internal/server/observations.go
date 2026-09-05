package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/autopilot"
	"github.com/voocel/ainovel-cli/internal/observability"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
)

func (s *Server) observationStore(store *qualitygate.Store, id string) *observability.Store {
	return &observability.Store{DB: store.Database(), ProjectID: id, Notify: s.observationNotify(id)}
}
func (s *Server) observationNotify(id string) func(string, string) {
	return func(kind, attempt string) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err := s.events.PublishContext(ctx, "observability.changed", id, map[string]string{"kind": kind, "attempt_id": attempt})
		if err != nil {
			slog.Warn("observation event could not be published", "code", "OBSERVATION_EVENT_FAILED")
		}
		slog.Info("model observation", "event", kind, "project_id", id, "attempt_id", attempt)
	}
}
func (s *Server) registerObservationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/projects/{id}/observability", s.handleObservability)
	mux.HandleFunc("/api/projects/{id}/observability/diagnostics", s.handleDiagnostics)
	mux.HandleFunc("/api/projects/{id}/observability/report", s.handleDiagnosticReport)
}
func observationFailure(err error) *apiFailure {
	if code := observability.ControlCode(err); code != "" {
		message, action := observability.Explain(code)
		return &apiFailure{Status: 409, Code: code, Message: message, Details: map[string]any{"action": action}}
	}
	switch {
	case errors.Is(err, observability.ErrInvalid):
		return &apiFailure{Status: 400, Code: "OBSERVATION_INVALID", Message: "observation input is invalid"}
	case errors.Is(err, observability.ErrConflict):
		return &apiFailure{Status: 409, Code: "OBSERVATION_CONFLICT", Message: "refresh the revision; currency and resolved historical costs cannot be silently replaced"}
	case errors.Is(err, observability.ErrNotFound):
		return &apiFailure{Status: 404, Code: "OBSERVATION_NOT_FOUND", Message: "attempt does not exist in this project"}
	}
	return projectFailure(err)
}
func (s *Server) handleObservability(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store, close, err := s.projects.OpenObservations(r.Context(), id)
	if err != nil {
		writeFailure(w, r, *projectFailure(err))
		return
	}
	defer close()
	store.Notify = s.observationNotify(id)
	switch r.Method {
	case http.MethodGet:
		limit, offset, f := parseBoundedPage(r, 50, 100)
		if f != nil {
			writeFailure(w, r, *f)
			return
		}
		chapter := 0
		if q := r.URL.Query().Get("chapter"); q != "" {
			chapter, err = strconv.Atoi(q)
			if err != nil || chapter < 1 || chapter > 1000 {
				writeFailure(w, r, *observationFailure(observability.ErrInvalid))
				return
			}
		}
		page, err := store.Page(r.Context(), r.URL.Query().Get("task"), chapter, limit, offset)
		if err != nil {
			writeFailure(w, r, *observationFailure(err))
			return
		}
		writeJSON(w, 200, page)
	case http.MethodPost:
		s.executeIdempotent(w, r, "observability.change", id, func(body []byte) (int, any, *apiFailure) {
			var m observability.Mutation
			if f := decodeJSONBody(body, &m, false); f != nil {
				return f.Status, nil, f
			}
			out, err := store.Mutate(r.Context(), r.Header.Get("Idempotency-Key"), m)
			if err != nil {
				f := observationFailure(err)
				return f.Status, nil, f
			}
			return 200, out, nil
		})
	default:
		writeMethodNotAllowed(w, r, http.MethodGet, http.MethodPost)
	}
}
func (s *Server) projectFindings(r *http.Request, id string) ([]observability.Finding, error) {
	store, close, err := s.projects.OpenObservations(r.Context(), id)
	if err != nil {
		return nil, err
	}
	defer close()
	findings, err := store.Findings(r.Context())
	if err != nil {
		return nil, err
	}
	jobs, err := s.jobs.List(r.Context(), id, 100, 0)
	if err != nil {
		return nil, err
	}
	chapter, nextErr := s.projects.AutopilotNextChapter(r.Context(), id)
	if nextErr != nil {
		return nil, nextErr
	}
	if chapter > 1000 {
		chapter = 1000
	}
	for _, j := range jobs {
		if !j.Terminal() && j.Chapter > chapter {
			chapter = j.Chapter
		}
		if j.ErrorCode != "" && !j.Terminal() {
			msg, action := observability.Explain(j.ErrorCode)
			findings = append(findings, observability.Finding{Code: j.ErrorCode, Severity: "warning", Count: 1, TaskID: j.ID, Chapter: j.Chapter, Message: msg, Action: action})
		}
		if (j.State == autopilot.Running || j.State == autopilot.Retrying) && time.Since(j.UpdatedAt) > 10*time.Minute {
			findings = append(findings, observability.Finding{Code: "TASK_PROGRESS_STALE", Severity: "warning", Count: 1, TaskID: j.ID, Chapter: j.Chapter, Message: "任务超过十分钟没有新的持久化进度；这不直接证明死锁。", Action: "先检查当前调用与控制意图，勿启动第二个写作进程。"})
		}
	}
	// Reuse the existing ledger's Chapter-N computed OVERDUE result, not a new
	// contradictory foreshadow status table. Diagnostics never mutate the ledger.
	ledger, err := s.projects.OpenNarrativeLedger(r.Context(), id)
	if err != nil {
		return nil, err
	}
	defer ledger.Close()
	snapshot, err := ledger.PlannerContext(r.Context(), id, chapter, "", "", 3)
	if err != nil {
		return nil, err
	}
	overdue := 0
	for _, f := range snapshot.Foreshadows {
		if strings.Contains(strings.ToLower(f.Kind), "overdue") {
			overdue++
		}
	}
	if overdue > 0 {
		findings = append(findings, observability.Finding{Code: "OVERDUE_FORESHADOWS", Severity: "warning", Count: overdue, Chapter: chapter, Message: "当前章节存在逾期伏笔。", Action: "在 Foreshadows 查看来源和回收窗口；诊断不会自动回收。"})
	}
	return findings, nil
}
func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	out, err := s.projectFindings(r, r.PathValue("id"))
	if err != nil {
		writeFailure(w, r, *projectFailure(err))
		return
	}
	writeJSON(w, 200, map[string]any{"findings": out, "generated_at": time.Now().UTC(), "task_history_limit": 100})
}
func (s *Server) handleDiagnosticReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}
	store, close, err := s.projects.OpenObservations(r.Context(), r.PathValue("id"))
	if err != nil {
		writeFailure(w, r, *projectFailure(err))
		return
	}
	defer close()
	p, err := store.Page(r.Context(), "", 0, 100, 0)
	if err != nil {
		writeFailure(w, r, *observationFailure(err))
		return
	}
	report := observability.Redact(p)
	report["version"] = s.cfg.Version
	report["go_version"] = runtime.Version()
	report["os"] = runtime.GOOS
	report["arch"] = runtime.GOARCH
	report["generated_at"] = time.Now().UTC()
	findings, err := s.projectFindings(r, r.PathValue("id"))
	if err != nil {
		writeFailure(w, r, *projectFailure(err))
		return
	}
	counts := map[string]int{}
	for _, f := range findings {
		counts[f.Code] += f.Count
	}
	report["findings"] = counts
	w.Header().Set("Content-Disposition", `attachment; filename="novelforge-diagnostics.json"`)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, report)
}
