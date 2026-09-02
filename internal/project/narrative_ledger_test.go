package project

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
	"github.com/voocel/ainovel-cli/internal/narrativeledger"
)

func TestCompletedQualityTransactionSynchronizesLedgerOnce(t *testing.T) {
	ctx := context.Background()
	db, err := migrate.Open(filepath.Join(t.TempDir(), "project.db"), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE chapter_transactions (
		id TEXT PRIMARY KEY,
		chapter_number INTEGER NOT NULL,
		state TEXT NOT NULL,
		final_candidate_id TEXT
	);
	CREATE TABLE fact_proposals (
		transaction_id TEXT NOT NULL,
		candidate_id TEXT NOT NULL,
		proposal_json TEXT NOT NULL
	);`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(narrativeledger.Migration().SQL); err != nil {
		t.Fatal(err)
	}
	proposal := `{
		"foreshadow_changes": [{
			"action": "create",
			"key": "glass-star",
			"title": "The glass star must break",
			"priority": "critical",
			"due_chapter": 4
		}],
		"secret_changes": [{
			"action": "create",
			"key": "star-maker",
			"title": "Who made the star",
			"status": "hidden",
			"public_from_chapter": 7,
			"holders": ["Archivist"]
		}]
	}`
	if _, err := db.Exec(`INSERT INTO chapter_transactions(id, chapter_number, state, final_candidate_id)
		VALUES ('tx-final', 2, 'completed', 'candidate-final')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO fact_proposals(transaction_id, candidate_id, proposal_json)
		VALUES ('tx-final', 'candidate-final', ?)`, proposal); err != nil {
		t.Fatal(err)
	}
	store := narrativeledger.NewStore(db)
	if err := syncCompletedQualityTransactions(ctx, db, store); err != nil {
		t.Fatal(err)
	}
	if err := syncCompletedQualityTransactions(ctx, db, store); err != nil {
		t.Fatalf("replay synchronization failed: %v", err)
	}
	foreshadow, err := store.GetForeshadow(ctx, "glass-star", 5)
	if err != nil {
		t.Fatal(err)
	}
	if foreshadow.EffectiveStatus != narrativeledger.ForeshadowOverdue {
		t.Fatalf("accepted proposal did not become an overdue ledger item: %#v", foreshadow)
	}
	secret, err := store.GetSecret(ctx, "star-maker", 6)
	if err != nil {
		t.Fatal(err)
	}
	if secret.Public || len(secret.Holders) != 1 || secret.Holders[0] != "Archivist" {
		t.Fatalf("secret boundary mismatch: %#v", secret)
	}
	var commits, foreshadowEvents, secretEvents int
	if err := db.QueryRow(`SELECT COUNT(*) FROM narrative_ledger_commits`).Scan(&commits); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM foreshadow_events`).Scan(&foreshadowEvents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM secret_events`).Scan(&secretEvents); err != nil {
		t.Fatal(err)
	}
	if commits != 1 || foreshadowEvents != 1 || secretEvents != 1 {
		t.Fatalf("Finalize replay duplicated ledger events: commits=%d foreshadow=%d secret=%d", commits, foreshadowEvents, secretEvents)
	}
}

func TestDraftTransactionCannotSynchronizeLedger(t *testing.T) {
	ctx := context.Background()
	db, err := migrate.Open(filepath.Join(t.TempDir(), "project.db"), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE chapter_transactions (id TEXT PRIMARY KEY, chapter INTEGER, state TEXT);
	CREATE TABLE fact_proposals (transaction_id TEXT, proposal_json TEXT);`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(narrativeledger.Migration().SQL); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chapter_transactions VALUES ('draft-tx', 1, 'draft')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO fact_proposals VALUES ('draft-tx', '{"foreshadow_changes":[{"key":"must-not-write","action":"create"}]}')`); err != nil {
		t.Fatal(err)
	}
	store := narrativeledger.NewStore(db)
	if err := syncCompletedQualityTransactions(ctx, db, store); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM narrative_ledger_commits`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("draft transaction wrote Narrative Ledger: %d", count)
	}
}
