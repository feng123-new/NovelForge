package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/project"
)

func TestWorkspaceSettingsAndModelsAreRealAndSafe(t *testing.T) {
	workspace := t.TempDir()
	app, err := New(Config{Workspace: workspace, Version: "v0.1.0-test"})
	if err != nil {
		t.Fatal(err)
	}
	settings := httptest.NewRecorder()
	app.Handler().ServeHTTP(settings, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if settings.Code != http.StatusOK {
		t.Fatalf("settings status=%d body=%s", settings.Code, settings.Body.String())
	}
	if strings.Contains(settings.Body.String(), filepath.ToSlash(workspace)) ||
		strings.Contains(settings.Body.String(), filepath.Clean(workspace)) {
		t.Fatalf("settings exposed workspace path: %s", settings.Body.String())
	}
	var payload WorkspaceSettings
	if err := json.Unmarshal(settings.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Product != productName || !payload.LoopbackOnly ||
		payload.Capabilities["formal_web_workspace"] != true {
		t.Fatalf("unexpected settings: %#v", payload)
	}

	modelsResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(modelsResponse, httptest.NewRequest(http.MethodGet, "/api/models?limit=3", nil))
	if modelsResponse.Code != http.StatusOK {
		t.Fatalf("models status=%d body=%s", modelsResponse.Code, modelsResponse.Body.String())
	}
	var modelPage ModelList
	if err := json.Unmarshal(modelsResponse.Body.Bytes(), &modelPage); err != nil {
		t.Fatal(err)
	}
	if modelPage.Total == 0 || len(modelPage.Models) == 0 || len(modelPage.Models) > 3 {
		t.Fatalf("unexpected model page: %#v", modelPage)
	}
}

func TestChapterAndFoundationWorkspaceRoutes(t *testing.T) {
	workspace := t.TempDir()
	app, err := New(Config{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	created, err := app.projects.Create(t.Context(), project.CreateInput{
		Title:          "Route Test",
		TargetChapters: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	chapterRoot := filepath.Join(workspace, created.Path, "chapters")
	if err := os.WriteFile(
		filepath.Join(chapterRoot, "001.md"),
		[]byte("# Arrival\nThe courier reaches the wall."),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	chapters := httptest.NewRecorder()
	app.Handler().ServeHTTP(
		chapters,
		httptest.NewRequest(http.MethodGet, "/api/projects/"+created.ID+"/chapters", nil),
	)
	if chapters.Code != http.StatusOK || !strings.Contains(chapters.Body.String(), `"chapter":1`) {
		t.Fatalf("chapters status=%d body=%s", chapters.Code, chapters.Body.String())
	}

	body := []byte(`{
		"idea":"A courier carries a forbidden map.",
		"style":"Close third person.",
		"model_profile":{"architect":"openai/gpt-test"},
		"automation":{"mode":"copilot","review_policy":"every_chapter","max_rewrites":2}
	}`)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/projects/"+created.ID+"/foundation",
		bytes.NewReader(body),
	)
	request.Header.Set("Idempotency-Key", "foundation-route-test")
	foundation := httptest.NewRecorder()
	app.Handler().ServeHTTP(foundation, request)
	if foundation.Code != http.StatusAccepted ||
		!strings.Contains(foundation.Body.String(), `"worker_available":false`) {
		t.Fatalf("foundation status=%d body=%s", foundation.Code, foundation.Body.String())
	}

	replayedRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/projects/"+created.ID+"/foundation",
		bytes.NewReader(body),
	)
	replayedRequest.Header.Set("Idempotency-Key", "foundation-route-test")
	replayed := httptest.NewRecorder()
	app.Handler().ServeHTTP(replayed, replayedRequest)
	if replayed.Code != http.StatusAccepted || replayed.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("replay status=%d headers=%v", replayed.Code, replayed.Header())
	}

	read := httptest.NewRecorder()
	app.Handler().ServeHTTP(
		read,
		httptest.NewRequest(http.MethodGet, "/api/projects/"+created.ID+"/foundation", nil),
	)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"status":"requested"`) {
		t.Fatalf("read status=%d body=%s", read.Code, read.Body.String())
	}
}
