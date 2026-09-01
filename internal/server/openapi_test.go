package server

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenAPICoversImplementedRoutesAndUniqueOperations(t *testing.T) {
	t.Parallel()
	type parameter struct {
		Ref  string `json:"$ref"`
		Name string `json:"name"`
		In   string `json:"in"`
	}
	var document struct {
		OpenAPI string `json:"openapi"`
		Paths   map[string]map[string]struct {
			OperationID string      `json:"operationId"`
			Parameters  []parameter `json:"parameters"`
		} `json:"paths"`
		Components struct {
			Parameters map[string]parameter `json:"parameters"`
		} `json:"components"`
	}
	if err := json.Unmarshal(openAPISpec, &document); err != nil {
		t.Fatalf("OpenAPI JSON: %v", err)
	}
	if document.OpenAPI != "3.1.0" {
		t.Fatalf("openapi = %q", document.OpenAPI)
	}
	idempotencyParameter, ok := document.Components.Parameters["IdempotencyKey"]
	if !ok || idempotencyParameter.Name != "Idempotency-Key" || idempotencyParameter.In != "header" {
		t.Fatalf("invalid reusable IdempotencyKey parameter: %#v", idempotencyParameter)
	}
	expected := map[string][]string{
		"/api/health":                   {"get"},
		"/api/openapi.json":             {"get"},
		"/api/events":                   {"get"},
		"/api/models":                   {"get"},
		"/api/settings":                 {"get"},
		"/api/projects":                 {"get", "post"},
		"/api/projects/{id}":            {"get", "patch", "delete"},
		"/api/projects/{id}/archive":    {"post"},
		"/api/projects/{id}/unarchive":  {"post"},
		"/api/projects/{id}/duplicate":  {"post"},
		"/api/projects/{id}/chapters":   {"get"},
		"/api/projects/{id}/foundation": {"get", "post"},
		"/api/truth/events":             {"get", "post"},
		"/api/truth/state":              {"get"},
		"/api/truth/state:batch":        {"post"},
		"/api/truth/conflicts":          {"get"},
		"/api/truth/rebuild":            {"post"},
		"/api/truth/verify":             {"get"},
	}
	seenOperations := make(map[string]string)
	for path, methods := range expected {
		operations, ok := document.Paths[path]
		if !ok {
			t.Errorf("missing OpenAPI path %s", path)
			continue
		}
		for _, method := range methods {
			operation, ok := operations[method]
			if !ok {
				t.Errorf("missing OpenAPI operation %s %s", strings.ToUpper(method), path)
				continue
			}
			if operation.OperationID == "" {
				t.Errorf("missing operationId for %s %s", strings.ToUpper(method), path)
				continue
			}
			if previous, exists := seenOperations[operation.OperationID]; exists {
				t.Errorf(
					"duplicate operationId %q for %s and %s %s",
					operation.OperationID,
					previous,
					strings.ToUpper(method),
					path,
				)
			}
			seenOperations[operation.OperationID] = strings.ToUpper(method) + " " + path
			if method != "get" {
				hasIdempotencyKey := false
				for _, parameter := range operation.Parameters {
					if (parameter.Name == "Idempotency-Key" && parameter.In == "header") ||
						parameter.Ref == "#/components/parameters/IdempotencyKey" {
						hasIdempotencyKey = true
					}
				}
				if !hasIdempotencyKey {
					t.Errorf("%s %s is missing Idempotency-Key", strings.ToUpper(method), path)
				}
			}
		}
	}
}

func TestOpenAPIErrorEnvelopeDoesNotExposeInternalFields(t *testing.T) {
	t.Parallel()
	var document map[string]any
	if err := json.Unmarshal(openAPISpec, &document); err != nil {
		t.Fatal(err)
	}
	serialized := string(openAPISpec)
	for _, forbidden := range []string{
		"absolute_path",
		"authorization_header",
		"api_key",
		"stack_trace",
		"raw_sql",
	} {
		if strings.Contains(strings.ToLower(serialized), forbidden) {
			t.Fatalf("OpenAPI exposes forbidden field %q", forbidden)
		}
	}
	if !strings.Contains(serialized, `"ErrorEnvelope"`) ||
		!strings.Contains(serialized, `"trace_id"`) {
		t.Fatal("OpenAPI is missing the common error envelope")
	}
}

func TestOpenAPITruthContractsMatchProductionTypes(t *testing.T) {
	t.Parallel()
	var document struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Ref  string `json:"$ref"`
				Name string `json:"name"`
			} `json:"parameters"`
			RequestBody struct {
				Content map[string]struct {
					Schema struct {
						Ref string `json:"$ref"`
					} `json:"schema"`
				} `json:"content"`
			} `json:"requestBody"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]struct {
				Required   []string `json:"required"`
				Properties map[string]struct {
					Type     any      `json:"type"`
					Enum     []string `json:"enum"`
					Ref      string   `json:"$ref"`
					MaxItems int      `json:"maxItems"`
				} `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(openAPISpec, &document); err != nil {
		t.Fatal(err)
	}

	authority := document.Components.Schemas["TruthEventInput"].Properties["authority"].Enum
	wantAuthority := []string{
		"llm_suggestion",
		"story_compass",
		"volume_plan",
		"arc_plan",
		"chapter_plan",
		"generated_final",
		"human_final",
	}
	if strings.Join(authority, ",") != strings.Join(wantAuthority, ",") {
		t.Fatalf("truth authority enum = %v, want %v", authority, wantAuthority)
	}

	source := document.Components.Schemas["TruthSource"]
	for _, field := range []string{"type", "id", "chapter", "version"} {
		if !containsString(source.Required, field) {
			t.Errorf("TruthSource is missing required field %q", field)
		}
	}
	conflict := document.Components.Schemas["TruthConflict"]
	if !containsString(conflict.Required, "status") || strings.Join(conflict.Properties["status"].Enum, ",") != "unresolved,resolved" {
		t.Fatalf("TruthConflict status contract is incomplete: %#v", conflict.Properties["status"])
	}
	batch := document.Components.Schemas["TruthBatchRequest"]
	if batch.Properties["queries"].MaxItems != 100 {
		t.Fatalf("TruthBatchRequest maxItems = %d, want 100", batch.Properties["queries"].MaxItems)
	}
	if ref := document.Paths["/api/truth/state:batch"]["post"].RequestBody.Content["application/json"].Schema.Ref; ref != "#/components/schemas/TruthBatchRequest" {
		t.Fatalf("batch request schema = %q", ref)
	}
	for _, route := range []string{"/api/truth/events", "/api/truth/state:batch", "/api/truth/rebuild"} {
		operation := document.Paths[route]["post"]
		if len(operation.Parameters) != 1 || operation.Parameters[0].Ref != "#/components/parameters/IdempotencyKey" {
			t.Fatalf("%s does not reuse IdempotencyKey: %#v", route, operation.Parameters)
		}
	}

	eventParameters := map[string]bool{}
	for _, parameter := range document.Paths["/api/truth/events"]["get"].Parameters {
		eventParameters[parameter.Name] = true
	}
	for _, name := range []string{"project_id", "after_sequence", "through_chapter", "subject_type", "subject_id", "predicate", "limit"} {
		if !eventParameters[name] {
			t.Errorf("GET /api/truth/events is missing %q", name)
		}
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
