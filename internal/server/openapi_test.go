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
