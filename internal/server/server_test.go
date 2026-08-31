package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestHealthAndOpenAPI(t *testing.T) {
	t.Parallel()
	app, err := New(Config{Workspace: t.TempDir(), Version: "v0.1.0-test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	health := httptest.NewRecorder()
	app.Handler().ServeHTTP(
		health,
		httptest.NewRequest(http.MethodGet, "/api/health", nil),
	)
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
	if health.Header().Get("X-Trace-ID") == "" {
		t.Fatal("health response is missing trace ID")
	}

	spec := httptest.NewRecorder()
	app.Handler().ServeHTTP(
		spec,
		httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil),
	)
	if spec.Code != http.StatusOK ||
		!strings.Contains(spec.Body.String(), `"openapi": "3.1.0"`) {
		t.Fatalf("unexpected OpenAPI response: status=%d body=%s", spec.Code, spec.Body.String())
	}
}

func TestProjectsListAndDetailRetainsLegacyCompatibility(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	projectDir := filepath.Join(workspace, "sky-road")
	mustMkdir(t, filepath.Join(projectDir, "meta"))
	mustMkdir(t, filepath.Join(projectDir, "chapters"))
	writeTestFile(
		t,
		filepath.Join(projectDir, "meta", "book.json"),
		`{"title":"天路","synopsis":"一部测试长篇。"}`,
	)
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

	app, err := New(Config{Workspace: workspace})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/api/projects", nil),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var list struct {
		Projects []ProjectSummary `json:"projects"`
		Total    int              `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode projects: %v", err)
	}
	if len(list.Projects) != 1 || list.Total != 1 {
		t.Fatalf("projects = %#v", list)
	}
	project := list.Projects[0]
	if project.Title != "天路" ||
		project.CurrentChapter != 14 ||
		project.CompletedChapters != 3 ||
		project.TotalWords != 42000 {
		t.Fatalf("unexpected project: %#v", project)
	}
	if strings.Contains(project.ID, "sky-road") ||
		project.Path != "sky-road" ||
		filepath.IsAbs(project.Path) {
		t.Fatalf("unexpected project identity: %#v", project)
	}

	detailRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(
		detailRecorder,
		httptest.NewRequest(http.MethodGet, "/api/projects/"+project.ID, nil),
	)
	if detailRecorder.Code != http.StatusOK {
		t.Fatalf(
			"detail status = %d, body = %s",
			detailRecorder.Code,
			detailRecorder.Body.String(),
		)
	}
	var detail ProjectDetail
	if err := json.Unmarshal(detailRecorder.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.Synopsis != "一部测试长篇。" || detail.FormatVersion != 2 {
		t.Fatalf("unexpected detail: %#v", detail)
	}
}

func TestProjectWriteLifecycleAndIdempotency(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	app, err := New(Config{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}

	createBody := `{
		"title":"Sky Road",
		"genre":"fantasy",
		"language":"zh-CN",
		"target_words":1000000,
		"target_chapters":300,
		"words_per_chapter":3500
	}`
	first := performJSON(
		t,
		app,
		http.MethodPost,
		"/api/projects",
		"create-sky-road",
		createBody,
	)
	if first.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", first.Code, first.Body.String())
	}
	var created ProjectDetail
	decodeRecorder(t, first, &created)
	if created.ID == "" {
		t.Fatal("created project ID is empty")
	}
	if strings.Contains(first.Body.String(), workspace) {
		t.Fatalf("absolute workspace leaked: %s", first.Body.String())
	}
	for _, relative := range []string{
		".novelforge/project.json",
		".novelforge/project.db",
		".novelforge/config.json",
		"chapters",
		"references",
	} {
		if _, err := os.Stat(
			filepath.Join(workspace, created.Path, filepath.FromSlash(relative)),
		); err != nil {
			t.Fatalf("%s: %v", relative, err)
		}
	}

	replay := performJSON(
		t,
		app,
		http.MethodPost,
		"/api/projects",
		"create-sky-road",
		createBody,
	)
	if replay.Code != http.StatusCreated ||
		replay.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("replay status=%d headers=%v body=%s", replay.Code, replay.Header(), replay.Body)
	}
	var replayed ProjectDetail
	decodeRecorder(t, replay, &replayed)
	if replayed.ID != created.ID {
		t.Fatalf("replayed ID=%q want %q", replayed.ID, created.ID)
	}

	conflict := performJSON(
		t,
		app,
		http.MethodPost,
		"/api/projects",
		"create-sky-road",
		`{"title":"Different"}`,
	)
	assertAPIError(t, conflict, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT")

	patch := performJSON(
		t,
		app,
		http.MethodPatch,
		"/api/projects/"+created.ID,
		"rename-sky-road",
		`{"title":"Sky Road Revised","target_chapters":500}`,
	)
	if patch.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", patch.Code, patch.Body.String())
	}
	var updated ProjectDetail
	decodeRecorder(t, patch, &updated)
	if updated.Title != "Sky Road Revised" || updated.TotalChapters != 500 {
		t.Fatalf("updated = %#v", updated)
	}

	archive := performJSON(
		t,
		app,
		http.MethodPost,
		"/api/projects/"+created.ID+"/archive",
		"archive-sky-road",
		"",
	)
	if archive.Code != http.StatusOK {
		t.Fatalf("archive status=%d body=%s", archive.Code, archive.Body.String())
	}
	var archived ProjectDetail
	decodeRecorder(t, archive, &archived)
	if !archived.Archived {
		t.Fatalf("archived = %#v", archived)
	}

	unarchive := performJSON(
		t,
		app,
		http.MethodPost,
		"/api/projects/"+created.ID+"/unarchive",
		"unarchive-sky-road",
		"{}",
	)
	if unarchive.Code != http.StatusOK {
		t.Fatalf("unarchive status=%d body=%s", unarchive.Code, unarchive.Body.String())
	}

	duplicate := performJSON(
		t,
		app,
		http.MethodPost,
		"/api/projects/"+created.ID+"/duplicate",
		"duplicate-sky-road",
		`{"title":"Sky Road Copy"}`,
	)
	if duplicate.Code != http.StatusCreated {
		t.Fatalf("duplicate status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
	var copied ProjectDetail
	decodeRecorder(t, duplicate, &copied)
	if copied.ID == created.ID {
		t.Fatal("duplicate reused source ID")
	}

	duplicateReplay := performJSON(
		t,
		app,
		http.MethodPost,
		"/api/projects/"+created.ID+"/duplicate",
		"duplicate-sky-road",
		`{"title":"Sky Road Copy"}`,
	)
	if duplicateReplay.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("duplicate was not replayed: %v", duplicateReplay.Header())
	}
	list := httptest.NewRecorder()
	app.Handler().ServeHTTP(
		list,
		httptest.NewRequest(http.MethodGet, "/api/projects?limit=100", nil),
	)
	var projects struct {
		Projects []ProjectSummary `json:"projects"`
		Total    int              `json:"total"`
	}
	decodeRecorder(t, list, &projects)
	if projects.Total != 2 || len(projects.Projects) != 2 {
		t.Fatalf("duplicate request executed more than once: %#v", projects)
	}

	badDelete := performJSON(
		t,
		app,
		http.MethodDelete,
		"/api/projects/"+created.ID,
		"delete-sky-road-bad",
		`{"confirm":"wrong"}`,
	)
	assertAPIError(t, badDelete, http.StatusBadRequest, "PROJECT_CONFIRMATION_MISMATCH")

	deleted := performJSON(
		t,
		app,
		http.MethodDelete,
		"/api/projects/"+created.ID,
		"delete-sky-road",
		`{"confirm":"`+created.ID+`"}`,
	)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	notFound := httptest.NewRecorder()
	app.Handler().ServeHTTP(
		notFound,
		httptest.NewRequest(http.MethodGet, "/api/projects/"+created.ID, nil),
	)
	assertAPIError(t, notFound, http.StatusNotFound, "PROJECT_NOT_FOUND")
}

func TestWriteRequiresIdempotencyKey(t *testing.T) {
	t.Parallel()
	app, err := New(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/projects",
		strings.NewReader(`{"title":"No Key"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	app.Handler().ServeHTTP(recorder, request)
	assertAPIError(t, recorder, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED")
}

func TestErrorsUseSafeEnvelope(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	app, err := New(Config{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/api/projects/not-found", nil),
	)
	assertAPIError(t, recorder, http.StatusNotFound, "PROJECT_NOT_FOUND")
	if strings.Contains(recorder.Body.String(), workspace) ||
		strings.Contains(strings.ToLower(recorder.Body.String()), "select ") ||
		strings.Contains(strings.ToLower(recorder.Body.String()), "authorization") {
		t.Fatalf("unsafe error details: %s", recorder.Body.String())
	}
	var envelope APIErrorEnvelope
	decodeRecorder(t, recorder, &envelope)
	if envelope.Error.TraceID == "" || envelope.Error.Details == nil {
		t.Fatalf("error envelope = %#v", envelope)
	}
}

func TestEventsEndpointStartsConnectedAndReplaysAfterRestart(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	firstProcess, err := New(Config{Workspace: workspace, Version: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := firstProcess.Events().PublishContext(
		context.Background(),
		"project.created",
		"project-a",
		map[string]any{"title": "A"},
	)
	if err != nil {
		t.Fatalf("PublishContext: %v", err)
	}
	if persisted.ID == 0 {
		t.Fatal("persisted event has zero ID")
	}

	secondProcess, err := New(Config{Workspace: workspace, Version: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(secondProcess.Handler())
	defer httpServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		httpServer.URL+"/api/events?project=project-a",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Last-Event-ID", "0")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("events request: %v", err)
	}
	defer response.Body.Close()
	if response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content type = %q", response.Header.Get("Content-Type"))
	}

	reader := bufio.NewReader(response.Body)
	connected := readSSEBlock(t, reader)
	if !strings.Contains(connected, "event: connected") ||
		!strings.Contains(connected, `"product":"NovelForge"`) {
		t.Fatalf("unexpected first SSE event: %s", connected)
	}
	replayed := readSSEBlock(t, reader)
	cancel()
	if !strings.Contains(replayed, "event: project.created") ||
		!strings.Contains(replayed, "id: "+strconv.FormatUint(persisted.ID, 10)) ||
		!strings.Contains(replayed, `"project":"project-a"`) {
		t.Fatalf("unexpected replayed SSE event: %s", replayed)
	}
}

func TestInvalidLastEventIDReturnsEnvelope(t *testing.T) {
	t.Parallel()
	app, err := New(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	request.Header.Set("Last-Event-ID", "not-a-number")
	app.Handler().ServeHTTP(recorder, request)
	assertAPIError(t, recorder, http.StatusBadRequest, "LAST_EVENT_ID_INVALID")
}

func TestMethodNotAllowed(t *testing.T) {
	t.Parallel()
	app, err := New(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodPut, "/api/projects", nil),
	)
	if recorder.Code != http.StatusMethodNotAllowed ||
		!strings.Contains(recorder.Header().Get("Allow"), http.MethodGet) ||
		!strings.Contains(recorder.Header().Get("Allow"), http.MethodPost) {
		t.Fatalf(
			"status=%d allow=%q body=%s",
			recorder.Code,
			recorder.Header().Get("Allow"),
			recorder.Body.String(),
		)
	}
}

func performJSON(
	t *testing.T,
	app *Server,
	method string,
	path string,
	idempotencyKey string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	app.Handler().ServeHTTP(recorder, request)
	return recorder
}

func assertAPIError(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	status int,
	code string,
) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status=%d want=%d body=%s", recorder.Code, status, recorder.Body.String())
	}
	var envelope APIErrorEnvelope
	decodeRecorder(t, recorder, &envelope)
	if envelope.Error.Code != code {
		t.Fatalf("code=%q want=%q body=%s", envelope.Error.Code, code, recorder.Body.String())
	}
	if envelope.Error.TraceID == "" {
		t.Fatalf("missing trace_id: %s", recorder.Body.String())
	}
}

func decodeRecorder(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
}

func readSSEBlock(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	result := make(chan struct {
		value string
		err   error
	}, 1)
	go func() {
		var block strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				result <- struct {
					value string
					err   error
				}{err: err}
				return
			}
			block.WriteString(line)
			if line == "\n" {
				result <- struct {
					value string
					err   error
				}{value: block.String()}
				return
			}
		}
	}()
	select {
	case item := <-result:
		if item.err != nil && item.err != io.EOF {
			t.Fatalf("read SSE: %v", item.err)
		}
		return item.value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out reading SSE block")
		return ""
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestJSONBodyRejectsTrailingDocument(t *testing.T) {
	t.Parallel()
	app, err := New(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	recorder := performJSON(
		t,
		app,
		http.MethodPost,
		"/api/projects",
		"trailing-json",
		`{"title":"A"} {"title":"B"}`,
	)
	assertAPIError(t, recorder, http.StatusBadRequest, "REQUEST_BODY_INVALID")
}

func TestRequestBodyLimit(t *testing.T) {
	t.Parallel()
	app, err := New(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	body := bytes.Repeat([]byte("a"), maxRequestBody+1)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/projects", bytes.NewReader(body))
	request.Header.Set("Idempotency-Key", "oversized")
	app.Handler().ServeHTTP(recorder, request)
	assertAPIError(t, recorder, http.StatusRequestEntityTooLarge, "REQUEST_BODY_TOO_LARGE")
}
