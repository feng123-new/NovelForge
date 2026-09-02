package qualitygate

import (
	"context"
	"strings"
	"testing"
)

func TestNarrativeLedgerContextInjection(t *testing.T) {
	type plannerInput struct {
		Context string
	}
	ctx := WithNarrativeLedgerContext(context.Background(), "[NARRATIVE_LEDGER]\n- OVERDUE MANDATORY key=seal")
	input := plannerInput{Context: "chapter foundation"}
	if !injectNarrativeLedgerContext(ctx, &input) {
		t.Fatal("planner context was not injected")
	}
	if !strings.Contains(input.Context, "chapter foundation") || !strings.Contains(input.Context, "OVERDUE MANDATORY") {
		t.Fatalf("planner context was overwritten or omitted: %q", input.Context)
	}
	if !injectNarrativeLedgerContext(ctx, &input) {
		t.Fatal("idempotent injection unexpectedly failed")
	}
	if strings.Count(input.Context, "[NARRATIVE_LEDGER]") != 1 {
		t.Fatalf("planner context was duplicated: %q", input.Context)
	}
}
