package narrativeledger

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
)

func openTestStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	db, err := migrate.Open(filepath.Join(t.TempDir(), "ledger.db"), 5*time.Second)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if _, err := db.Exec(Migration().SQL); err != nil {
		db.Close()
		t.Fatalf("apply migration: %v", err)
	}
	store := NewStore(db).WithClock(func() time.Time {
		return time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	})
	t.Cleanup(func() { _ = store.Close() })
	return store, db
}

func intValue(value int) *int { return &value }
func priorityValue(value Priority) *Priority { return &value }
func foreshadowStatusValue(value ForeshadowStatus) *ForeshadowStatus { return &value }
func secretStatusValue(value SecretStatus) *SecretStatus { return &value }

func TestAcceptedFinalReplayAndContentConflict(t *testing.T) {
	store, db := openTestStore(t)
	ctx := context.Background()
	input := ChangeSet{
		Source: Source{
			TransactionID: "tx-1",
			CandidateID:   "candidate-final",
			Chapter:       1,
			Authority:     AuthorityAcceptedFinal,
			Provenance:    map[string]string{"proposal": "fact-proposal-1"},
		},
		Foreshadows: []ForeshadowChange{{
			Action:     "create",
			Key:        "Moon Seal",
			Title:      "The moon seal cracks",
			Priority:   priorityValue(PriorityCritical),
			DueChapter: intValue(3),
		}},
	}
	first, err := store.ApplyAcceptedFinal(ctx, input)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if first.Replay {
		t.Fatal("first apply unexpectedly marked as replay")
	}
	second, err := store.ApplyAcceptedFinal(ctx, input)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !second.Replay || second.Commit.ContentHash != first.Commit.ContentHash {
		t.Fatalf("replay mismatch: %#v", second)
	}
	var commits, events int
	if err := db.QueryRow(`SELECT COUNT(*) FROM narrative_ledger_commits`).Scan(&commits); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM foreshadow_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if commits != 1 || events != 1 {
		t.Fatalf("replay duplicated rows: commits=%d events=%d", commits, events)
	}
	changed := input
	changed.Foreshadows = append([]ForeshadowChange{}, input.Foreshadows...)
	changed.Foreshadows[0].Title = "different content"
	if _, err := store.ApplyAcceptedFinal(ctx, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected content conflict, got %v", err)
	}
	retrieval := input
	retrieval.Source.TransactionID = "rag-1"
	retrieval.Source.Authority = AuthorityRetrieval
	if _, err := store.ApplyAcceptedFinal(ctx, retrieval); !errors.Is(err, ErrAuthority) {
		t.Fatalf("retrieval authority wrote ledger: %v", err)
	}
}

func TestScenarioEOverduePlannerAndSecretBoundary(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	_, err := store.ApplyAcceptedFinal(ctx, ChangeSet{
		Source: Source{TransactionID: "scenario-e-1", CandidateID: "final-1", Chapter: 1, Authority: AuthorityAcceptedFinal},
		Foreshadows: []ForeshadowChange{
			{
				Action:         "create",
				Key:            "broken-crown",
				Title:          "The broken crown must answer",
				Priority:       priorityValue(PriorityCritical),
				PlantedChapter: intValue(1),
				DueChapter:     intValue(2),
			},
			{
				Action:     "create",
				Key:        "river-map",
				Title:      "Return to the river map",
				Priority:   priorityValue(PriorityNormal),
				DueChapter: intValue(5),
			},
		},
		Secrets: []SecretChange{{
			Action:            "create",
			Key:               "heir-identity",
			Title:             "Identity of the hidden heir",
			Description:       "The archivist is the missing heir.",
			Status:            secretStatusValue(SecretHidden),
			PublicFromChapter: intValue(5),
			Knowledge: []SecretKnowledgeChange{
				{Holder: "Alice", KnownFromChapter: 1, KnownUntilChapter: intValue(3)},
				{Holder: "Bob", KnownFromChapter: 2},
			},
		}},
	})
	if err != nil {
		t.Fatalf("seed scenario E: %v", err)
	}

	overdue, err := store.GetForeshadow(ctx, "broken-crown", 3)
	if err != nil {
		t.Fatal(err)
	}
	if overdue.Status == ForeshadowOverdue || overdue.EffectiveStatus != ForeshadowOverdue {
		t.Fatalf("OVERDUE must be computed, stored=%q effective=%q", overdue.Status, overdue.EffectiveStatus)
	}
	planner, err := store.BuildPlannerContext(ctx, 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(planner.Items) < 2 || !planner.Items[0].Mandatory {
		t.Fatalf("mandatory ledger obligations missing: %#v", planner.Items)
	}
	if !strings.Contains(planner.Text, "OVERDUE MANDATORY") || !strings.Contains(planner.Text, "broken-crown") {
		t.Fatalf("planner context omitted overdue foreshadow: %s", planner.Text)
	}

	chapterTwo, err := store.GetSecret(ctx, "heir-identity", 2)
	if err != nil {
		t.Fatal(err)
	}
	if chapterTwo.Public || strings.Join(chapterTwo.Holders, ",") != "Alice,Bob" {
		t.Fatalf("chapter 2 boundary mismatch: %#v", chapterTwo)
	}
	chapterFour, err := store.GetSecret(ctx, "heir-identity", 4)
	if err != nil {
		t.Fatal(err)
	}
	if chapterFour.Public || strings.Join(chapterFour.Holders, ",") != "Bob" {
		t.Fatalf("chapter 4 boundary mismatch: %#v", chapterFour)
	}
	chapterFive, err := store.GetSecret(ctx, "heir-identity", 5)
	if err != nil {
		t.Fatal(err)
	}
	if !chapterFive.Public {
		t.Fatalf("secret should be public at chapter 5: %#v", chapterFive)
	}
	boundary, err := store.SecretBoundary(ctx, "heir-identity", 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, leaked := boundary["description"]; leaked {
		t.Fatalf("boundary response leaked secret description: %#v", boundary)
	}

	diagnostics, err := store.Diagnostics(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) == 0 || diagnostics[0].Code != "LEDGER_FORESHADOW_OVERDUE" {
		t.Fatalf("missing overdue diagnostic: %#v", diagnostics)
	}
	dashboard, err := store.Dashboard(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.ForeshadowsOverdue != 1 || dashboard.SecretsHidden != 1 || dashboard.SecretsPublic != 0 {
		t.Fatalf("dashboard is not derived from Chapter-N state: %#v", dashboard)
	}
}

func TestLifecycleAndDeterministicPagination(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := context.Background()
	for index, key := range []string{"gamma", "alpha", "beta"} {
		_, err := store.ApplyHuman(ctx, ChangeSet{
			Source: Source{TransactionID: "human-create-" + key, Chapter: index + 1, Authority: AuthorityHuman},
			Foreshadows: []ForeshadowChange{{
				Action:     "create",
				Key:        key,
				Title:      key,
				Priority:   priorityValue(PriorityNormal),
				DueChapter: intValue(10),
			}},
		})
		if err != nil {
			t.Fatalf("create %s: %v", key, err)
		}
	}
	page, err := store.ListForeshadows(ctx, ListOptions{AsOfChapter: 3, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].Key != "alpha" || page.Items[1].Key != "beta" || page.NextOffset == nil {
		t.Fatalf("pagination is not deterministic: %#v", page)
	}
	_, err = store.ApplyHuman(ctx, ChangeSet{
		Source: Source{TransactionID: "human-plant-alpha", Chapter: 4, Authority: AuthorityHuman},
		Foreshadows: []ForeshadowChange{{Action: "plant", Key: "alpha"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ApplyHuman(ctx, ChangeSet{
		Source: Source{TransactionID: "human-reveal-alpha", Chapter: 5, Authority: AuthorityHuman},
		Foreshadows: []ForeshadowChange{{Action: "reveal", Key: "alpha"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ApplyHuman(ctx, ChangeSet{
		Source: Source{TransactionID: "human-reopen-alpha", Chapter: 6, Authority: AuthorityHuman},
		Foreshadows: []ForeshadowChange{{Action: "plant", Key: "alpha", Status: foreshadowStatusValue(ForeshadowPlanted)}},
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("terminal foreshadow was reopened: %v", err)
	}
}

func TestMigrationViewsAndIndexes(t *testing.T) {
	store, db := openTestStore(t)
	ctx := context.Background()
	views := map[string]bool{}
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'view'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		views[name] = true
	}
	rows.Close()
	for _, name := range []string{"narrative_ledger_current_chapter", "foreshadow_status_view", "secret_status_view"} {
		if !views[name] {
			t.Fatalf("missing view %s", name)
		}
	}
	plans, err := store.ExplainForeshadowSchedule(ctx, 100, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(plans, "\n"), "idx_foreshadows_status_due_priority") {
		t.Fatalf("schedule query did not use required index: %v", plans)
	}
	secretPlans, err := store.ExplainSecretBoundary(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(secretPlans, "\n"), "idx_secret_knowledge_temporal") {
		t.Fatalf("secret boundary query did not use required index: %v", secretPlans)
	}
}
