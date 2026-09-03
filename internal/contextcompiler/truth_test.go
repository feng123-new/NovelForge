package contextcompiler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/voocel/ainovel-cli/internal/truthstore"
)

type truthReaderStub struct{ page truthstore.StatePage }

func (s truthReaderStub) State(context.Context, truthstore.StateQuery) (truthstore.StatePage, error) {
	return s.page, nil
}

func TestTruthProviderPinsPOVRuleAndKnowledgeBoundary(t *testing.T) {
	facts := []truthstore.Fact{
		{Event: truthstore.Event{ID: "pov", SubjectType: "character", SubjectID: "hero", Predicate: "state.location", Value: json.RawMessage(`"gate"`), Authority: truthstore.AuthorityGeneratedFinal, Source: truthstore.Source{Chapter: 40, Version: "v1"}}},
		{Event: truthstore.Event{ID: "rule", SubjectType: "world_rule", SubjectID: "magic", Predicate: "hard_rule.cost", Value: json.RawMessage(`"memory"`), Authority: truthstore.AuthorityHumanFinal, Source: truthstore.Source{Chapter: 1, Version: "v1"}}},
		{Event: truthstore.Event{ID: "knowledge", SubjectType: "character", SubjectID: "hero", Predicate: "knowledge.secret-x", Value: json.RawMessage(`false`), Authority: truthstore.AuthorityGeneratedFinal, Source: truthstore.Source{Chapter: 30, Version: "v1"}}},
	}
	items, err := TruthProvider{Store: truthReaderStub{page: truthstore.StatePage{Facts: facts}}}.Collect(context.Background(), Request{Chapter: 50, POVEntityID: "hero"})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Item{}
	for _, item := range items {
		byID[item.ID] = item
	}
	if got := byID["truth:pov"]; !got.Mandatory || got.Requirement != RequirementPOVCharacterState {
		t.Fatalf("POV state not pinned: %+v", got)
	}
	if got := byID["truth:rule"]; !got.Mandatory || got.Requirement != RequirementCriticalWorldRule {
		t.Fatalf("rule not pinned: %+v", got)
	}
	if got := byID["truth:knowledge"]; got.Kind != "knowledge" {
		t.Fatalf("knowledge classification: %+v", got)
	}
}

func TestKnowledgeBoundaryCannotBeTrimmed(t *testing.T) {
	boundary := Item{ID: "boundary", Layer: LayerTruth, Kind: "knowledge_boundary", Content: "POV does not know secret X", Mandatory: true, Requirement: RequirementKnowledgeBoundary, SourceChapter: 49, Tokens: 8}
	optional := Item{ID: "secret-truth", Layer: LayerTruth, Kind: "secret", Content: "secret X truth must not leak", SourceChapter: 51, Tokens: 8}
	result, err := New(Providers{Truth: provider(boundary, optional)}, fixedCounter{}).Compile(context.Background(), Request{Chapter: 50, TotalTokens: 20, RequiredRequirements: []Requirement{RequirementKnowledgeBoundary}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != "boundary" {
		t.Fatalf("knowledge boundary lost/leaked: %+v", result.Items)
	}
}
