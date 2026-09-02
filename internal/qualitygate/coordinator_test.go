package qualitygate

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/voocel/ainovel-cli/internal/narrativeledger"
	"github.com/voocel/ainovel-cli/internal/truthstore"
)

func TestCoordinatorRetainsDraftOnLibrarianFailureAndBlocksContinuityFail(t *testing.T) {
	store := openQualityTestStore(t)
	writer := &fakeWriter{}
	librarian := &fakeLibrarian{fail: true, change: FactChange{Subject: "character:hero", Predicate: "location", Object: json.RawMessage(`"tower"`)}}
	coordinator := Coordinator{Store: store, Writer: writer, Librarian: librarian, Continuity: fixedContinuity{ContinuityResult{Status: ContinuityPass}}, Editor: &fixedEditor{score: 8}, Policy: DefaultPolicy()}
	if _, err := coordinator.Generate(context.Background(), "p", 1, ChapterPlan{Chapter: 1}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := coordinator.Check(context.Background(), "p", 1)
	if err != nil || snapshot.Transaction.State != StateHold || len(snapshot.Candidates) != 1 || snapshot.Candidates[0].Text != "draft-0" {
		t.Fatalf("librarian failure snapshot=%+v err=%v", snapshot, err)
	}

	store2 := openQualityTestStore(t)
	writer2 := &fakeWriter{}
	librarian2 := &fakeLibrarian{change: FactChange{Subject: "character:hero", Predicate: "inventory", Object: json.RawMessage(`[]`)}}
	coordinator2 := Coordinator{Store: store2, Writer: writer2, Librarian: librarian2, Continuity: fixedContinuity{ContinuityResult{Status: ContinuityFail, Blocking: true, Issues: []ContinuityIssue{{IssueCode: "CONTINUITY_INVENTORY_CONFLICT", Predicate: "inventory"}}}}, Editor: &fixedEditor{score: 10}, Policy: DefaultPolicy()}
	_, _ = coordinator2.Generate(context.Background(), "p", 2, ChapterPlan{Chapter: 2})
	snapshot, err = coordinator2.Check(context.Background(), "p", 2)
	if err != nil || snapshot.Transaction.State != StateRewritePending || snapshot.Editor != nil {
		t.Fatalf("FAIL must block editor/finalization state=%s editor=%v err=%v", snapshot.Transaction.State, snapshot.Editor, err)
	}
	if _, err := coordinator2.Finalize(context.Background(), "p", 2, "finalize"); err == nil {
		t.Fatal("continuity FAIL unexpectedly finalized")
	}
}

func TestCoordinatorBoundedRewriteAndBestCandidateSelection(t *testing.T) {
	store := openQualityTestStore(t)
	writer := &fakeWriter{}
	librarian := &fakeLibrarian{change: FactChange{Subject: "character:hero", Predicate: "mood", Object: json.RawMessage(`"focused"`)}}
	editor := &fixedEditor{score: 6}
	coordinator := Coordinator{Store: store, Writer: writer, Librarian: librarian, Continuity: fixedContinuity{ContinuityResult{Status: ContinuityPass}}, Editor: editor, Policy: DefaultPolicy()}
	ctx := context.Background()
	_, _ = coordinator.Generate(ctx, "p", 3, ChapterPlan{Chapter: 3})
	first, err := coordinator.Check(ctx, "p", 3)
	if err != nil || first.Transaction.State != StateRewritePending {
		t.Fatalf("first state=%s err=%v", first.Transaction.State, err)
	}
	_, _ = coordinator.Rewrite(ctx, "p", 3, ChapterPlan{Chapter: 3})
	_, _ = coordinator.Check(ctx, "p", 3)
	_, _ = coordinator.Rewrite(ctx, "p", 3, ChapterPlan{Chapter: 3})
	final, err := coordinator.Check(ctx, "p", 3)
	if err != nil || final.Transaction.Attempt != 2 || final.Transaction.State != StateFinalCandidate || final.Transaction.FinalCandidateID == "" {
		t.Fatalf("rewrite cap snapshot=%+v err=%v", final.Transaction, err)
	}
	callsBefore := writer.calls
	again, err := coordinator.Rewrite(ctx, "p", 3, ChapterPlan{Chapter: 3})
	if err != nil || again.Transaction.Attempt != 2 || writer.calls != callsBefore {
		t.Fatalf("rewrite exceeded cap attempts=%d calls=%d->%d err=%v", again.Transaction.Attempt, callsBefore, writer.calls, err)
	}
	selected := 0
	for _, c := range final.Candidates {
		if c.Selected {
			selected++
			if c.SelectionReason == "" {
				t.Fatal("selection reason missing")
			}
		}
	}
	if selected != 1 {
		t.Fatalf("selected=%d", selected)
	}
}

type recordingLedger struct {
	mu       sync.Mutex
	attempts int
	commits  int
	keys     map[string]narrativeledger.CommitResult
}

func (l *recordingLedger) CommitAcceptedFinal(_ context.Context, input narrativeledger.AcceptedFinalInput) (narrativeledger.CommitResult, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.attempts++
	if l.keys == nil {
		l.keys = make(map[string]narrativeledger.CommitResult)
	}
	if result, ok := l.keys[input.IdempotencyKey]; ok {
		result.Replayed = true
		return result, nil
	}
	l.commits++
	result := narrativeledger.CommitResult{CommitID: "ledger-commit", TransactionID: input.TransactionID}
	l.keys[input.IdempotencyKey] = result
	return result, nil
}

func TestCoordinatorFinalizeIsRecoverableIdempotentAndConcurrentSafe(t *testing.T) {
	store := openQualityTestStore(t)
	truth := openTruthTestStore(t)
	writer := &fakeWriter{}
	librarian := &fakeLibrarian{change: FactChange{Subject: "character:hero", Predicate: "mood", Object: json.RawMessage(`"focused"`)}}
	editor := &fixedEditor{score: 9}
	finalWriter := &memoryFinalWriter{}
	ledger := &recordingLedger{}
	fault := &oneShotFault{point: "after_truth_commit"}
	coordinator := Coordinator{Store: store, Truth: truth, Ledger: ledger, Writer: writer, Librarian: librarian, Continuity: fixedContinuity{ContinuityResult{Status: ContinuityPass}}, Editor: editor, Policy: DefaultPolicy(), FinalWriter: finalWriter, Faults: fault}
	ctx := context.Background()
	_, _ = coordinator.Generate(ctx, "p", 4, ChapterPlan{Chapter: 4})
	checked, err := coordinator.Check(ctx, "p", 4)
	if err != nil || checked.Transaction.State != StateFinalCandidate || checked.Editor == nil || checked.Editor.Score != 9 {
		t.Fatalf("check snapshot=%+v err=%v", checked.Transaction, err)
	}
	if _, err := coordinator.Finalize(ctx, "p", 4, "finish-4"); err == nil {
		t.Fatal("expected injected failure")
	}
	failed, _ := store.Snapshot(ctx, "p", 4)
	if failed.Transaction.State != StateTruthCommitPending || failed.Transaction.FinalCandidateID == "" {
		t.Fatalf("truth failure did not preserve final candidate: %+v", failed.Transaction)
	}
	complete, err := coordinator.Finalize(ctx, "p", 4, "finish-4")
	if err != nil || complete.Transaction.State != StateCompleted {
		t.Fatalf("resume finalize state=%s err=%v", complete.Transaction.State, err)
	}
	page, err := truth.Events(ctx, truthstore.EventQuery{Limit: 100})
	if err != nil || len(page.Events) != 1 {
		t.Fatalf("truth events=%d err=%v", len(page.Events), err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := coordinator.Finalize(ctx, "p", 4, "finish-4")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	page, _ = truth.Events(ctx, truthstore.EventQuery{Limit: 100})
	if len(page.Events) != 1 {
		t.Fatalf("concurrent finalize duplicated truth: %d events", len(page.Events))
	}
	if writer.calls != 1 || librarian.calls != 1 || editor.calls != 1 {
		t.Fatalf("finalize repeated model stages writer=%d librarian=%d editor=%d", writer.calls, librarian.calls, editor.calls)
	}
	if ledger.commits != 1 || ledger.attempts < 2 {
		t.Fatalf("Finalize did not use replay-safe ledger boundary: commits=%d attempts=%d", ledger.commits, ledger.attempts)
	}
}

func TestTruthContinuityUsesChapterNInventoryAndKnowledge(t *testing.T) {
	truth := openTruthTestStore(t)
	ctx := context.Background()
	appendFact := func(key, predicate, value string, valid, known int) {
		t.Helper()
		_, err := truth.Append(ctx, truthstore.AppendInput{
			IdempotencyKey: key, Kind: truthstore.EventAssert, SubjectType: "character", SubjectID: "hero", Predicate: predicate,
			Value: json.RawMessage(value), ValidFromChapter: valid, KnownFromChapter: known, Authority: truthstore.AuthorityGeneratedFinal, Confidence: 1,
			Source: truthstore.Source{Type: "chapter_final", ID: key, Chapter: valid, Version: key},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	appendFact("inv-100", "inventory", `["key"]`, 100, 100)
	appendFact("inv-180", "inventory", `["key","x"]`, 180, 180)
	appendFact("know-100", "knowledge", `"secret"`, 100, 130)
	service := TruthContinuityService{Truth: truth}
	candidate := Candidate{ID: "c", Chapter: 120, SourceVersion: "v120", TextSHA: "sha"}
	inventoryProposal := proposalFor(candidate, "p", FactChange{Subject: "character:hero", Predicate: "inventory", Object: json.RawMessage(`["x"]`)})
	result, err := service.Check(ctx, ContinuityRequest{ProjectID: "p", Chapter: 120, Candidate: candidate, Proposal: inventoryProposal})
	if err != nil || result.Status != ContinuityFail || !result.Blocking {
		t.Fatalf("Chapter-120 inventory check=%+v err=%v", result, err)
	}
	knowledgeProposal := proposalFor(candidate, "p", FactChange{Subject: "character:hero", Predicate: "knowledge", Object: json.RawMessage(`"other"`)})
	result, err = service.Check(ctx, ContinuityRequest{ProjectID: "p", Chapter: 120, Candidate: candidate, Proposal: knowledgeProposal})
	if err != nil || result.Status != ContinuityPass {
		t.Fatalf("future knowledge leaked into Chapter-120: %+v err=%v", result, err)
	}
	candidate.Chapter = 140
	knowledgeProposal = proposalFor(candidate, "p", FactChange{Subject: "character:hero", Predicate: "knowledge", Object: json.RawMessage(`"other"`)})
	result, err = service.Check(ctx, ContinuityRequest{ProjectID: "p", Chapter: 140, Candidate: candidate, Proposal: knowledgeProposal})
	if err != nil || result.Status != ContinuityFail {
		t.Fatalf("known Chapter-140 conflict not detected: %+v err=%v", result, err)
	}
}

func TestNoSafeCandidateEntersHoldAndDraftFaultIsRecoverable(t *testing.T) {
	store := openQualityTestStore(t)
	writer := &fakeWriter{}
	fault := &oneShotFault{point: "after_draft_persist"}
	coordinator := Coordinator{Store: store, Writer: writer, Policy: Policy{MaxRewrites: 0, QualityThreshold: 7, AllowWarn: true}, Faults: fault}
	snapshot, err := coordinator.Generate(context.Background(), "p", 5, ChapterPlan{Chapter: 5})
	if err != nil || snapshot.Transaction.State != StateHold || len(snapshot.Candidates) != 1 {
		t.Fatalf("draft was not retained after post-persist crash: state=%s candidates=%d err=%v", snapshot.Transaction.State, len(snapshot.Candidates), err)
	}
	_, err = coordinator.Generate(context.Background(), "p", 5, ChapterPlan{Chapter: 5})
	if err != nil || writer.calls != 1 {
		t.Fatalf("durable draft triggered duplicate writer call: calls=%d err=%v", writer.calls, err)
	}

	librarian := &fakeLibrarian{change: FactChange{Subject: "character:hero", Predicate: "location", Object: json.RawMessage(`"tower"`)}}
	coordinator.Librarian = librarian
	coordinator.Continuity = fixedContinuity{ContinuityResult{Status: ContinuityFail, Blocking: true, Issues: []ContinuityIssue{{IssueCode: "CONTINUITY_LOCATION_CONFLICT", Predicate: "location"}}}}
	coordinator.Editor = &fixedEditor{score: 10}
	snapshot, err = coordinator.Check(context.Background(), "p", 5)
	if err != nil || snapshot.Transaction.State != StateHold || snapshot.Transaction.HoldReason == "" {
		t.Fatalf("no-safe-candidate state=%s hold=%q err=%v", snapshot.Transaction.State, snapshot.Transaction.HoldReason, err)
	}
}
