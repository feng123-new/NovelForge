package contextcompiler

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLegacySelectionReturnsOnlyCompiledRecords(t *testing.T) {
	raw := map[string]any{"working_memory": map[string]any{"chapter_plan": "must remain", "style_reference": strings.Repeat("optional ", 5000)}}
	compiled, err := CompileLegacyMap(context.Background(), raw, Request{Chapter: 1, TotalTokens: 500}, nil)
	if err != nil { t.Fatal(err) }
	selected, err := SelectLegacyMap(raw, compiled)
	if err != nil { t.Fatal(err) }
	memory := selected["working_memory"].(map[string]any)
	if memory["chapter_plan"] != "must remain" { t.Fatal("mandatory plan dropped") }
	if _, exists := memory["style_reference"]; exists { t.Fatal("unselected raw field escaped budget") }
	if _, exists := raw["working_memory"].(map[string]any)["style_reference"]; !exists { t.Fatal("input mutated") }
}

func TestLegacyMandatoryOverflowStillFails(t *testing.T) {
	_, err := CompileLegacyMap(context.Background(), map[string]any{"chapter_plan": strings.Repeat("must ", 10000)}, Request{Chapter: 1, TotalTokens: 100}, nil)
	if !errors.Is(err, ErrMandatoryOverflow) { t.Fatalf("expected mandatory overflow, got %v", err) }
}
