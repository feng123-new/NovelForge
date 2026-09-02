package qualitygate

import (
	"context"
	"reflect"
	"strings"
)

type narrativeLedgerContextKey struct{}

// WithNarrativeLedgerContext carries the deterministic Phase 6 planner injection
// without widening model credentials or granting an Agent write access.
func WithNarrativeLedgerContext(ctx context.Context, value string) context.Context {
	value = strings.TrimSpace(value)
	if value == "" {
		return ctx
	}
	return context.WithValue(ctx, narrativeLedgerContextKey{}, value)
}

// NarrativeLedgerContext returns the immutable request-scoped ledger text.
func NarrativeLedgerContext(ctx context.Context) (string, bool) {
	value, ok := ctx.Value(narrativeLedgerContextKey{}).(string)
	value = strings.TrimSpace(value)
	return value, ok && value != ""
}

// injectNarrativeLedgerContext appends the request-scoped ledger block to the
// Planner input. Reflection keeps the Phase 6 adapter compatible with the
// existing stable Agent request structs while Phase 7 owns a typed compiler.
func injectNarrativeLedgerContext(ctx context.Context, target any) bool {
	ledger, ok := NarrativeLedgerContext(ctx)
	if !ok || target == nil {
		return false
	}
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return false
	}
	value = value.Elem()
	if value.Kind() == reflect.Map && value.Type().Key().Kind() == reflect.String {
		if value.IsNil() {
			value.Set(reflect.MakeMap(value.Type()))
		}
		entry := reflect.ValueOf(ledger)
		if entry.Type().AssignableTo(value.Type().Elem()) {
			value.SetMapIndex(reflect.ValueOf("narrative_ledger"), entry)
			return true
		}
	}
	if value.Kind() != reflect.Struct {
		return false
	}
	preferred := []string{"PlannerContext", "Context", "Instructions", "Prompt", "Constraints"}
	for _, name := range preferred {
		field := value.FieldByName(name)
		if !field.IsValid() || !field.CanSet() || field.Kind() != reflect.String {
			continue
		}
		current := strings.TrimSpace(field.String())
		if current == "" {
			field.SetString(ledger)
		} else if !strings.Contains(current, "[NARRATIVE_LEDGER]") {
			field.SetString(current + "\n\n" + ledger)
		}
		return true
	}
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		structField := value.Type().Field(index)
		if !field.CanSet() || field.Kind() != reflect.String || structField.PkgPath != "" {
			continue
		}
		current := strings.TrimSpace(field.String())
		if current == "" {
			field.SetString(ledger)
		} else if !strings.Contains(current, "[NARRATIVE_LEDGER]") {
			field.SetString(current + "\n\n" + ledger)
		}
		return true
	}
	return false
}
