package narrativeledger

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
	"github.com/voocel/ainovel-cli/internal/truthstore"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "project.db")
	migrations := []migrate.Migration{{Version: 1, Name: "base", SQL: `CREATE TABLE project_metadata(id TEXT PRIMARY KEY);`}, Migration()}
	if err := (migrate.Runner{Path: path, Migrations: migrations}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	store, err := OpenExisting(path, time.Second, WithClock(func() time.Time { return time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC) }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func foreshadowInput() ForeshadowInput {
	return ForeshadowInput{Title: "The sealed gate", Description: "A gate that must reopen.", Importance: ImportanceCritical, PlantedChapter: 20, ExpectedPayoffMin: 100, ExpectedPayoffMax: 130, Status: StatusPlanted, RelatedEntities: []string{"hero"}, RelatedArcs: []string{"arc-2"}, LastProgressChapter: 20, Urgency: UrgencyHigh, SourceVersion: "chapter-v1", Authority: truthstore.AuthorityHumanFinal}
}

func TestForeshadowLifecycleAndComputedOverdue(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	created, err := store.CreateForeshadow(ctx, "project", "create-gate", foreshadowInput())
	if err != nil {
		t.Fatal(err)
	}
	progress := StatusProgressing
	at90, err := store.UpdateForeshadow(ctx, "project", created.ID, "progress-gate", ForeshadowPatch{Status: &progress, Chapter: 90, Reason: "progress"})
	if err != nil || at90.Overdue {
		t.Fatalf("progress = %#v, %v", at90, err)
	}
	at135, err := store.GetForeshadow(ctx, "project", created.ID, 135)
	if err != nil || !at135.Overdue || at135.OverdueByChapters != 5 || at135.Status == Status("overdue") {
		t.Fatalf("chapter 135 = %#v, %v", at135, err)
	}
	resolved := StatusResolved
	actual := 136
	final, err := store.UpdateForeshadow(ctx, "project", created.ID, "resolve-gate", ForeshadowPatch{Status: &resolved, ActualPayoff: &actual, Chapter: 136, Reason: "payoff"})
	if err != nil || final.Overdue || final.Status != StatusResolved {
		t.Fatalf("resolved = %#v, %v", final, err)
	}
	planned := StatusPlanned
	_, err = store.UpdateForeshadow(ctx, "project", created.ID, "illegal-reopen", ForeshadowPatch{Status: &planned, Chapter: 137, Reason: "illegal"})
	if !errors.Is(err, ErrStateConflict) {
		t.Fatalf("illegal transition error = %v", err)
	}
}

func TestScenarioEAppearsInDashboardDiagnosticsAndPlanner(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	created, err := store.CreateForeshadow(ctx, "project", "scenario-e", foreshadowInput())
	if err != nil {
		t.Fatal(err)
	}
	dashboard, err := store.Dashboard(ctx, "project", 135)
	if err != nil || dashboard.OverdueCount != 1 || dashboard.CriticalOverdue != 1 {
		t.Fatalf("dashboard = %#v, %v", dashboard, err)
	}
	diagnostics, err := store.Diagnostics(ctx, "project", 135)
	if err != nil || len(diagnostics) == 0 || diagnostics[0].Code != "OVERDUE_FORESHADOW" {
		t.Fatalf("diagnostics = %#v, %v", diagnostics, err)
	}
	planner, err := store.PlannerContext(ctx, "project", 135, "hero", "arc-2", 3)
	if err != nil || len(planner.Foreshadows) != 1 || planner.Foreshadows[0].ID != created.ID || !planner.Foreshadows[0].Mandatory || planner.Foreshadows[0].Kind != "overdue_foreshadow" {
		t.Fatalf("planner = %#v, %v", planner, err)
	}
}

func TestOverdueQueryIsStablePagedAndIndexed(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for index, title := range []string{"C", "A", "B"} {
		input := foreshadowInput()
		input.ID = "fs-" + title
		input.Title = title
		// deterministic importance variety
		if index == 0 {
			input.Importance = ImportanceCritical
		}
		if index == 1 {
			input.Importance = ImportanceHigh
		}
		if index == 2 {
			input.Importance = ImportanceMedium
		}
		if _, err := store.CreateForeshadow(ctx, "project", "create-"+title, input); err != nil {
			t.Fatal(err)
		}
	}
	overdue := true
	first, err := store.ListForeshadows(ctx, "project", ForeshadowQuery{CurrentChapter: 135, Overdue: &overdue, Limit: 2})
	if err != nil || first.Total != 3 || len(first.Foreshadows) != 2 || first.NextOffset == nil {
		t.Fatalf("first page = %#v, %v", first, err)
	}
	second, err := store.ListForeshadows(ctx, "project", ForeshadowQuery{CurrentChapter: 135, Overdue: &overdue, Limit: 2, Offset: *first.NextOffset})
	if err != nil || len(second.Foreshadows) != 1 || second.Foreshadows[0].ID == first.Foreshadows[0].ID {
		t.Fatalf("second page = %#v, %v", second, err)
	}
	plan, err := store.ForeshadowQueryPlan(ctx, "project", 135)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.ToLower(strings.Join(plan, " "))
	if !strings.Contains(joined, "idx_foreshadows_project_status_payoff") && !strings.Contains(joined, "idx_foreshadows_project_importance") {
		t.Fatalf("query plan did not use a Phase 6 index: %v", plan)
	}
}

func TestSecretHolderChapterBoundaryAndNoTruthLeak(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	to := 3
	secret, err := store.CreateSecret(ctx, "project", "secret-create", SecretInput{Description: "The heir's origin", Truth: "The heir is from the old capital", CreatedChapter: 1, PublicStatus: PublicPrivate, SourceVersion: "v1", Authority: truthstore.AuthorityHumanFinal, Holders: []HolderInput{{EntityID: "hero", ValidFromChapter: 2, ValidToChapter: &to, SourceVersion: "v1", Authority: truthstore.AuthorityHumanFinal, Provenance: truthstore.Source{Type: "chapter", ID: "chapter-2", Chapter: 2, Version: "v1"}}}})
	if err != nil {
		t.Fatal(err)
	}
	atOne, err := store.GetSecret(ctx, "project", secret.ID, 1, false)
	if err != nil || atOne.Truth != "" {
		t.Fatalf("safe secret = %#v, %v", atOne, err)
	}
	holders2, _ := store.SecretHolders(ctx, secret.ID, 2)
	holders4, _ := store.SecretHolders(ctx, secret.ID, 4)
	if len(holders2) != 1 || len(holders4) != 0 {
		t.Fatalf("holders = %#v / %#v", holders2, holders4)
	}
	ctx2, err := store.PlannerContext(ctx, "project", 2, "hero", "", 3)
	if err != nil || len(ctx2.KnownSecrets) != 1 || ctx2.KnownSecrets[0].Summary == "" {
		t.Fatalf("known context = %#v, %v", ctx2, err)
	}
	ctx4, err := store.PlannerContext(ctx, "project", 4, "hero", "", 3)
	if err != nil || len(ctx4.KnownSecrets) != 0 || len(ctx4.UnknownSecrets) != 1 || strings.Contains(ctx4.UnknownSecrets[0].Summary, "old capital") {
		t.Fatalf("unknown boundary = %#v, %v", ctx4, err)
	}
	public := PublicPublic
	reveal := 5
	if _, err := store.UpdateSecret(ctx, "project", secret.ID, "secret-reveal", SecretPatch{PublicStatus: &public, RevealedChapter: &reveal, Chapter: 5, Reason: "public reveal"}); err != nil {
		t.Fatal(err)
	}
	ctx5, err := store.PlannerContext(ctx, "project", 5, "other", "", 3)
	if err != nil || len(ctx5.KnownSecrets) != 1 {
		t.Fatalf("public context = %#v, %v", ctx5, err)
	}
}

func TestAcceptedFinalOnlyAndReplaySafe(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	object, _ := json.Marshal(map[string]any{"id": "accepted-gate", "title": "Accepted", "description": "Accepted final only", "importance": "high", "planted_chapter": 20, "expected_payoff_min": 100, "expected_payoff_max": 130, "status": "planted", "last_progress_chapter": 20, "urgency": "normal"})
	change := AcceptedChange{Subject: "foreshadow:accepted-gate", Predicate: "foreshadow.upsert", Object: object, SourceChapter: 20, SourceVersion: "final-v1", SourceSHA: "sha", Extractor: "librarian", Confidence: 1, Authority: truthstore.AuthorityGeneratedFinal, ValidFromChapter: 20, KnownFromChapter: 20, Reason: "accepted"}
	input := AcceptedFinalInput{ProjectID: "project", TransactionID: "tx-1", ProposalID: "proposal-1", CandidateID: "candidate-1", Chapter: 20, SourceVersion: "final-v1", IdempotencyKey: "finalize-1", ForeshadowUpdates: []AcceptedChange{change}}
	first, err := store.CommitAcceptedFinal(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CommitAcceptedFinal(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if first.CommitID != second.CommitID || !second.Replayed {
		t.Fatalf("commit replay = %#v / %#v", first, second)
	}
	var events int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM foreshadow_events WHERE foreshadow_id='accepted-gate'`).Scan(&events); err != nil || events != 1 {
		t.Fatalf("events=%d err=%v", events, err)
	}
	changed := input
	changed.CandidateID = "candidate-2"
	if _, err := store.CommitAcceptedFinal(ctx, changed); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed replay error=%v", err)
	}
}

func TestConcurrentIdempotencyCreatesOneForeshadow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	const workers = 8
	var wg sync.WaitGroup
	ids := make(chan string, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			item, err := store.CreateForeshadow(ctx, "project", "same-create", foreshadowInput())
			if err != nil {
				errs <- err
				return
			}
			ids <- item.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatalf("ids differ %q %q", id, first)
		}
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM foreshadows`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}
