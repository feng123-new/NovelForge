package truthstore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHundredThousandFactTemporalQueryUsesIndex(t *testing.T) {
	if os.Getenv("NOVELFORGE_SCALE_TEST") != "1" {
		t.Skip("set NOVELFORGE_SCALE_TEST=1 for the 100k projection gate")
	}
	store := newTestStore(t)
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	eventStatement, err := tx.Prepare(`INSERT INTO truth_events(id, idempotency_key, request_hash,
		kind, subject_type, subject_id, predicate, value_json, valid_from_chapter,
		known_from_chapter, authority, confidence, source_type, source_id, source_chapter, source_version, created_at, checksum)
		VALUES (?, ?, 'scale', 'assert', 'character', ?, 'status.scale', ?, ?, ?, 'generated_final', 1, 'test', ?, ?, 'scale-v1', ?, 'scale')`)
	if err != nil {
		t.Fatal(err)
	}
	factStatement, err := tx.Prepare(`INSERT INTO truth_facts(event_id, sequence, subject_type,
		subject_id, predicate, value_json, value_hash, valid_from_chapter,
		known_from_chapter, effective_from_chapter, authority, authority_rank, confidence)
		VALUES (?, ?, 'character', ?, 'status.scale', ?, ?, ?, ?, ?, 'generated_final', 60, 1)`)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	for index := 0; index < 100_000; index++ {
		id := fmt.Sprintf("scale-%06d", index)
		subject := fmt.Sprintf("subject-%04d", index%1000)
		chapter := index % 500
		value := fmt.Sprintf("%d", index)
		if _, err := eventStatement.Exec(id, "key-"+id, subject, value, chapter, chapter, id, chapter, created); err != nil {
			t.Fatalf("event %d: %v", index, err)
		}
		if _, err := factStatement.Exec(id, index+1, subject, value, sha256Text(value), chapter, chapter, chapter); err != nil {
			t.Fatalf("fact %d: %v", index, err)
		}
	}
	_ = eventStatement.Close()
	_ = factStatement.Close()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	rows, err := store.db.Query(`EXPLAIN QUERY PLAN SELECT event_id FROM truth_facts
		WHERE subject_type='character' AND subject_id='subject-0042' AND predicate='status.scale'
		AND effective_from_chapter <= 300 AND (effective_to_chapter IS NULL OR effective_to_chapter >= 300)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	plan := ""
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan += detail + "\n"
	}
	if !strings.Contains(plan, "idx_truth_facts_asof") {
		t.Fatalf("temporal query did not use idx_truth_facts_asof:\n%s", plan)
	}
	page, err := store.State(context.Background(), StateQuery{Chapter: 300, SubjectType: "character", SubjectID: "subject-0042", Predicate: "status.scale", Limit: 500})
	if err != nil || page.Total == 0 {
		t.Fatalf("100k state query = %d, %v", page.Total, err)
	}
}

var _ sql.Result
