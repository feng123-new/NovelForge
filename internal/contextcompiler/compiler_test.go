package contextcompiler

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fixedCounter struct{}

func (fixedCounter) Count(value string) int { return len(strings.Fields(value)) }

func provider(items ...Item) ItemProvider {
	return ProviderFunc(func(context.Context, Request) ([]Item, error) {
		clone := append([]Item(nil), items...)
		return clone, nil
	})
}

func TestDefaultBudgetAllocationAndCriticalRetention(t *testing.T) {
	required := []struct {
		id    string
		layer Layer
		req   Requirement
	}{
		{"plan", LayerNarrative, RequirementCurrentChapterPlan},
		{"pov", LayerTruth, RequirementPOVCharacterState},
		{"rule", LayerTruth, RequirementCriticalWorldRule},
		{"foreshadow", LayerTruth, RequirementCriticalForeshadow},
		{"boundary", LayerTruth, RequirementKnowledgeBoundary},
		{"beat", LayerNarrative, RequirementRequiredContractBeat},
	}
	truth := []Item{}
	narrative := []Item{}
	for _, entry := range required {
		item := Item{ID: entry.id, Layer: entry.layer, Kind: "required", Content: "one two", Mandatory: true, Requirement: entry.req, Priority: 1000, SourceChapter: 50}
		if entry.layer == LayerTruth {
			truth = append(truth, item)
		} else {
			narrative = append(narrative, item)
		}
	}
	truth = append(truth, Item{ID: "optional-truth", Kind: "fact", Content: strings.Repeat("word ", 30), SourceChapter: 50})
	result, err := New(Providers{Truth: provider(truth...), Narrative: provider(narrative...)}, fixedCounter{}).Compile(context.Background(), Request{
		ProjectID: "p1", Chapter: 50, TotalTokens: 100,
		RequiredRequirements: []Requirement{
			RequirementCurrentChapterPlan, RequirementPOVCharacterState,
			RequirementCriticalWorldRule, RequirementCriticalForeshadow,
			RequirementKnowledgeBoundary, RequirementRequiredContractBeat,
		},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if result.Diagnostics.SystemTokens != 10 || result.Diagnostics.ContentTokens != 90 {
		t.Fatalf("unexpected system/content allocation: %+v", result.Diagnostics)
	}
	wantAlloc := map[Layer]int{LayerTruth: 20, LayerNarrative: 15, LayerRecent: 25, LayerHistorical: 20, LayerStyle: 10}
	for layer, want := range wantAlloc {
		if got := result.Diagnostics.Layers[layer].AllocatedTokens; got != want {
			t.Errorf("%s allocation=%d want=%d", layer, got, want)
		}
	}
	selected := map[string]bool{}
	for _, item := range result.Items {
		selected[item.ID] = true
	}
	for _, entry := range required {
		if !selected[entry.id] {
			t.Errorf("required item %q was trimmed", entry.id)
		}
	}
	if result.ContextSHA == "" || result.Text == "" {
		t.Fatal("missing deterministic text/hash")
	}
}

func TestOverflowFailsClosedBeforeDroppingMandatory(t *testing.T) {
	_, err := New(Providers{Truth: provider(Item{ID: "pov", Kind: "state", Content: strings.Repeat("x ", 95), Mandatory: true, Requirement: RequirementPOVCharacterState})}, fixedCounter{}).Compile(context.Background(), Request{Chapter: 10, TotalTokens: 100})
	if !errors.Is(err, ErrMandatoryOverflow) {
		t.Fatalf("error=%v, want ErrMandatoryOverflow", err)
	}
}

func TestNoFutureStateLeakage(t *testing.T) {
	result, err := New(Providers{Truth: provider(
		Item{ID: "past", Kind: "state", Content: "past state", SourceChapter: 49},
		Item{ID: "future", Kind: "state", Content: "future state", SourceChapter: 51},
	)}, fixedCounter{}).Compile(context.Background(), Request{Chapter: 50, TotalTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != "past" {
		t.Fatalf("future leak: %+v", result.Items)
	}
	if result.Diagnostics.FutureItems != 1 {
		t.Fatalf("future diagnostics=%d", result.Diagnostics.FutureItems)
	}
	if !containsTrim(result.Diagnostics.Layers[LayerTruth].Trimmed, "future", TrimFutureState) {
		t.Fatal("future trim reason not recorded")
	}
}

func TestFutureMandatoryFailsClosed(t *testing.T) {
	_, err := New(Providers{Truth: provider(Item{ID: "future-pov", Kind: "state", Content: "future", SourceChapter: 51, Mandatory: true, Requirement: RequirementPOVCharacterState})}, fixedCounter{}).Compile(context.Background(), Request{Chapter: 50, TotalTokens: 100})
	if !errors.Is(err, ErrFutureMandatory) {
		t.Fatalf("error=%v", err)
	}
}

func TestDeterministicOrderingHashAndHistoricalPipeline(t *testing.T) {
	calls := []RetrievalStage{}
	stageProvider := func(stage RetrievalStage, id string) ItemProvider {
		return ProviderFunc(func(context.Context, Request) ([]Item, error) {
			calls = append(calls, stage)
			return []Item{{ID: id, Kind: string(stage), Content: "item", Priority: 10}}, nil
		})
	}
	providers := Providers{
		Structured: stageProvider(StageStructured, "z"), Timeline: stageProvider(StageTimeline, "y"),
		Foreshadow: stageProvider(StageForeshadow, "x"), Relation: stageProvider(StageRelation, "w"),
		HistoricalRecent: stageProvider(StageRecent, "v"), FTS5: stageProvider(StageFTS5, "u"),
		Vector: vectorFunc(func(context.Context, Request) ([]Item, error) {
			calls = append(calls, StageVector)
			return []Item{{ID: "t", Kind: "vector", Content: "item", Priority: 10}}, nil
		}),
	}
	compiler := New(providers, fixedCounter{})
	request := Request{Chapter: 10, TotalTokens: 100}
	first, err := compiler.Compile(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := []RetrievalStage{StageStructured, StageTimeline, StageForeshadow, StageRelation, StageRecent, StageFTS5, StageVector}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls=%v want=%v", calls, wantCalls)
	}
	calls = nil
	second, err := compiler.Compile(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ContextSHA != second.ContextSHA || first.Text != second.Text {
		t.Fatal("same inputs produced unstable output")
	}
	for i, stage := range wantCalls {
		if first.Items[i].Stage != stage {
			t.Fatalf("item %d stage=%s want=%s", i, first.Items[i].Stage, stage)
		}
	}
}

type vectorFunc func(context.Context, Request) ([]Item, error)

func (f vectorFunc) Retrieve(ctx context.Context, r Request) ([]Item, error) { return f(ctx, r) }

func TestMissingRequirementFailsClosed(t *testing.T) {
	_, err := New(Providers{}, fixedCounter{}).Compile(context.Background(), Request{Chapter: 1, TotalTokens: 100, RequiredRequirements: []Requirement{RequirementCurrentChapterPlan}})
	if !errors.Is(err, ErrMissingRequirement) {
		t.Fatalf("error=%v", err)
	}
}

func TestLayerTrimReasonsAndUnusedBudgetRedistribution(t *testing.T) {
	items := []Item{
		{ID: "a", Kind: "fact", Content: strings.Repeat("a ", 18), Priority: 10},
		{ID: "b", Kind: "fact", Content: strings.Repeat("b ", 18), Priority: 9},
		{ID: "c", Kind: "fact", Content: strings.Repeat("c ", 18), Priority: 8},
	}
	result, err := New(Providers{Truth: provider(items...)}, fixedCounter{}).Compile(context.Background(), Request{Chapter: 1, TotalTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	// Truth starts with 20 tokens, but all other layers are empty, so unused
	// capacity is deterministically borrowed and all three records fit.
	if len(result.Items) != 3 {
		t.Fatalf("selected=%d want=3", len(result.Items))
	}
	if result.Diagnostics.Layers[LayerTruth].UsedTokens != 54 {
		t.Fatalf("truth used=%d", result.Diagnostics.Layers[LayerTruth].UsedTokens)
	}
	if result.Diagnostics.RemainingTokens != 36 {
		t.Fatalf("remaining=%d", result.Diagnostics.RemainingTokens)
	}
}

func TestCompileLegacyMapPreservesLayerSemantics(t *testing.T) {
	raw := map[string]any{
		"current_chapter_plan": map[string]any{"goal": "escape"},
		"pov_character_state":  map[string]any{"alive": true},
		"recent_summaries":     []string{"chapter 8", "chapter 9"},
		"selected_memory":      []string{"old gate"},
		"style_reference":      "terse",
	}
	result, err := CompileLegacyMap(context.Background(), raw, Request{ProjectID: "p", Chapter: 10, TotalTokens: 200, RequiredRequirements: []Requirement{RequirementCurrentChapterPlan, RequirementPOVCharacterState}}, fixedCounter{})
	if err != nil {
		t.Fatal(err)
	}
	layers := map[Layer]bool{}
	for _, item := range result.Items {
		layers[item.Layer] = true
	}
	for _, layer := range []Layer{LayerTruth, LayerNarrative, LayerRecent, LayerHistorical, LayerStyle} {
		if !layers[layer] {
			t.Errorf("missing %s layer", layer)
		}
	}
}

func containsTrim(items []TrimmedItem, id string, reason TrimReason) bool {
	for _, item := range items {
		if item.ID == id && item.Reason == reason {
			return true
		}
	}
	return false
}

func BenchmarkCompilerFiveLayers(b *testing.B) {
	makeItems := func(layer Layer, n int) []Item {
		out := make([]Item, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, Item{ID: string(layer) + string(rune(i+32)), Layer: layer, Kind: "fixture", Content: "bounded deterministic fixture content", SourceChapter: 100 - i%20, Priority: i % 7})
		}
		return out
	}
	compiler := New(Providers{Truth: provider(makeItems(LayerTruth, 200)...), Narrative: provider(makeItems(LayerNarrative, 80)...), Recent: provider(makeItems(LayerRecent, 50)...), Structured: provider(makeItems(LayerHistorical, 200)...), Style: provider(makeItems(LayerStyle, 80)...)}, HeuristicTokenCounter{})
	request := Request{ProjectID: "bench", Chapter: 100, TotalTokens: 8192}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := compiler.Compile(context.Background(), request); err != nil {
			b.Fatal(err)
		}
	}
}
