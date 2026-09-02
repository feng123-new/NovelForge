package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/voocel/ainovel-cli/internal/project"
)

func TestTruthAPIAppendReplayAndChapterState(t *testing.T) {
	workspace := t.TempDir()
	repository, err := project.NewRepository(workspace)
	if err != nil {
		t.Fatal(err)
	}
	created, err := repository.Create(context.Background(), project.CreateInput{Title: "Truth API"})
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(Config{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"project_id": created.ID, "event": map[string]any{
		"kind": "assert", "subject_type": "character", "subject_id": "hero",
		"predicate": "location.current", "value": "Harbor", "valid_from_chapter": 4,
		"known_from_chapter": 4, "authority": "generated_final", "confidence": 1,
		"source": map[string]any{"type": "chapter", "id": "chapter-4", "chapter": 4, "version": "chapter-v1"},
	}}
	data, _ := json.Marshal(body)
	appendRequest := httptest.NewRequest(http.MethodPost, "/api/truth/events", bytes.NewReader(data))
	appendRequest.Header.Set("Idempotency-Key", "api-truth-1")
	appendResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(appendResponse, appendRequest)
	if appendResponse.Code != http.StatusCreated {
		t.Fatalf("append = %d %s", appendResponse.Code, appendResponse.Body.String())
	}
	replayRequest := httptest.NewRequest(http.MethodPost, "/api/truth/events", bytes.NewReader(data))
	replayRequest.Header.Set("Idempotency-Key", "api-truth-1")
	replayResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(replayResponse, replayRequest)
	if replayResponse.Code != http.StatusCreated || replayResponse.Header().Get("Idempotency-Replayed") != "true" || replayResponse.Body.String() != appendResponse.Body.String() {
		t.Fatalf("replay = %d %s", replayResponse.Code, replayResponse.Body.String())
	}
	before := httptest.NewRecorder()
	server.Handler().ServeHTTP(before, httptest.NewRequest(http.MethodGet, "/api/truth/state?project_id="+created.ID+"&chapter=3", nil))
	if before.Code != http.StatusOK || !bytes.Contains(before.Body.Bytes(), []byte(`"total":0`)) {
		t.Fatalf("before = %d %s", before.Code, before.Body.String())
	}
	after := httptest.NewRecorder()
	server.Handler().ServeHTTP(after, httptest.NewRequest(http.MethodGet, "/api/truth/state?project_id="+created.ID+"&chapter=4", nil))
	if after.Code != http.StatusOK || !bytes.Contains(after.Body.Bytes(), []byte(`"total":1`)) {
		t.Fatalf("after = %d %s", after.Code, after.Body.String())
	}
}

func TestTruthAPIRequiresIdempotencyKeyAndUsesSafeEnvelope(t *testing.T) {
	workspace := t.TempDir()
	server, err := New(Config{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/truth/events", bytes.NewBufferString(`{}`))
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", response.Code)
	}
	body := response.Body.String()
	for _, required := range []string{"IDEMPOTENCY_KEY_REQUIRED", "trace_id", "retryable"} {
		if !bytes.Contains([]byte(body), []byte(required)) {
			t.Fatalf("safe error missing %q: %s", required, body)
		}
	}
}

func TestTruthAPIBatchReplayAndEventFilters(t *testing.T) {
	workspace := t.TempDir()
	repository, err := project.NewRepository(workspace)
	if err != nil {
		t.Fatal(err)
	}
	created, err := repository.Create(context.Background(), project.CreateInput{Title: "Truth Batch API"})
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(Config{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	for chapter, key := range map[int]string{2: "api-filter-2", 7: "api-filter-7"} {
		payload := map[string]any{"project_id": created.ID, "event": map[string]any{
			"kind": "assert", "subject_type": "character", "subject_id": "hero",
			"predicate": "state.realm", "value": chapter, "valid_from_chapter": chapter,
			"known_from_chapter": chapter, "authority": "generated_final", "confidence": 1,
			"source": map[string]any{"type": "chapter", "id": key, "chapter": chapter, "version": "chapter-v1"},
		}}
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/truth/events", bytes.NewReader(data))
		request.Header.Set("Idempotency-Key", key)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("append chapter %d = %d %s", chapter, response.Code, response.Body.String())
		}
	}

	events := httptest.NewRecorder()
	server.Handler().ServeHTTP(events, httptest.NewRequest(http.MethodGet,
		"/api/truth/events?project_id="+created.ID+"&through_chapter=2&subject_type=character&subject_id=hero&predicate=state.realm&limit=10", nil))
	if events.Code != http.StatusOK {
		t.Fatalf("events = %d %s", events.Code, events.Body.String())
	}
	var eventPage struct {
		Events []struct {
			ValidFromChapter int `json:"valid_from_chapter"`
		} `json:"events"`
	}
	if err := json.Unmarshal(events.Body.Bytes(), &eventPage); err != nil {
		t.Fatal(err)
	}
	if len(eventPage.Events) != 1 || eventPage.Events[0].ValidFromChapter != 2 {
		t.Fatalf("filtered events = %#v", eventPage)
	}

	batchBody, err := json.Marshal(map[string]any{
		"project_id": created.ID,
		"queries": []map[string]any{
			{"chapter": 1, "subject_id": "hero", "limit": 10},
			{"chapter": 7, "subject_id": "hero", "predicate": "state.realm", "limit": 10},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	callBatch := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/truth/state:batch", bytes.NewReader(batchBody))
		request.Header.Set("Idempotency-Key", "api-batch-replay")
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		return response
	}
	first := callBatch()
	second := callBatch()
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("batch status = %d/%d, first=%s second=%s", first.Code, second.Code, first.Body.String(), second.Body.String())
	}
	if second.Header().Get("Idempotency-Replayed") != "true" || second.Body.String() != first.Body.String() {
		t.Fatalf("batch replay mismatch: first=%s second=%s", first.Body.String(), second.Body.String())
	}
	var batch struct {
		Results []struct {
			Total int `json:"total"`
		} `json:"results"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Results) != 2 || batch.Results[0].Total != 0 || batch.Results[1].Total != 2 {
		t.Fatalf("batch response = %#v", batch)
	}
}

func TestTruthAPIRejectsInvalidConflictStatusAndUnknownFields(t *testing.T) {
	workspace := t.TempDir()
	repository, err := project.NewRepository(workspace)
	if err != nil {
		t.Fatal(err)
	}
	created, err := repository.Create(context.Background(), project.CreateInput{Title: "Truth Validation API"})
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(Config{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}

	invalidStatus := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidStatus, httptest.NewRequest(http.MethodGet,
		"/api/truth/conflicts?project_id="+created.ID+"&status=open", nil))
	if invalidStatus.Code != http.StatusBadRequest || !bytes.Contains(invalidStatus.Body.Bytes(), []byte(string("TRUTH_VALIDATION_FAILED"))) {
		t.Fatalf("invalid status = %d %s", invalidStatus.Code, invalidStatus.Body.String())
	}

	unknown := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/truth/rebuild", bytes.NewBufferString(
		`{"project_id":"`+created.ID+`","from_chapter":0,"unexpected":true}`))
	request.Header.Set("Idempotency-Key", "api-rebuild-unknown")
	server.Handler().ServeHTTP(unknown, request)
	if unknown.Code != http.StatusBadRequest || !bytes.Contains(unknown.Body.Bytes(), []byte("REQUEST_BODY_INVALID")) {
		t.Fatalf("unknown field = %d %s", unknown.Code, unknown.Body.String())
	}
}
