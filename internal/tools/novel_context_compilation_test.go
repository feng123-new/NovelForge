package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContextPayloadUsesCompilerSelection(t *testing.T) {
	raw := map[string]any{
		"working_memory": map[string]any{"chapter_plan": "mandatory plan"},
		"reference_pack": map[string]any{"references": strings.Repeat("optional ", 5000)},
	}
	data, err := finalizeContextPayload(raw, 2, 8000)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "optional optional") {
		t.Fatal("uncompiled optional content leaked into tool result")
	}
	if result["_context_compiler"].(map[string]any)["status"] != "applied" {
		t.Fatal("compiler must govern actual returned payload")
	}
	if result["working_memory"].(map[string]any)["chapter_plan"] != "mandatory plan" {
		t.Fatal("mandatory plan disappeared")
	}
}
