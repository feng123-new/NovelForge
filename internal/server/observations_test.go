package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/voocel/ainovel-cli/internal/autopilot"
	"github.com/voocel/ainovel-cli/internal/observability"
	"github.com/voocel/ainovel-cli/internal/project"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
)

func TestObservationConfiguredAttemptsFallbackReplayAndPause(t *testing.T) {
	var primaryCalls, backupCalls atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":{"message":"test rate limit PRIVATE_ERROR_CANARY","type":"rate_limit_error"}}`))
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backupCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "test", "object": "chat.completion", "model": "test-model", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "PRIVATE_DRAFT_CANARY 张三走进山门。"}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 20, "completion_tokens": 10, "total_tokens": 30}})
	}))
	defer backup.Close()
	conf := map[string]any{"provider": "primary", "model": "test-model", "providers": map[string]any{"primary": map[string]any{"type": "openai", "api": "chat", "base_url": primary.URL + "/v1", "api_key": "PRIVATE_KEY_CANARY"}, "backup": map[string]any{"type": "openai", "api": "chat", "base_url": backup.URL + "/v1", "api_key": "test-only"}}, "roles": map[string]any{"writer": map[string]any{"provider": "primary", "model": "test-model", "fallbacks": []any{map[string]any{"provider": "backup", "model": "test-model"}}}}}
	raw, _ := json.Marshal(conf)
	config := filepath.Join(t.TempDir(), "model.json")
	if err := os.WriteFile(config, raw, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New(Config{Workspace: t.TempDir(), QualityConfigEnabled: true, QualityConfigPath: config})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := s.projects.Create(t.Context(), project.CreateInput{Title: "Observation integration"})
	if err != nil {
		t.Fatal(err)
	}
	store, close, err := s.projects.OpenObservations(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	policy := observability.DefaultPolicy()
	policy.ProjectMaxCalls = 2
	policy.Prices = []observability.Price{{Provider: "primary", Model: "test-model", InputMicrosPerMillion: 1000000, OutputMicrosPerMillion: 1000000}, {Provider: "backup", Model: "test-model", InputMicrosPerMillion: 1000000, OutputMicrosPerMillion: 1000000}}
	if _, err = store.Mutate(t.Context(), "limits", observability.Mutation{ExpectedRevision: 1, Policy: &policy}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	coordinator, cleanup, f := s.qualityCoordinator(request, p.ID)
	if f != nil {
		t.Fatal(f)
	}
	defer cleanup()
	var plan qualitygate.ChapterPlan
	_ = json.Unmarshal(apiPlan(1), &plan)
	wr := qualitygate.WriterRequest{ProjectID: p.ID, Chapter: 1, TransactionID: "observation-write", Plan: plan}
	if _, err = coordinator.Writer.Write(t.Context(), wr); err != nil {
		t.Fatal(err)
	}
	if _, err = coordinator.Writer.Write(t.Context(), wr); err != nil {
		t.Fatal(err)
	}
	page, err := store.Page(t.Context(), "", 0, 50, 0)
	if err != nil || page.Total != 2 || page.Replays != 1 || page.Totals.CostMicros != 30 || page.Totals.UnknownCost != 1 || primaryCalls.Load() != 1 || backupCalls.Load() != 1 {
		t.Fatalf("attempt and billing counts: %+v %v primary=%d backup=%d", page, err, primaryCalls.Load(), backupCalls.Load())
	}
	if page.Attempts[0].Provider != "backup" || page.Attempts[1].Provider != "primary" || page.Attempts[0].LogicalID != page.Attempts[1].LogicalID {
		t.Fatal("fallback identity", page.Attempts)
	}
	job, err := s.jobs.Enqueue(t.Context(), p.ID, "limited", autopilot.Input{Idea: "A guarded task", StartChapter: 1, TargetChapter: 1})
	if err != nil {
		t.Fatal(err)
	}
	next, err := (chapterJobEngine{s: s}).Step(t.Context(), job)
	if err != nil || next.State != autopilot.Paused || next.ErrorCode != "PROJECT_CALL_LIMIT" {
		t.Fatalf("budget didn't pause %+v %v", next, err)
	}
	if primaryCalls.Load() != 1 || backupCalls.Load() != 1 {
		t.Fatal("admission sent a request")
	}
	base := "/api/projects/" + p.ID + "/observability"
	blocked := postJob(t, s, base, "active", `{"expected_revision":2}`)
	if blocked.Code != 409 {
		t.Fatal("active edits", blocked.Code)
	}
	_, _ = s.jobs.Control(t.Context(), job.ID, "stop")
	for _, suffix := range []string{"", "/diagnostics", "/report"} {
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, base+suffix, nil))
		if response.Code != 200 {
			t.Fatal(suffix, response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "PRIVATE_") || strings.Contains(response.Body.String(), primary.URL) {
			t.Fatal("private request in observation", response.Body.String())
		}
	}
	// Current schema-11 observations survive the same safe portable backup route.
	archive, err := s.projects.BackupLifecycle(t.Context(), p.ID)
	if err != nil || len(archive) == 0 {
		t.Fatal("schema-11 backup", err)
	}
}
func TestObservationAPIRevisionContractAndUnknownUsage(t *testing.T) {
	s, err := New(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := s.projects.Create(t.Context(), project.CreateInput{Title: "Local diagnostics"})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/projects/" + p.ID + "/observability"
	policy := observability.DefaultPolicy()
	body, _ := json.Marshal(observability.Mutation{ExpectedRevision: 1, Policy: &policy})
	r := postJob(t, s, base, "policy", string(body))
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	r = postJob(t, s, base, "policy", string(body))
	if r.Code != 200 || r.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatal("replay")
	}
	r = postJob(t, s, base, "stale", string(body))
	if r.Code != 409 {
		t.Fatal("stale revision", r.Code)
	}
	r = postJob(t, s, base, "unknown", `{"expected_revision":2,"unexpected":true}`)
	if r.Code != 400 {
		t.Fatal("unknown fields", r.Code)
	}
	for _, path := range []string{"/api/projects/missing/observability", "/api/projects/missing/observability/report"} {
		r = httptest.NewRecorder()
		s.Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
		if r.Code != 404 {
			t.Fatal("project isolation", r.Code)
		}
	}
	var spec map[string]any
	if err = json.Unmarshal(openAPISpec, &spec); err != nil {
		t.Fatal(err)
	}
	paths := spec["paths"].(map[string]any)
	for _, path := range []string{"/api/projects/{id}/observability", "/api/projects/{id}/observability/diagnostics", "/api/projects/{id}/observability/report"} {
		if paths[path] == nil {
			t.Fatal("missing route contract", path)
		}
	}
}
