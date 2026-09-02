package truthstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "truth.db")
	if err := (migrate.Runner{Path: path, Migrations: []migrate.Migration{{Version: 1, Name: "truth_test_base", SQL: "CREATE TABLE truth_test_base(id INTEGER PRIMARY KEY);"}, Migration()}}).Run(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store, err := OpenExisting(path, 500*time.Millisecond,
		WithClock(func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) }))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func input(key, subject, predicate string, value any, chapter int) AppendInput {
	data, _ := json.Marshal(value)
	return AppendInput{IdempotencyKey: key, Kind: EventAssert, SubjectType: "character",
		SubjectID: subject, Predicate: predicate, Value: data, ValidFromChapter: chapter,
		KnownFromChapter: chapter, Authority: AuthorityGeneratedFinal, Confidence: 1,
		Source: Source{Type: "chapter", ID: fmt.Sprintf("chapter-%d", chapter), Chapter: chapter, Version: "chapter-v1"}}
}

func TestChapterQueriesDoNotLeakFutureTruth(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.Append(ctx, input("future-location", "lin", "location.current", "North Gate", 10)); err != nil {
		t.Fatal(err)
	}
	before, err := store.State(ctx, StateQuery{Chapter: 9, SubjectID: "lin"})
	if err != nil {
		t.Fatal(err)
	}
	if before.Total != 0 {
		t.Fatalf("chapter 9 leaked %d future facts", before.Total)
	}
	after, err := store.State(ctx, StateQuery{Chapter: 10, SubjectID: "lin"})
	if err != nil {
		t.Fatal(err)
	}
	if after.Total != 1 || string(after.Facts[0].Value) != `"North Gate"` {
		t.Fatalf("chapter 10 state = %#v", after)
	}
}

func TestKnowledgeBoundaryPreventsRetroactiveLeakage(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	entry := input("secret-origin", "lin", "origin.city", "Old Capital", 1)
	entry.KnownFromChapter = 8
	if _, err := store.Append(ctx, entry); err != nil {
		t.Fatal(err)
	}
	atSeven, _ := store.State(ctx, StateQuery{Chapter: 7, SubjectID: "lin"})
	atEight, _ := store.State(ctx, StateQuery{Chapter: 8, SubjectID: "lin"})
	if atSeven.Total != 0 || atEight.Total != 1 {
		t.Fatalf("knowledge boundary totals = %d, %d", atSeven.Total, atEight.Total)
	}
}

func TestInventoryFactsRespectTemporalRanges(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	until := 5
	entry := input("inventory-sword", "lin", "inventory.sword", map[string]any{"count": 1}, 2)
	entry.ValidToChapter = &until
	if _, err := store.Append(ctx, entry); err != nil {
		t.Fatal(err)
	}
	atFive, _ := store.State(ctx, StateQuery{Chapter: 5, Predicate: "inventory.sword"})
	atSix, _ := store.State(ctx, StateQuery{Chapter: 6, Predicate: "inventory.sword"})
	if atFive.Total != 1 || atSix.Total != 0 {
		t.Fatalf("inventory temporal totals = %d, %d", atFive.Total, atSix.Total)
	}
}

func TestConcurrentIdempotentAppendCreatesOneEvent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	entry := input("same-request", "lin", "status.alive", true, 1)
	const workers = 16
	var wg sync.WaitGroup
	ids := make(chan string, workers)
	errs := make(chan error, workers)
	for index := 0; index < workers; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := store.Append(ctx, entry)
			if err != nil {
				errs <- err
				return
			}
			ids <- result.Event.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatalf("append: %v", err)
	}
	var first string
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatalf("idempotent append ids differ: %s != %s", id, first)
		}
	}
	page, err := store.Events(ctx, EventQuery{Limit: 100})
	if err != nil || len(page.Events) != 1 {
		t.Fatalf("events = %#v, %v", page, err)
	}
	changed := entry
	changed.Value = json.RawMessage(`false`)
	_, err = store.Append(ctx, changed)
	var storeErr *Error
	if !errors.As(err, &storeErr) || storeErr.Code != CodeIdempotencyConflict {
		t.Fatalf("changed replay error = %v", err)
	}
}

func TestConflictRequiresExplicitAuthorizedSupersede(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	first := input("canon-eye", "lin", "appearance.eye_color", "black", 1)
	first.Authority = AuthorityHumanFinal
	firstResult, err := store.Append(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	second := input("agent-eye", "lin", "appearance.eye_color", "blue", 2)
	second.Authority = AuthorityLLMSuggestion
	if _, err := store.Append(ctx, second); err != nil {
		t.Fatal(err)
	}
	state, _ := store.State(ctx, StateQuery{Chapter: 2, SubjectID: "lin", Predicate: "appearance.eye_color"})
	conflicts, _ := store.Conflicts(ctx, ConflictQuery{Chapter: intRef(2)})
	if state.Total != 2 || conflicts.Total != 1 {
		t.Fatalf("state/conflicts = %d/%d", state.Total, conflicts.Total)
	}
	lower := input("bad-supersede", "lin", "appearance.eye_color", "green", 3)
	lower.Kind = EventSupersede
	lower.Authority = AuthorityLLMSuggestion
	lower.SupersedesEventID = firstResult.Event.ID
	_, err = store.Append(ctx, lower)
	var storeErr *Error
	if !errors.As(err, &storeErr) || storeErr.Code != CodeAuthority {
		t.Fatalf("lower-authority supersede error = %v", err)
	}
}

func TestAuthorizedSupersedeClosesOldFact(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	first := input("old-title", "book", "title.current", "Old", 1)
	first.SubjectType = "project"
	first.Authority = AuthorityGeneratedFinal
	created, err := store.Append(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	replacement := input("new-title", "book", "title.current", "New", 5)
	replacement.SubjectType = "project"
	replacement.Kind = EventSupersede
	replacement.Authority = AuthorityHumanFinal
	replacement.SupersedesEventID = created.Event.ID
	if _, err := store.Append(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	atFour, _ := store.State(ctx, StateQuery{Chapter: 4, SubjectID: "book"})
	atFive, _ := store.State(ctx, StateQuery{Chapter: 5, SubjectID: "book"})
	if atFour.Total != 1 || string(atFour.Facts[0].Value) != `"Old"` || atFive.Total != 1 || string(atFive.Facts[0].Value) != `"New"` {
		t.Fatalf("supersede states = %#v / %#v", atFour, atFive)
	}
}

func TestBoundedRebuildRestoresProjectionAndDigest(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for chapter := 1; chapter <= 8; chapter++ {
		if _, err := store.Append(ctx, input(fmt.Sprintf("event-%d", chapter), fmt.Sprintf("c-%d", chapter), "status.marker", chapter, chapter)); err != nil {
			t.Fatal(err)
		}
	}
	before, err := store.Verify(ctx)
	if err != nil || !before.Valid {
		t.Fatalf("before verify = %#v, %v", before, err)
	}
	if _, err := store.db.Exec(`DELETE FROM truth_facts WHERE effective_from_chapter >= 5`); err != nil {
		t.Fatal(err)
	}
	broken, _ := store.Verify(ctx)
	if broken.Valid {
		t.Fatal("expected broken projection")
	}
	rebuilt, err := store.Rebuild(ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	after, err := store.Verify(ctx)
	if err != nil || !after.Valid || after.ProjectionDigest != before.ProjectionDigest || rebuilt.FromChapter != 5 {
		t.Fatalf("rebuild/verify = %#v / %#v / %v", rebuilt, after, err)
	}
}

func TestEventLogIsAppendOnly(t *testing.T) {
	store := newTestStore(t)
	result, err := store.Append(context.Background(), input("immutable", "lin", "status.alive", true, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE truth_events SET value_json='false' WHERE id=?`, result.Event.ID); err == nil {
		t.Fatal("truth event update unexpectedly succeeded")
	}
	if _, err := store.db.Exec(`DELETE FROM truth_events WHERE id=?`, result.Event.ID); err == nil {
		t.Fatal("truth event delete unexpectedly succeeded")
	}
}

func TestBusyStoreReturnsRetryableError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "busy.db")
	if err := (migrate.Runner{Path: path, Migrations: []migrate.Migration{{Version: 1, Name: "truth_test_base", SQL: "CREATE TABLE truth_test_base(id INTEGER PRIMARY KEY);"}, Migration()}}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	first, _ := OpenExisting(path, 50*time.Millisecond)
	second, _ := OpenExisting(path, 50*time.Millisecond)
	defer first.Close()
	defer second.Close()
	tx, err := first.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE truth_projection_meta SET last_sequence=last_sequence WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	_, err = second.Append(context.Background(), input("busy", "lin", "status.alive", true, 1))
	var storeErr *Error
	if !errors.As(err, &storeErr) || !storeErr.Retryable || storeErr.Code != CodeBusy {
		_ = tx.Rollback()
		t.Fatalf("busy error = %#v", err)
	}
	_ = tx.Rollback()
	if _, err := second.Append(context.Background(), input("after-busy", "lin", "status.alive", true, 1)); err != nil {
		t.Fatalf("append after rollback: %v", err)
	}
}

func intRef(value int) *int { return &value }

var _ *sql.DB

func TestPhase4RequiredTemporalBoundaries(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	futureState := input("future-state-100", "hero", "state.cultivation", "realm-9", 100)
	if _, err := store.Append(ctx, futureState); err != nil {
		t.Fatal(err)
	}
	inventory := input("inventory-180", "hero", "inventory.ancient_sword", map[string]any{"quantity": 1}, 180)
	if _, err := store.Append(ctx, inventory); err != nil {
		t.Fatal(err)
	}
	knowledge := input("knowledge-50", "hero", "knowledge.secret_x", true, 1)
	knowledge.KnownFromChapter = 50
	if _, err := store.Append(ctx, knowledge); err != nil {
		t.Fatal(err)
	}

	chapter50, err := store.State(ctx, StateQuery{Chapter: 50, Predicate: "state.cultivation"})
	if err != nil || chapter50.Total != 0 {
		t.Fatalf("chapter 100 state leaked into chapter 50: %#v, %v", chapter50, err)
	}
	chapter120, err := store.State(ctx, StateQuery{Chapter: 120, Predicate: "inventory.ancient_sword"})
	if err != nil || chapter120.Total != 0 {
		t.Fatalf("chapter 180 inventory leaked into chapter 120: %#v, %v", chapter120, err)
	}
	chapter49, err := store.State(ctx, StateQuery{Chapter: 49, Predicate: "knowledge.secret_x"})
	if err != nil || chapter49.Total != 0 {
		t.Fatalf("chapter 50 knowledge leaked into chapter 49: %#v, %v", chapter49, err)
	}
	chapter50Knowledge, err := store.State(ctx, StateQuery{Chapter: 50, Predicate: "knowledge.secret_x"})
	if err != nil || chapter50Knowledge.Total != 1 {
		t.Fatalf("chapter 50 knowledge missing: %#v, %v", chapter50Knowledge, err)
	}
}

func TestStateManyAndEventFiltersAreBoundedAndStable(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for chapter := 1; chapter <= 4; chapter++ {
		entry := input(fmt.Sprintf("batch-%d", chapter), fmt.Sprintf("hero-%d", chapter), "location.current", fmt.Sprintf("place-%d", chapter), chapter)
		if _, err := store.Append(ctx, entry); err != nil {
			t.Fatal(err)
		}
	}

	pages, err := store.StateMany(ctx, []StateQuery{
		{Chapter: 2, SubjectID: "hero-2", Limit: 10},
		{Chapter: 4, Predicate: "location.current", Limit: 2},
		{Chapter: 1, SubjectID: "missing", Limit: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 3 || pages[0].Total != 1 || pages[1].Total != 4 || len(pages[1].Facts) != 2 || pages[1].NextOffset == nil || pages[2].Total != 0 {
		t.Fatalf("batch pages = %#v", pages)
	}
	through := 2
	events, err := store.Events(ctx, EventQuery{ThroughChapter: &through, SubjectType: "character", Predicate: "location.current", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events.Events) != 2 || events.Events[0].Sequence >= events.Events[1].Sequence {
		t.Fatalf("filtered events = %#v", events)
	}
}

func TestSourceVersionAndAuthorityContract(t *testing.T) {
	store := newTestStore(t)
	entry := input("missing-source-version", "hero", "status.alive", true, 1)
	entry.Source.Version = ""
	_, err := store.Append(context.Background(), entry)
	var storeErr *Error
	if !errors.As(err, &storeErr) || storeErr.Code != CodeValidation {
		t.Fatalf("missing source version error = %v", err)
	}
	authorities := []Authority{
		AuthorityLLMSuggestion,
		AuthorityStoryCompass,
		AuthorityVolumePlan,
		AuthorityArcPlan,
		AuthorityChapterPlan,
		AuthorityGeneratedFinal,
		AuthorityHumanFinal,
	}
	previous := 0
	for _, authority := range authorities {
		rank, ok := authority.rank()
		if !ok || rank <= previous {
			t.Fatalf("authority %q rank = %d after %d", authority, rank, previous)
		}
		previous = rank
	}
}

func TestConflictHistoryBecomesResolvedAfterExplicitSupersede(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	first := input("conflict-first", "hero", "appearance.eye_color", "black", 1)
	first.Authority = AuthorityGeneratedFinal
	if _, err := store.Append(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := input("conflict-second", "hero", "appearance.eye_color", "blue", 2)
	second.Authority = AuthorityLLMSuggestion
	secondResult, err := store.Append(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	unresolved, err := store.Conflicts(ctx, ConflictQuery{Chapter: intRef(2), Status: ConflictUnresolved})
	if err != nil || unresolved.Total != 1 {
		t.Fatalf("unresolved conflicts = %#v, %v", unresolved, err)
	}
	replacement := input("conflict-resolution", "hero", "appearance.eye_color", "black", 3)
	replacement.Kind = EventSupersede
	replacement.Authority = AuthorityHumanFinal
	replacement.SupersedesEventID = secondResult.Event.ID
	if _, err := store.Append(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	unresolved, err = store.Conflicts(ctx, ConflictQuery{Chapter: intRef(3), Status: ConflictUnresolved})
	if err != nil || unresolved.Total != 0 {
		t.Fatalf("active conflicts after resolution = %#v, %v", unresolved, err)
	}
	resolved, err := store.Conflicts(ctx, ConflictQuery{Status: ConflictResolved})
	if err != nil || resolved.Total != 1 || resolved.Conflicts[0].ToChapter == nil {
		t.Fatalf("resolved conflict history = %#v, %v", resolved, err)
	}
}

func TestVerifyDetectsProjectionAuthorityCorruption(t *testing.T) {
	store := newTestStore(t)
	result, err := store.Append(context.Background(), input("verify-authority", "hero", "status.alive", true, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE truth_facts SET authority='llm_suggestion', authority_rank=10 WHERE event_id=?`, result.Event.ID); err != nil {
		t.Fatal(err)
	}
	verified, err := store.Verify(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if verified.Valid || len(verified.Violations) == 0 {
		t.Fatalf("corrupt projection was accepted: %#v", verified)
	}
}

func TestTruthMigrationPreservesExistingDataAndCreatesBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "project.db")
	base := migrate.Migration{Version: 1, Name: "legacy", SQL: `CREATE TABLE legacy_truth(id INTEGER PRIMARY KEY, value TEXT NOT NULL); INSERT INTO legacy_truth(value) VALUES ('kept')`}
	if err := (migrate.Runner{Path: path, Migrations: []migrate.Migration{base}}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := (migrate.Runner{Path: path, Migrations: []migrate.Migration{base, Migration()}}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := migrate.Open(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var kept, truthTables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM legacy_truth WHERE value='kept'`).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='truth_events'`).Scan(&truthTables); err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(filepath.Join(dir, "backups", "project-*.db"))
	if err != nil || kept != 1 || truthTables != 1 || len(backups) != 1 {
		t.Fatalf("migration kept=%d truth=%d backups=%v err=%v", kept, truthTables, backups, err)
	}
}

func TestKnowledgeMayPrecedeStoryValidity(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	entry := input("planned-future-event", "hero", "timeline.destination", "moon-palace", 20)
	entry.Authority = AuthorityChapterPlan
	entry.KnownFromChapter = 0
	result, err := store.Append(ctx, entry)
	if err != nil {
		t.Fatal(err)
	}
	if result.Event.KnownFromChapter != 0 {
		t.Fatalf("known_from_chapter = %d, want 0", result.Event.KnownFromChapter)
	}
	before, err := store.State(ctx, StateQuery{Chapter: 19, Predicate: "timeline.destination"})
	if err != nil || before.Total != 0 {
		t.Fatalf("future story state leaked before valid time: %#v, %v", before, err)
	}
	atBoundary, err := store.State(ctx, StateQuery{Chapter: 20, Predicate: "timeline.destination"})
	if err != nil || atBoundary.Total != 1 {
		t.Fatalf("planned state missing at valid boundary: %#v, %v", atBoundary, err)
	}
}

func TestStoreRejectsUnsafeIdempotencyKeys(t *testing.T) {
	store := newTestStore(t)
	entry := input("contains space", "hero", "status.alive", true, 1)
	_, err := store.Append(context.Background(), entry)
	var storeErr *Error
	if !errors.As(err, &storeErr) || storeErr.Code != CodeValidation {
		t.Fatalf("unsafe idempotency key error = %v", err)
	}
}

func TestTruthRejectsCredentialLookingValuesAndProvenance(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	valueSecret := input("secret-value", "hero", "profile.metadata", map[string]any{"api_key": "sk-do-not-store"}, 1)
	_, err := store.Append(ctx, valueSecret)
	var storeErr *Error
	if !errors.As(err, &storeErr) || storeErr.Code != CodeValidation {
		t.Fatalf("credential value error = %v", err)
	}

	sourceSecret := input("secret-source", "hero", "profile.note", "safe", 1)
	sourceSecret.Source.Excerpt = "Authorization: Bearer do-not-store"
	_, err = store.Append(ctx, sourceSecret)
	if !errors.As(err, &storeErr) || storeErr.Code != CodeValidation {
		t.Fatalf("credential provenance error = %v", err)
	}
}
