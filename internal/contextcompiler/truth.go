package contextcompiler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/voocel/ainovel-cli/internal/truthstore"
)

// TruthStateReader is the minimal read-only authority boundary used by the
// compiler. It intentionally omits every Truth write method.
type TruthStateReader interface {
	State(context.Context, truthstore.StateQuery) (truthstore.StatePage, error)
}

type TruthProvider struct {
	Store TruthStateReader
	Limit int
}

func (p TruthProvider) Collect(ctx context.Context, request Request) ([]Item, error) {
	if p.Store == nil {
		return nil, nil
	}
	limit := p.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	page, err := p.Store.State(ctx, truthstore.StateQuery{Chapter: request.Chapter, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("query Chapter-N Truth: %w", err)
	}
	items := make([]Item, 0, len(page.Facts)+1)
	for _, fact := range page.Facts {
		kind, priority := truthKind(fact)
		content := formatTruthFact(fact)
		item := Item{
			ID: "truth:" + fact.ID, Layer: LayerTruth, Kind: kind,
			Title:   fact.SubjectType + ":" + fact.SubjectID + " " + fact.Predicate,
			Content: content, SourceChapter: fact.Source.Chapter,
			SourceVersion: fact.Source.Version, Priority: priority,
			Metadata: map[string]string{"authority": string(fact.Authority)},
		}
		predicate := strings.ToLower(fact.Predicate)
		if request.POVEntityID != "" && fact.SubjectID == request.POVEntityID && isCharacterState(predicate) {
			item.Mandatory = true
			item.Requirement = RequirementPOVCharacterState
			item.Priority = 1000
		}
		if kind == "world_rule" && isCriticalFact(fact) {
			item.Mandatory = true
			item.Requirement = RequirementCriticalWorldRule
			item.Priority = 1000
		}
		items = append(items, item)
	}
	if page.Conflicts > 0 {
		data, _ := json.Marshal(map[string]int{"unresolved_conflicts": page.Conflicts})
		items = append(items, Item{ID: "truth:conflicts", Layer: LayerTruth, Kind: "conflict_boundary", Title: "Truth conflicts", Content: string(data), SourceChapter: request.Chapter, Priority: 900, Mandatory: true})
	}
	return items, nil
}

func truthKind(fact truthstore.Fact) (string, int) {
	predicate := strings.ToLower(fact.Predicate)
	subject := strings.ToLower(fact.SubjectType)
	switch {
	case strings.Contains(subject, "rule") || strings.Contains(predicate, "world_rule") || strings.HasPrefix(predicate, "rule."):
		return "world_rule", 90
	case strings.Contains(predicate, "knowledge") || strings.Contains(predicate, "knows"):
		return "knowledge", 85
	case strings.Contains(predicate, "inventory") || strings.Contains(predicate, "item") || strings.Contains(predicate, "possess"):
		return "inventory", 80
	case strings.Contains(predicate, "relation") || strings.Contains(predicate, "relationship"):
		return "relation", 75
	case strings.Contains(predicate, "timeline") || strings.Contains(subject, "event"):
		return "timeline", 70
	case isCharacterState(predicate) || strings.Contains(subject, "character"):
		return "character_state", 88
	default:
		return "fact", 60
	}
}

func isCharacterState(predicate string) bool {
	for _, part := range []string{"state", "alive", "dead", "location", "injury", "cultivation", "status", "goal", "emotion"} {
		if strings.Contains(predicate, part) {
			return true
		}
	}
	return false
}

func isCriticalFact(fact truthstore.Fact) bool {
	p := strings.ToLower(fact.Predicate)
	if strings.Contains(p, "critical") || strings.Contains(p, "hard_rule") {
		return true
	}
	return fact.Authority == truthstore.AuthorityHumanFinal || fact.Authority == truthstore.AuthorityGeneratedFinal
}

func formatTruthFact(fact truthstore.Fact) string {
	value := strings.TrimSpace(string(fact.Value))
	if value == "" {
		value = "null"
	}
	return fmt.Sprintf("%s %s = %s", fact.SubjectID, fact.Predicate, value)
}
