package qualitygate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
	"github.com/voocel/ainovel-cli/internal/truthstore"
)

func openQualityTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "project.db")
	db, err := migrate.Open(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(Migration().SQL); err != nil {
		_ = db.Close()
		t.Fatalf("apply quality migration: %v", err)
	}
	_ = db.Close()
	store, err := OpenExisting(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func openTruthTestStore(t *testing.T) *truthstore.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "truth.db")
	db, err := migrate.Open(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(truthstore.Migration().SQL); err != nil {
		_ = db.Close()
		t.Fatalf("apply truth migration: %v", err)
	}
	_ = db.Close()
	store, err := truthstore.OpenExisting(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func proposalFor(candidate Candidate, projectID string, changes ...FactChange) FactProposal {
	for i := range changes {
		changes[i].SourceChapter = candidate.Chapter
		changes[i].SourceVersion = candidate.SourceVersion
		changes[i].SourceSHA = candidate.TextSHA
		changes[i].Extractor = "test-librarian"
		if changes[i].Confidence == 0 {
			changes[i].Confidence = .9
		}
		if changes[i].ProposedAuthority == "" {
			changes[i].ProposedAuthority = "generated_final"
		}
		if changes[i].ValidFromChapter == 0 {
			changes[i].ValidFromChapter = candidate.Chapter
		}
		if changes[i].KnownFromChapter == 0 {
			changes[i].KnownFromChapter = candidate.Chapter
		}
		if changes[i].Reason == "" {
			changes[i].Reason = "draft explicitly establishes the fact"
		}
	}
	return FactProposal{
		ProposalID:       "proposal-" + candidate.ID,
		ProjectID:        projectID,
		Chapter:          candidate.Chapter,
		SourceVersion:    candidate.SourceVersion,
		SourceSHA:        candidate.TextSHA,
		Extractor:        "test-librarian",
		Authority:        "generated_final",
		CharacterChanges: changes,
		EntityChanges:    []FactChange{}, RelationshipChanges: []FactChange{}, LocationChanges: []FactChange{},
		InventoryChanges: []FactChange{}, KnowledgeChanges: []FactChange{}, TimelineEvents: []FactChange{},
		WorldFacts: []FactChange{}, ForeshadowUpdates: []FactChange{}, Secrets: []FactChange{}, Injuries: []FactChange{}, CultivationChanges: []FactChange{},
		Diagnostics: []string{},
	}
}

type repairerFunc func(context.Context, string, []byte, error) ([]byte, error)

func (f repairerFunc) Repair(ctx context.Context, schema string, raw []byte, err error) ([]byte, error) {
	return f(ctx, schema, raw, err)
}

func TestStrictDecoderRepairsMalformedJSONAndRejectsUnknownOrTrailingData(t *testing.T) {
	valid := []byte(`{"score":8,"strengths":[],"weaknesses":[],"line_level_issues":[],"pacing":"steady","characterization":"consistent","prose":"clear","dialogue":"natural","ending":"hook","rewrite_recommended":false,"summary":"ok"}`)
	calls := 0
	decoder := StrictDecoder{MaxRepairs: 1, Repairer: repairerFunc(func(context.Context, string, []byte, error) ([]byte, error) {
		calls++
		return valid, nil
	})}
	var review EditorReview
	repairs, err := decoder.Decode(context.Background(), "EditorReview", []byte(`{"score":`), &review)
	if err != nil || repairs != 1 || calls != 1 || review.Score != 8 {
		t.Fatalf("repair result repairs=%d calls=%d score=%v err=%v", repairs, calls, review.Score, err)
	}
	for _, raw := range [][]byte{
		[]byte(`{"score":8,"unknown":true}`),
		append(valid, []byte(` {}`)...),
	} {
		var got EditorReview
		if _, err := (StrictDecoder{}).Decode(context.Background(), "EditorReview", raw, &got); err == nil {
			t.Fatalf("expected strict decode failure for %s", raw)
		}
	}
	var never EditorReview
	if repairs, err := (StrictDecoder{MaxRepairs: 1, Repairer: repairerFunc(func(context.Context, string, []byte, error) ([]byte, error) {
		return []byte(`{"score":`), nil
	})}).Decode(context.Background(), "EditorReview", []byte(`{`), &never); err == nil || repairs != 1 {
		t.Fatalf("repair cap not enforced repairs=%d err=%v", repairs, err)
	}
}

func TestFactProposalValidationRequiresMatchingProvenance(t *testing.T) {
	candidate := Candidate{ID: "c", Chapter: 12, SourceVersion: "v1", TextSHA: "sha"}
	proposal := proposalFor(candidate, "project", FactChange{Subject: "character:hero", Predicate: "location", Object: json.RawMessage(`"tower"`)})
	if err := proposal.Validate(); err != nil {
		t.Fatal(err)
	}
	proposal.CharacterChanges[0].SourceVersion = "other"
	if err := proposal.Validate(); err == nil {
		t.Fatal("expected provenance mismatch")
	}
}

func TestStateMachineAndDefaultRewritePolicy(t *testing.T) {
	if got := DefaultPolicy().MaxRewrites; got != 2 {
		t.Fatalf("default max_rewrites=%d", got)
	}
	if err := ValidateTransition(StatePlanned, StateFinalCandidate); err == nil {
		t.Fatal("illegal jump accepted")
	}
	if !DefaultPolicy().Allows(ContinuityResult{Status: ContinuityWarn}) {
		t.Fatal("default WARN policy should be deterministic and permissive")
	}
	if DefaultPolicy().Allows(ContinuityResult{Status: ContinuityFail, Blocking: true}) {
		t.Fatal("FAIL must never be accepted")
	}
}

type countingInvoker struct {
	mu        sync.Mutex
	calls     int
	failures  int
	code      string
	retryable bool
}

func (f *countingInvoker) Invoke(_ context.Context, _ string, _ []byte) ([]byte, ModelUsage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	usage := ModelUsage{Provider: "fake", Model: "deterministic", InputTokens: 10, OutputTokens: 5}
	if f.failures > 0 {
		f.failures--
		return nil, usage, &ModelCallError{Code: f.code, Retryable: f.retryable, Err: errors.New(f.code)}
	}
	return []byte(`{"ok":true}`), usage, nil
}

func TestModelCallIdempotencyConflictAndBoundedRetry(t *testing.T) {
	store := openQualityTestStore(t)
	invoker := &countingInvoker{}
	caller := IdempotentModelCaller{Repository: store, Invoker: invoker, NewID: func() string { return fmt.Sprintf("call-%d", time.Now().UnixNano()) }}
	req := CallRequest{IdempotencyKey: "same-key", ProjectID: "p", Chapter: 1, TransactionID: "tx", Agent: "librarian", Operation: "propose", Payload: []byte("one")}
	first, call, replayed, err := caller.Call(context.Background(), req)
	if err != nil || replayed || string(first) != `{"ok":true}` || call.InputTokens != 10 {
		t.Fatalf("first call response=%s replay=%v call=%+v err=%v", first, replayed, call, err)
	}
	_, _, replayed, err = caller.Call(context.Background(), req)
	if err != nil || !replayed || invoker.calls != 1 {
		t.Fatalf("idempotent replay calls=%d replay=%v err=%v", invoker.calls, replayed, err)
	}
	req.Payload = []byte("different")
	if _, _, _, err := caller.Call(context.Background(), req); !errors.Is(err, ErrIdempotencyConflict) || invoker.calls != 1 {
		t.Fatalf("expected content conflict without extra call; calls=%d err=%v", invoker.calls, err)
	}

	for _, tc := range []struct {
		name string
		code string
	}{
		{"timeout", "MODEL_TIMEOUT"}, {"rate", "MODEL_429"}, {"server", "MODEL_5XX"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &countingInvoker{failures: 2, code: tc.code, retryable: true}
			retrying := RetryingModelInvoker{Invoker: fake, MaxRetries: 2}
			response, _, err := retrying.Invoke(context.Background(), "test", nil)
			if err != nil || string(response) != `{"ok":true}` || fake.calls != 3 {
				t.Fatalf("bounded retry response=%s calls=%d err=%v", response, fake.calls, err)
			}
		})
	}
	permanent := &countingInvoker{failures: 10, code: "MODEL_5XX", retryable: true}
	_, _, err = (RetryingModelInvoker{Invoker: permanent, MaxRetries: 2}).Invoke(context.Background(), "test", nil)
	if err == nil || permanent.calls != 3 {
		t.Fatalf("retry cap calls=%d err=%v", permanent.calls, err)
	}
}

type fakeWriter struct {
	mu    sync.Mutex
	calls int
	fail  bool
}

func (w *fakeWriter) Write(_ context.Context, req WriterRequest) (WriterResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls++
	if w.fail {
		return WriterResult{}, errors.New("writer failed")
	}
	return WriterResult{Text: fmt.Sprintf("draft-%d", req.Attempt), SourceVersion: fmt.Sprintf("v%d", req.Attempt)}, nil
}

type fakeLibrarian struct {
	calls  int
	fail   bool
	change FactChange
}

func (l *fakeLibrarian) Propose(_ context.Context, req LibrarianRequest) (FactProposal, error) {
	l.calls++
	if l.fail {
		return FactProposal{}, errors.New("librarian failed")
	}
	return proposalFor(req.Candidate, req.ProjectID, l.change), nil
}

type fixedContinuity struct{ result ContinuityResult }

func (f fixedContinuity) Check(context.Context, ContinuityRequest) (ContinuityResult, error) {
	return f.result, nil
}

type fixedEditor struct {
	score float64
	calls int
}

func (f *fixedEditor) Review(context.Context, EditorRequest) (EditorReview, error) {
	f.calls++
	return EditorReview{Score: f.score, Strengths: []string{"clear"}, Weaknesses: []string{}, LineLevelIssues: []string{}, Summary: "fixed"}, nil
}

type oneShotFault struct {
	point string
	mu    sync.Mutex
	used  bool
}

func (f *oneShotFault) Fail(point string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if point == f.point && !f.used {
		f.used = true
		return errors.New("injected " + point)
	}
	return nil
}

type memoryFinalWriter struct {
	mu      sync.Mutex
	calls   int
	content string
}

func (w *memoryFinalWriter) WriteFinalChapter(_ context.Context, _ string, _ int, content, expectedSHA string) error {
	if HashText(content) != expectedSHA {
		return errors.New("hash mismatch")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls++
	w.content = content
	return nil
}
