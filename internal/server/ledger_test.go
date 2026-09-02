package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/narrativeledger"
)

func createLedgerProject(t *testing.T, app *Server) ProjectDetail {
	t.Helper()
	recorder := performJSON(t, app, http.MethodPost, "/api/projects", "ledger-project", `{
		"title":"Ledger Test",
		"genre":"fantasy",
		"language":"en",
		"target_words":100000,
		"target_chapters":200,
		"words_per_chapter":2500
	}`)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create project status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var created ProjectDetail
	decodeRecorder(t, recorder, &created)
	return created
}

func TestNarrativeLedgerHTTPScenarioEAndIdempotency(t *testing.T) {
	t.Parallel()
	app, err := New(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	project := createLedgerProject(t, app)
	path := "/api/projects/" + project.ID + "/foreshadows"
	body := `{
		"id":"sealed-gate",
		"title":"The sealed gate",
		"description":"The gate must reopen.",
		"importance":"critical",
		"planted_chapter":20,
		"expected_payoff_min":100,
		"expected_payoff_max":130,
		"status":"planted",
		"related_entities":["hero"],
		"related_arcs":["arc-2"],
		"last_progress_chapter":20,
		"urgency":"high",
		"source_version":"chapter-v1"
	}`
	first := performJSON(t, app, http.MethodPost, path, "scenario-e-create", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("create foreshadow status=%d body=%s", first.Code, first.Body.String())
	}
	var created narrativeledger.Foreshadow
	decodeRecorder(t, first, &created)
	if created.ID != "sealed-gate" || created.Overdue {
		t.Fatalf("created foreshadow=%#v", created)
	}
	replay := performJSON(t, app, http.MethodPost, path, "scenario-e-create", body)
	if replay.Code != http.StatusCreated || replay.Body.String() != first.Body.String() {
		t.Fatalf("idempotent replay status=%d body=%s want=%s", replay.Code, replay.Body.String(), first.Body.String())
	}
	conflict := performJSON(t, app, http.MethodPost, path, "scenario-e-create", strings.Replace(body, "The sealed gate", "Changed", 1))
	assertAPIError(t, conflict, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT")

	list := httptestGet(t, app, path+"?chapter=135&overdue=true&limit=10")
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var page narrativeledger.ForeshadowPage
	decodeRecorder(t, list, &page)
	if page.Total != 1 || len(page.Foreshadows) != 1 || !page.Foreshadows[0].Overdue || page.Foreshadows[0].OverdueByChapters != 5 {
		t.Fatalf("overdue page=%#v", page)
	}

	dashboard := httptestGet(t, app, "/api/projects/"+project.ID+"/ledger/dashboard?chapter=135")
	var metrics narrativeledger.Dashboard
	decodeRecorder(t, dashboard, &metrics)
	if dashboard.Code != http.StatusOK || metrics.OverdueCount != 1 || metrics.CriticalOverdue != 1 {
		t.Fatalf("dashboard status=%d metrics=%#v body=%s", dashboard.Code, metrics, dashboard.Body.String())
	}
	diagnostics := httptestGet(t, app, "/api/projects/"+project.ID+"/ledger/diagnostics?chapter=135")
	if diagnostics.Code != http.StatusOK || !strings.Contains(diagnostics.Body.String(), "OVERDUE_FORESHADOW") {
		t.Fatalf("diagnostics status=%d body=%s", diagnostics.Code, diagnostics.Body.String())
	}
	planner := httptestGet(t, app, "/api/projects/"+project.ID+"/ledger/planner-context?chapter=135&pov=hero&arc=arc-2")
	var context narrativeledger.PlannerContext
	decodeRecorder(t, planner, &context)
	if planner.Code != http.StatusOK || len(context.Foreshadows) != 1 || !context.Foreshadows[0].Mandatory || context.Foreshadows[0].Kind != "overdue_foreshadow" {
		t.Fatalf("planner status=%d context=%#v body=%s", planner.Code, context, planner.Body.String())
	}
}

func TestNarrativeLedgerSecretChapterBoundaryAndSafeResponse(t *testing.T) {
	t.Parallel()
	app, err := New(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	project := createLedgerProject(t, app)
	path := "/api/projects/" + project.ID + "/secrets"
	body := `{
		"id":"heir-origin",
		"description":"The heir's origin",
		"truth":"The heir is from the old capital",
		"created_chapter":1,
		"public_status":"private",
		"source_version":"v1",
		"holders":[{
			"entity_id":"hero",
			"valid_from_chapter":2,
			"valid_to_chapter":3,
			"source_version":"v1",
			"authority":"human_final",
			"provenance":{"type":"chapter","id":"chapter-2","chapter":2,"version":"v1"}
		}]
	}`
	create := performJSON(t, app, http.MethodPost, path, "secret-create", body)
	if create.Code != http.StatusCreated {
		t.Fatalf("secret create status=%d body=%s", create.Code, create.Body.String())
	}

	at2 := httptestGet(t, app, path+"/heir-origin?chapter=2")
	if at2.Code != http.StatusOK {
		t.Fatalf("secret chapter 2 status=%d body=%s", at2.Code, at2.Body.String())
	}
	var secret2 narrativeledger.Secret
	decodeRecorder(t, at2, &secret2)
	if secret2.Truth != "" || len(secret2.Holders) != 1 || secret2.PublicAtChapter {
		t.Fatalf("safe secret at 2=%#v", secret2)
	}
	at4 := httptestGet(t, app, path+"/heir-origin?chapter=4")
	var secret4 narrativeledger.Secret
	decodeRecorder(t, at4, &secret4)
	if at4.Code != http.StatusOK || secret4.Truth != "" || len(secret4.Holders) != 0 {
		t.Fatalf("safe secret at 4=%#v body=%s", secret4, at4.Body.String())
	}
	planner := httptestGet(t, app, "/api/projects/"+project.ID+"/ledger/planner-context?chapter=4&pov=hero")
	if planner.Code != http.StatusOK || strings.Contains(planner.Body.String(), "old capital") || !strings.Contains(planner.Body.String(), "unknown_secret_boundaries") {
		t.Fatalf("secret boundary response status=%d body=%s", planner.Code, planner.Body.String())
	}
	admin := httptestGet(t, app, path+"/heir-origin?chapter=4&include_truth=true")
	if admin.Code != http.StatusOK || !strings.Contains(admin.Body.String(), "old capital") {
		t.Fatalf("admin secret status=%d body=%s", admin.Code, admin.Body.String())
	}
}

func TestNarrativeLedgerStrictJSONAndBounds(t *testing.T) {
	t.Parallel()
	app, err := New(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	project := createLedgerProject(t, app)
	unknown := performJSON(t, app, http.MethodPost, "/api/projects/"+project.ID+"/foreshadows", "unknown-ledger-field", `{
		"title":"x","description":"x","importance":"low","planted_chapter":0,
		"expected_payoff_min":1,"expected_payoff_max":2,"status":"planned",
		"related_entities":[],"related_arcs":[],"last_progress_chapter":0,
		"urgency":"low","source_version":"v1","unknown":true
	}`)
	assertAPIError(t, unknown, http.StatusBadRequest, "REQUEST_BODY_INVALID")
	badChapter := httptestGet(t, app, "/api/projects/"+project.ID+"/foreshadows?chapter=-1")
	assertAPIError(t, badChapter, http.StatusBadRequest, "CHAPTER_BOUNDARY_INVALID")
	tooLarge := httptestGet(t, app, "/api/projects/"+project.ID+"/foreshadows?limit=101")
	assertAPIError(t, tooLarge, http.StatusBadRequest, "PAGINATION_INVALID")
}

func httptestGet(t *testing.T, app *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}
