package contextcompiler

import (
	"context"
	"strings"
	"testing"
)

type ledgerStub struct{ snapshot LedgerContext }

func (s ledgerStub) Context(context.Context, LedgerRequest) (LedgerContext, error) {
	return s.snapshot, nil
}

func TestLedgerProviderPinsOverdueAndNeverCarriesUnknownTruth(t *testing.T) {
	provider := LedgerProvider{Reader: ledgerStub{snapshot: LedgerContext{
		MandatoryForeshadows: []LedgerForeshadow{{ID: "gate", Title: "Sealed gate", Summary: "must reopen", SourceChapter: 20, SourceVersion: "v1", Critical: true, Overdue: true, OverdueBy: 5}},
		KnownSecrets:         []LedgerSecret{{ID: "known", Summary: "POV knows the key", SourceChapter: 30, SourceVersion: "v1"}},
		UnknownBoundaries:    []LedgerBoundary{{SecretID: "origin", Description: "POV lacks this knowledge", SourceChapter: 1, SourceVersion: "v1"}},
	}}}
	items, err := provider.Collect(context.Background(), Request{ProjectID: "p", Chapter: 135, POVEntityID: "hero"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("items=%d", len(items))
	}
	if !items[0].Mandatory || items[0].Requirement != RequirementCriticalForeshadow || !strings.Contains(items[0].Content, "OVERDUE +5") {
		t.Fatalf("foreshadow=%+v", items[0])
	}
	if !items[2].Mandatory || items[2].Requirement != RequirementKnowledgeBoundary {
		t.Fatalf("boundary=%+v", items[2])
	}
	for _, item := range items {
		if strings.Contains(item.Content, "old capital") {
			t.Fatalf("unknown truth leaked: %+v", item)
		}
	}
}
