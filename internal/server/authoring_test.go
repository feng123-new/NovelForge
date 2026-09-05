package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/authoring"
	"github.com/voocel/ainovel-cli/internal/autopilot"
	"github.com/voocel/ainovel-cli/internal/project"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
)

func TestAuthoringAPIRevisionIsolationAndJobGuard(t *testing.T) {
	s, err := New(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := s.projects.Create(t.Context(), project.CreateInput{Title: "Craft"})
	if err != nil {
		t.Fatal(err)
	}
	url := "/api/projects/" + p.ID + "/authoring"
	e := authoring.Entry{Kind: "style", Title: "克制叙述", Markdown: "用动作代替概括。", Enabled: true, Pinned: true, Priority: 50}
	body, _ := json.Marshal(authoring.Mutation{ExpectedRevision: 1, Entry: &e})
	response := postJob(t, s, url, "add", string(body))
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	replay := postJob(t, s, url, "add", string(body))
	if replay.Code != 200 || replay.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatal("HTTP replay", replay.Code, replay.Body.String())
	}
	stale := postJob(t, s, url, "stale", string(body))
	if stale.Code != 409 {
		t.Fatal("stale revision", stale.Code)
	}
	bad := postJob(t, s, url, "unknown", `{"expected_revision":2,"unexpected":true}`)
	if bad.Code != 400 {
		t.Fatal("unknown fields allowed", bad.Code)
	}
	state := httptest.NewRecorder()
	s.Handler().ServeHTTP(state, httptest.NewRequest(http.MethodGet, url, nil))
	if state.Code != 200 || !strings.Contains(state.Body.String(), "克制叙述") {
		t.Fatal(state.Body.String())
	}
	missing := httptest.NewRecorder()
	s.Handler().ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/projects/missing/authoring", nil))
	if missing.Code != 404 {
		t.Fatal("unknown project exposed", missing.Code)
	}
	job, err := s.jobs.Enqueue(t.Context(), p.ID, "job", autopilot.Input{Idea: "Story", StartChapter: 1, TargetChapter: 1, ReviewEvery: 1})
	if err != nil {
		t.Fatal(err)
	}
	blocked := postJob(t, s, url, "while-running", string(body))
	if blocked.Code != 409 {
		t.Fatal("active job edits allowed", blocked.Code)
	}
	if _, err = s.jobs.Control(t.Context(), job.ID, "stop"); err != nil {
		t.Fatal(err)
	}
	lint := postJob(t, s, url+"/lint", "lint", `{"chapter":1,"text":"不由得，不由得。"}`)
	if lint.Code != 200 || !strings.Contains(lint.Body.String(), "PHRASE_OVERUSE") {
		t.Fatal("lint", lint.Code, lint.Body.String())
	}
	var spec map[string]any
	if err = json.Unmarshal(openAPISpec, &spec); err != nil {
		t.Fatal(err)
	}
	paths := spec["paths"].(map[string]any)
	for _, path := range []string{"/api/projects/{id}/authoring", "/api/projects/{id}/authoring/search", "/api/projects/{id}/authoring/lint"} {
		if paths[path] == nil {
			t.Fatal("missing contract", path)
		}
	}
}

type craftCapture struct {
	payloads map[string][]byte
	count    int
}

func (c *craftCapture) Invoke(ctx context.Context, operation string, payload []byte) ([]byte, qualitygate.ModelUsage, error) {
	c.count++
	c.payloads[operation] = append([]byte(nil), payload...)
	if operation == "editor:review" {
		b, _ := json.Marshal(qualitygate.EditorReview{Score: 8, Summary: "Reviewed"})
		return b, qualitygate.ModelUsage{}, nil
	}
	return []byte("不由得，不由得。Mira enters the silent hall."), qualitygate.ModelUsage{}, nil
}
func TestAuthoringActualModelRequestsAndReplay(t *testing.T) {
	model := &craftCapture{payloads: map[string][]byte{}}
	s, err := New(Config{Workspace: t.TempDir(), QualityModel: model})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := s.projects.Create(t.Context(), project.CreateInput{Title: "Craft requests"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := s.projects.OpenAuthoring(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	revision := int64(1)
	for _, e := range []authoring.Entry{{Kind: "skill", Role: "writing", Title: "Writing custom", Markdown: "WRITING_CUSTOM_MARKER", Enabled: true}, {Kind: "skill", Role: "polish", Title: "Polish custom", Markdown: "POLISH_CUSTOM_MARKER", Enabled: true}, {Kind: "skill", Role: "review", Title: "Review custom", Markdown: "REVIEW_CUSTOM_MARKER", Enabled: true}, {Kind: "skill", Role: "planning", Title: "Planning custom", Markdown: "PLAN_CUSTOM_MARKER", Enabled: true}, {Kind: "style", Title: "Style sample", Markdown: "STYLE_SAMPLE_MARKER", Enabled: true, Pinned: true}, {Kind: "knowledge", Title: "Reference", Markdown: "REFERENCE_MARKER", Enabled: true, Pinned: true}, {Kind: "knowledge", Title: "Future reference", Markdown: "FORBIDDEN_FUTURE_MARKER", Enabled: true, Pinned: true, FromChapter: 99}} {
		e.Priority = 50
		changed, err := store.Mutate(t.Context(), "add-"+e.Title, authoring.Mutation{ExpectedRevision: revision, Entry: &e})
		if err != nil {
			t.Fatal(err)
		}
		revision = changed.Revision
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c, cleanup, f := s.qualityCoordinator(req, p.ID)
	if f != nil {
		t.Fatal(f)
	}
	defer cleanup()
	var plan qualitygate.ChapterPlan
	if err = json.Unmarshal(apiPlan(1), &plan); err != nil {
		t.Fatal(err)
	}
	wr := qualitygate.WriterRequest{ProjectID: p.ID, Chapter: 1, TransactionID: "craft-draft", Plan: plan}
	if _, err = c.Writer.Write(t.Context(), wr); err != nil {
		t.Fatal(err)
	}
	draftPayload := string(model.payloads["writer:draft"])
	for _, marker := range []string{"WRITING_CUSTOM_MARKER", "STYLE_SAMPLE_MARKER", "REFERENCE_MARKER"} {
		if !strings.Contains(draftPayload, marker) {
			t.Fatal("actual Writer request missing", marker, draftPayload)
		}
	}
	if strings.Contains(draftPayload, "FORBIDDEN_FUTURE_MARKER") {
		t.Fatal("future leaked")
	}
	wr.Attempt = 1
	wr.PreviousDraft = "A previous draft."
	wr.Feedback = []string{"Improve clarity"}
	if _, err = c.Writer.Write(t.Context(), wr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(model.payloads["writer:draft"]), "POLISH_CUSTOM_MARKER") {
		t.Fatal("polish not in actual request")
	}
	review, err := c.Editor.Review(t.Context(), qualitygate.EditorRequest{ProjectID: p.ID, Chapter: 1, TransactionID: "craft-review", Candidate: qualitygate.Candidate{ID: "candidate", Chapter: 1, Text: "不由得，不由得。"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(model.payloads["editor:review"]), "REVIEW_CUSTOM_MARKER") || len(review.Weaknesses) == 0 || review.Score != 8 {
		t.Fatal("review skills/rules or score integrity", review)
	}
	planning, err := s.projects.PlanningContext(t.Context(), p.ID, 1, plan.POV, "craft-job")
	if err != nil || !strings.Contains(string(planning), "PLAN_CUSTOM_MARKER") {
		t.Fatal("planner selection", string(planning), err)
	}
	foundation, err := s.projects.CompilePlanningSkills(t.Context(), p.ID, "foundation-scope", "Reference")
	if err != nil || !strings.Contains(string(foundation), "PLAN_CUSTOM_MARKER") {
		t.Fatal("foundation selection", err)
	}
	before, err := s.projects.AutopilotFingerprint(t.Context(), p.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	rules := authoring.DefaultRules()
	rules.Enabled = false
	if _, err = store.Mutate(t.Context(), "rules-off", authoring.Mutation{ExpectedRevision: revision, Rules: &rules}); err != nil {
		t.Fatal(err)
	}
	after, err := s.projects.AutopilotFingerprint(t.Context(), p.ID, 1)
	if err != nil || before == after {
		t.Fatal("authoring edits didn't invalidate old plan", err)
	}
	// Immutable request selection prevents duplicate model work after a library edit.
	callsBefore := model.count
	wr.Attempt = 0
	wr.PreviousDraft = ""
	wr.Feedback = nil
	if _, err = c.Writer.Write(t.Context(), wr); err != nil {
		t.Fatal("selected request replay changed", err)
	}
	if model.count != callsBefore {
		t.Fatal("unexpected operation")
	}
}
