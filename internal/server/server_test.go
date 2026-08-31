package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHealthAndOpenAPI(t *testing.T) {
	t.Parallel()
	server, err := New(Config{Workspace: t.TempDir(), Version: "v0.1.0-test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	health := httptest.NewRecorder()
	server.Handler().ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", health.Code, health.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(health.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if payload["product"] != productName || payload["version"] != "v0.1.0-test" {
		t.Fatalf("unexpected health payload: %#v", payload)
	}

	spec := httptest.NewRecorder()
	server.Handler().ServeHTTP(spec, httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil))
	if spec.Code != http.StatusOK || !strings.Contains(spec.Body.String(), `"openapi": "3.1.0"`) {
		t.Fatalf("unexpected OpenAPI response: status=%d body=%s", spec.Code, spec.Body.String())
	}
}

func TestProjectsListAndDetail(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	projectDir := filepath.Join(workspace, "sky-road")
	if err := os.MkdirAll(filepath.Join(projectDir, "meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(projectDir, "meta", "book.json"), `{"title":"天路","synopsis":"一部测试长篇。"}`)
	writeTestFile(t, filepath.Join(projectDir, "meta", "progress.json"), `{
		"phase":"writing",
		"current_chapter":13,
		"in_progress_chapter":14,
		"total_chapters":300,
		"total_word_count":42000,
		"current_volume":2,
		"current_arc":1,
		"completed_chapters":[1,2,3]
	}`)
	writeTestFile(t, filepath.Join(projectDir, "meta", "format.json"), `{"version":2}`)

	server, err := New(Config{Workspace: workspace})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/projects", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var list struct {
		Projects []ProjectSummary `json:"projects"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode projects: %v", err)
	}
	if len(list.Projects) != 1 {
		t.Fatalf("projects = %#v", list.Projects)
	}
	project := list.Projects[0]
	if project.Title != "天路" || project.CurrentChapter != 14 || project.CompletedChapters != 3 || project.TotalWords != 42000 {
		t.Fatalf("unexpected project: %#v", project)
	}
	if strings.Contains(project.ID, "sky-road") || project.Path != "sky-road" {
		t.Fatalf("unexpected project identity: %#v", project)
	}

	detailRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(detailRecorder, httptest.NewRequest(http.MethodGet, "/api/projects/"+project.ID, nil))
	if detailRecorder.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body = %s", detailRecorder.Code, detailRecorder.Body.String())
	}
	var detail ProjectDetail
	if err := json.Unmarshal(detailRecorder.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.Synopsis != "一部测试长篇。" || detail.FormatVersion != 2 {
		t.Fatalf("unexpected detail: %#v", detail)
	}
}

func TestEventsEndpointStartsWithConnectedEvent(t *testing.T) {
	t.Parallel()
	server, err := New(Config{Workspace: t.TempDir(), Version: "dev"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("events request: %v", err)
	}
	defer response.Body.Close()
	if response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content type = %q", response.Header.Get("Content-Type"))
	}

	reader := bufio.NewReader(response.Body)
	deadline := time.AfterFunc(2*time.Second, cancel)
	defer deadline.Stop()
	var block strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE: %v", err)
		}
		block.WriteString(line)
		if line == "\n" {
			break
		}
	}
	cancel()
	if !strings.Contains(block.String(), "event: connected") || !strings.Contains(block.String(), `"product":"NovelForge"`) {
		t.Fatalf("unexpected first SSE event: %s", block.String())
	}
}

func TestMethodNotAllowed(t *testing.T) {
	t.Parallel()
	server, err := New(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/projects", nil))
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status=%d allow=%q", recorder.Code, recorder.Header().Get("Allow"))
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
