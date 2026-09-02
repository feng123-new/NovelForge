package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/project"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
)

type apiWriter struct{ calls int }

func (w *apiWriter) Write(context.Context, qualitygate.WriterRequest) (qualitygate.WriterResult, error) {
	w.calls++
	return qualitygate.WriterResult{Text: "A durable generated chapter.", SourceVersion: "api-v1"}, nil
}

type apiLibrarian struct{ calls int }

func (l *apiLibrarian) Propose(_ context.Context, req qualitygate.LibrarianRequest) (qualitygate.FactProposal, error) {
	l.calls++
	return qualitygate.FactProposal{
		ProposalID: "api-proposal-" + req.Candidate.ID, ProjectID: req.ProjectID, Chapter: req.Chapter,
		SourceVersion: req.Candidate.SourceVersion, SourceSHA: req.Candidate.TextSHA, Extractor: "api-test", Authority: "generated_final",
		EntityChanges: []qualitygate.FactChange{}, CharacterChanges: []qualitygate.FactChange{}, RelationshipChanges: []qualitygate.FactChange{}, LocationChanges: []qualitygate.FactChange{}, InventoryChanges: []qualitygate.FactChange{}, KnowledgeChanges: []qualitygate.FactChange{}, TimelineEvents: []qualitygate.FactChange{}, WorldFacts: []qualitygate.FactChange{}, ForeshadowUpdates: []qualitygate.FactChange{}, Secrets: []qualitygate.FactChange{}, Injuries: []qualitygate.FactChange{}, CultivationChanges: []qualitygate.FactChange{}, Diagnostics: []string{},
	}, nil
}

type apiEditor struct{ calls int }

func (e *apiEditor) Review(context.Context, qualitygate.EditorRequest) (qualitygate.EditorReview, error) {
	e.calls++
	return qualitygate.EditorReview{Score: 8.5, Strengths: []string{"clear"}, Weaknesses: []string{}, LineLevelIssues: []string{}, Pacing: "steady", Characterization: "consistent", Prose: "clear", Dialogue: "natural", Ending: "hook", Summary: "accepted"}, nil
}

func apiPlan(chapter int) []byte {
	body, _ := json.Marshal(qualitygate.ChapterPlan{Chapter: chapter, Title: "Arrival", POV: "Mira", Location: "Gate", Objective: "enter", Conflict: "guard", RequiredBeats: []string{"arrive"}, ForbiddenOutcomes: []string{"teleport"}, KnowledgeBoundary: []string{"Mira does not know the code"}, InventoryConstraints: []string{"no key"}, ForeshadowObligations: []string{}, EndingHook: "the bell rings"})
	return body
}

func TestQualityAPIRequiresProviderAndSafeEnvelope(t *testing.T) {
	app, err := New(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	created, err := app.projects.Create(t.Context(), project.CreateInput{Title: "No Provider"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/projects/"+created.ID+"/chapters/1/generate", bytes.NewReader(apiPlan(1)))
	request.Header.Set("Idempotency-Key", "quality-no-provider")
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"code":"QUALITY_SERVICE_UNAVAILABLE"`) || !strings.Contains(response.Body.String(), `"trace_id"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), app.projects.Workspace()) {
		t.Fatalf("absolute path leaked: %s", response.Body.String())
	}
}

func TestQualityAPIGenerateCheckFinalizeAndReplay(t *testing.T) {
	writer := &apiWriter{}
	librarian := &apiLibrarian{}
	editor := &apiEditor{}
	app, err := New(Config{Workspace: t.TempDir(), QualityWriter: writer, QualityLibrarian: librarian, QualityEditor: editor})
	if err != nil {
		t.Fatal(err)
	}
	created, err := app.projects.Create(t.Context(), project.CreateInput{Title: "Quality API"})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/projects/" + created.ID + "/chapters/1"

	generate := httptest.NewRequest(http.MethodPost, base+"/generate", bytes.NewReader(apiPlan(1)))
	generate.Header.Set("Idempotency-Key", "quality-generate")
	generated := httptest.NewRecorder()
	app.Handler().ServeHTTP(generated, generate)
	if generated.Code != http.StatusAccepted || !strings.Contains(generated.Body.String(), `"state":"draft_ready"`) {
		t.Fatalf("generate status=%d body=%s", generated.Code, generated.Body.String())
	}

	replay := httptest.NewRequest(http.MethodPost, base+"/generate", bytes.NewReader(apiPlan(1)))
	replay.Header.Set("Idempotency-Key", "quality-generate")
	replayed := httptest.NewRecorder()
	app.Handler().ServeHTTP(replayed, replay)
	if replayed.Code != http.StatusAccepted || replayed.Header().Get("Idempotency-Replayed") != "true" || writer.calls != 1 {
		t.Fatalf("generate replay status=%d header=%q writer=%d", replayed.Code, replayed.Header().Get("Idempotency-Replayed"), writer.calls)
	}

	check := httptest.NewRequest(http.MethodPost, base+"/check", bytes.NewReader([]byte(`{}`)))
	check.Header.Set("Idempotency-Key", "quality-check")
	checked := httptest.NewRecorder()
	app.Handler().ServeHTTP(checked, check)
	if checked.Code != http.StatusAccepted || !strings.Contains(checked.Body.String(), `"state":"final_candidate"`) || !strings.Contains(checked.Body.String(), `"score":8.5`) {
		t.Fatalf("check status=%d body=%s", checked.Code, checked.Body.String())
	}

	finalize := httptest.NewRequest(http.MethodPost, base+"/finalize", bytes.NewReader([]byte(`{}`)))
	finalize.Header.Set("Idempotency-Key", "quality-finalize")
	finalized := httptest.NewRecorder()
	app.Handler().ServeHTTP(finalized, finalize)
	if finalized.Code != http.StatusAccepted || !strings.Contains(finalized.Body.String(), `"state":"completed"`) {
		t.Fatalf("finalize status=%d body=%s", finalized.Code, finalized.Body.String())
	}
	if writer.calls != 1 || librarian.calls != 1 || editor.calls != 1 {
		t.Fatalf("unexpected model-stage calls writer=%d librarian=%d editor=%d", writer.calls, librarian.calls, editor.calls)
	}

	state := httptest.NewRecorder()
	app.Handler().ServeHTTP(state, httptest.NewRequest(http.MethodGet, base+"/quality", nil))
	if state.Code != http.StatusOK || !strings.Contains(state.Body.String(), `"state":"completed"`) || strings.Contains(state.Body.String(), "A durable generated chapter") {
		t.Fatalf("state status=%d body=%s", state.Code, state.Body.String())
	}
	candidates := httptest.NewRecorder()
	app.Handler().ServeHTTP(candidates, httptest.NewRequest(http.MethodGet, base+"/candidates", nil))
	if candidates.Code != http.StatusOK || !strings.Contains(candidates.Body.String(), `"total":1`) || strings.Contains(candidates.Body.String(), "A durable generated chapter") {
		t.Fatalf("candidates status=%d body=%s", candidates.Code, candidates.Body.String())
	}
}

func TestQualityAPIRejectsUnknownFieldsAndMissingIdempotency(t *testing.T) {
	writer := &apiWriter{}
	app, err := New(Config{Workspace: t.TempDir(), QualityWriter: writer, QualityLibrarian: &apiLibrarian{}, QualityEditor: &apiEditor{}})
	if err != nil {
		t.Fatal(err)
	}
	created, _ := app.projects.Create(t.Context(), project.CreateInput{Title: "Strict API"})
	base := "/api/projects/" + created.ID + "/chapters/1/generate"

	missing := httptest.NewRecorder()
	app.Handler().ServeHTTP(missing, httptest.NewRequest(http.MethodPost, base, bytes.NewReader(apiPlan(1))))
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "IDEMPOTENCY_KEY_REQUIRED") {
		t.Fatalf("missing key status=%d body=%s", missing.Code, missing.Body.String())
	}

	var raw map[string]any
	_ = json.Unmarshal(apiPlan(1), &raw)
	raw["unknown"] = true
	body, _ := json.Marshal(raw)
	request := httptest.NewRequest(http.MethodPost, base, bytes.NewReader(body))
	request.Header.Set("Idempotency-Key", "quality-strict")
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "REQUEST_BODY_INVALID") || writer.calls != 0 {
		t.Fatalf("strict status=%d writer=%d body=%s", response.Code, writer.calls, response.Body.String())
	}
}
