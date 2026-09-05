package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/voocel/ainovel-cli/internal/chapterversion"
	"github.com/voocel/ainovel-cli/internal/project"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
)

// Only the paid provider is replaced. All application paths use production code.
func TestConfiguredQualityHTTPAndHumanRevision(t *testing.T) {
	var writerCalls atomic.Int32
	var sawContext atomic.Bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct { Messages []struct { Role string `json:"role"`; Content json.RawMessage `json:"content"` } `json:"messages"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil { http.Error(w, "bad request", 400); return }
		var data string
		for _, msg := range body.Messages {
			if msg.Role != "user" { continue }
			if json.Unmarshal(msg.Content, &data) != nil {
				var parts []struct { Text string `json:"text"` }
				if json.Unmarshal(msg.Content, &parts) == nil { for _, p := range parts { data += p.Text } }
			}
		}
		var input struct {
			Schema string `json:"schema"`
			ProjectID string `json:"project_id"`
			Chapter int `json:"chapter"`
			Candidate qualitygate.Candidate `json:"candidate"`
			Compiled struct { Text string `json:"text"` } `json:"compiled_context"`
		}
		if json.Unmarshal([]byte(data), &input) != nil { http.Error(w, "missing structured input", 400); return }
		text := "张三来到山门，钟声响起。"
		switch input.Schema {
		case "FactProposal":
			p := qualitygate.FactProposal{ProposalID: "proposal-" + input.Candidate.ID, ProjectID: input.ProjectID, Chapter: input.Chapter, SourceVersion: input.Candidate.SourceVersion, SourceSHA: input.Candidate.TextSHA, Extractor: "smoke", Authority: "llm_suggestion"}
			raw, _ := json.Marshal(p); text = string(raw)
		case "EditorReview":
			raw, _ := json.Marshal(qualitygate.EditorReview{Score: 8.5, Summary: "clear and consistent"}); text = string(raw)
		default:
			writerCalls.Add(1)
			sawContext.Store(strings.Contains(input.Compiled.Text, "knowledge_boundary") && strings.Contains(input.Compiled.Text, "Mira"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "smoke", "object": "chat.completion", "model": "smoke-model", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": text}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 20, "completion_tokens": 10, "total_tokens": 30}})
	}))
	defer provider.Close()
	workspace := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "models.json")
	config, _ := json.Marshal(map[string]any{"provider": "smoke", "model": "smoke-model", "providers": map[string]any{"smoke": map[string]any{"type": "openai", "api": "chat", "base_url": provider.URL + "/v1", "api_key": "test-only"}}})
	if err := os.WriteFile(configPath, config, 0600); err != nil { t.Fatal(err) }
	cfg := Config{Workspace: workspace, QualityConfigEnabled: true, QualityConfigPath: configPath}
	app, err := New(cfg)
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = app.Close() })
	created, err := app.projects.Create(t.Context(), project.CreateInput{Title: "Configured short flow"})
	if err != nil { t.Fatal(err) }
	if !app.qualityConfigured(created.ID) { t.Fatal("project config not connected") }
	base := "/api/projects/" + created.ID + "/chapters/1"
	post := func(path, key string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)); req.Header.Set("Idempotency-Key", key)
		resp := httptest.NewRecorder(); app.Handler().ServeHTTP(resp, req)
		if resp.Code < 200 || resp.Code >= 300 { t.Fatalf("%s status=%d body=%s", path, resp.Code, resp.Body.String()) }
		return resp
	}
	post(base + "/generate", "smoke-generate", apiPlan(1))
	replay := post(base + "/generate", "smoke-generate", apiPlan(1))
	if writerCalls.Load() != 1 || !sawContext.Load() || replay.Header().Get("Idempotency-Replayed") != "true" { t.Fatal("compiled context / paid-call replay broken") }
	post(base + "/check", "smoke-check", []byte(`{}`))
	post(base + "/finalize", "smoke-finalize", []byte(`{}`))
	humanText := "张三没有进山门，而是停在门前听钟。"
	raw, _ := json.Marshal(map[string]string{"content": humanText})
	saved := post(base + "/versions", "smoke-save", raw)
	var revision chapterversion.Version
	if err := json.Unmarshal(saved.Body.Bytes(), &revision); err != nil || revision.ID == "" { t.Fatalf("revision=%s err=%v", saved.Body.String(), err) }
	versionPath := base + "/versions/" + revision.ID
	post(versionPath + "/check", "smoke-human-check", []byte(`{}`))
	post(versionPath + "/accept", "smoke-human-accept", []byte(`{}`))
	post(versionPath + "/finalize", "smoke-human-finalize", []byte(`{}`))
	post(versionPath + "/finalize", "smoke-human-finalize", []byte(`{}`))
	restarted, err := New(cfg)
	if err != nil { t.Fatal(err) }
	defer restarted.Close()
	state := httptest.NewRecorder(); restarted.Handler().ServeHTTP(state, httptest.NewRequest(http.MethodGet, base, nil))
	var view struct { ActiveFinal *chapterversion.Version `json:"active_final"` }
	if err := json.Unmarshal(state.Body.Bytes(), &view); err != nil || state.Code != 200 || view.ActiveFinal == nil || view.ActiveFinal.Content != humanText { t.Fatalf("restarted state=%s err=%v", state.Body.String(), err) }
}
