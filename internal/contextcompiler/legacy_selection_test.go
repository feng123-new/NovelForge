package contextcompiler

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLegacySelectionChangesPayloadWithoutMutatingInput(t *testing.T) {
	raw := map[string]any{
		"working_memory": map[string]any{"chapter_plan": "keep the plan", "user_rules": "keep rules"},
		"reference_pack": map[string]any{"references": strings.Repeat("optional ", 3000)},
	}
	selected, compiled, err := SelectLegacyMap(context.Background(), raw, Request{Chapter: 5, TotalTokens: 500}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if selected["working_memory"].(map[string]any)["chapter_plan"] != "keep the plan" || compiled.ContextSHA == "" {
		t.Fatal("mandatory plan or context identity missing")
	}
	if _, ok := selected["reference_pack"].(map[string]any)["references"]; ok {
		t.Fatal("trimmed optional context still reaches the model")
	}
	if _, ok := raw["reference_pack"].(map[string]any)["references"]; !ok {
		t.Fatal("input was mutated")
	}
}

func TestLegacySelectionFailsClosed(t *testing.T) {
	raw := map[string]any{"chapter_plan": strings.Repeat("mandatory ", 3000)}
	if out, _, err := SelectLegacyMap(context.Background(), raw, Request{Chapter: 5, TotalTokens: 50}, nil); !errors.Is(err, ErrMandatoryOverflow) || out != nil {
		t.Fatalf("mandatory overflow returned usable context: out=%v err=%v", out, err)
	}
	if out, _, err := SelectLegacyMap(context.Background(), map[string]any{"chapter_plan": make(chan int)}, Request{TotalTokens: 100}, nil); err == nil || out != nil {
		t.Fatal("non-serializable plan was silently dropped")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := SelectLegacyMap(ctx, raw, Request{TotalTokens: 100}, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation was ignored")
	}
}
