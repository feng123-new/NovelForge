package narrativeledger

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestScheduleIndexGate100K(t *testing.T) {
	store, db := openTestStore(t)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO narrative_ledger_commits(
		source_transaction_id, source_candidate_id, chapter, authority,
		content_hash, provenance_json, committed_at
	) VALUES ('perf-source', 'perf-candidate', 100000, 'accepted_final', 'perf', '{}', ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	statement, err := tx.Prepare(`INSERT INTO foreshadows(
		id, key, title, description, priority, status, planted_chapter,
		due_chapter, reveal_chapter, source_transaction_id, updated_chapter,
		created_at, updated_at
	) VALUES (?, ?, ?, '', ?, 'planted', 1, ?, NULL, 'perf-source', 1, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for index := 0; index < 100000; index++ {
		priority := PriorityNormal
		if index%97 == 0 {
			priority = PriorityCritical
		}
		key := fmt.Sprintf("perf-%06d", index)
		if _, err := statement.Exec(key, key, key, priority, index%1000, now, now); err != nil {
			statement.Close()
			tx.Rollback()
			t.Fatalf("insert %d: %v", index, err)
		}
	}
	if err := statement.Close(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	plans, err := store.ExplainForeshadowSchedule(ctx, 500, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(plans, "\n"), "idx_foreshadows_status_due_priority") {
		t.Fatalf("100k schedule query did not use the blocking index: %v", plans)
	}
	started := time.Now()
	page, err := store.ListForeshadows(ctx, ListOptions{AsOfChapter: 500, Status: "overdue", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 100 || page.Total == 0 {
		t.Fatalf("unexpected 100k schedule result: %#v", page)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("100k schedule query exceeded five-second gate: %s", elapsed)
	}
}
