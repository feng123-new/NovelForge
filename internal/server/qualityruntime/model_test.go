package qualityruntime

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/qualitygate"
)

func TestQualityModelMissingAndInvalidConfiguration(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	if model, err := LoadQualityModel(home, root, ""); err != nil || model != nil {
		t.Fatalf("missing config should leave management available: model=%v err=%v", model, err)
	}
	path := filepath.Join(root, "explicit.json")
	if err := os.WriteFile(path, []byte(`{"provider":"secret-marker",broken-json`), 0600); err != nil {
		t.Fatal(err)
	}
	if model, err := LoadQualityModel(home, root, path); err == nil || model != nil || strings.Contains(err.Error(), "secret-marker") || strings.Contains(err.Error(), path) {
		t.Fatalf("invalid explicit config must fail safely: model=%v err=%v", model, err)
	}
}

func TestQualityContractsUseProductionFields(t *testing.T) {
	if role, _, _, err := qualityContract("librarian:fact_proposal"); err != nil || role != "librarian" {
		t.Fatal("librarian contract is missing")
	}
	schema := qualitySchema(reflect.TypeOf(qualitygate.FactProposal{}))
	properties := schema["properties"].(map[string]any)
	for _, name := range []string{"project_id", "source_version", "source_sha", "foreshadow_updates", "secrets"} {
		if _, ok := properties[name]; !ok {
			t.Fatalf("missing production field %s", name)
		}
	}
	var model *QualityModel
	if _, _, err := model.Invoke(context.Background(), "writer:draft", nil); err == nil {
		t.Fatal("unconfigured model did not fail")
	}
	if _, _, _, err := qualityContract("writer:arbitrary"); err == nil {
		t.Fatal("unsupported operation did not fail")
	}
}
