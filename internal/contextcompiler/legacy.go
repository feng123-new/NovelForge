package contextcompiler

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// CompileLegacyMap incrementally migrates the existing novel_context payload.
// The legacy map remains authoritative for reads; this adapter classifies its
// bounded records into the five compiler layers without deleting old fields or
// old regression behavior.
func CompileLegacyMap(ctx context.Context, raw map[string]any, request Request, counter TokenCounter) (Result, error) {
	classified := classifyLegacy(raw, request.Chapter)
	providers := Providers{
		Truth:     ProviderFunc(func(context.Context, Request) ([]Item, error) { return classified[LayerTruth], nil }),
		Narrative: ProviderFunc(func(context.Context, Request) ([]Item, error) { return classified[LayerNarrative], nil }),
		Recent:    ProviderFunc(func(context.Context, Request) ([]Item, error) { return classified[LayerRecent], nil }),
		Structured: ProviderFunc(func(context.Context, Request) ([]Item, error) {
			return historicalByStage(classified[LayerHistorical], StageStructured), nil
		}),
		Timeline: ProviderFunc(func(context.Context, Request) ([]Item, error) {
			return historicalByStage(classified[LayerHistorical], StageTimeline), nil
		}),
		Foreshadow: ProviderFunc(func(context.Context, Request) ([]Item, error) {
			return historicalByStage(classified[LayerHistorical], StageForeshadow), nil
		}),
		Relation: ProviderFunc(func(context.Context, Request) ([]Item, error) {
			return historicalByStage(classified[LayerHistorical], StageRelation), nil
		}),
		HistoricalRecent: ProviderFunc(func(context.Context, Request) ([]Item, error) {
			return historicalByStage(classified[LayerHistorical], StageRecent), nil
		}),
		FTS5: ProviderFunc(func(context.Context, Request) ([]Item, error) {
			return historicalByStage(classified[LayerHistorical], StageFTS5), nil
		}),
		Style: ProviderFunc(func(context.Context, Request) ([]Item, error) { return classified[LayerStyle], nil }),
	}
	return New(providers, counter).Compile(ctx, request)
}

func classifyLegacy(raw map[string]any, chapter int) map[Layer][]Item {
	result := make(map[Layer][]Item, len(layerOrder))
	keys := make([]string, 0, len(raw))
	for key := range raw {
		if !strings.HasPrefix(key, "_") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		if section, ok := raw[key].(map[string]any); ok && isLegacyContainer(key) {
			sectionKeys := make([]string, 0, len(section))
			for sectionKey := range section {
				if !strings.HasPrefix(sectionKey, "_") {
					sectionKeys = append(sectionKeys, sectionKey)
				}
			}
			sort.Strings(sectionKeys)
			for _, sectionKey := range sectionKeys {
				appendLegacyItem(result, key+"."+sectionKey, sectionKey, section[sectionKey], chapter)
			}
			continue
		}
		appendLegacyItem(result, key, key, raw[key], chapter)
	}
	return result
}

func isLegacyContainer(key string) bool {
	switch key {
	case "working_memory", "episodic_memory", "planning_memory", "foundation_memory", "reference_pack", "selected_memory":
		return true
	default:
		return false
	}
}

func appendLegacyItem(result map[Layer][]Item, path, semanticKey string, value any, chapter int) {
	layer, stage := legacyLayer(semanticKey)
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	item := Item{
		ID:            "legacy:" + path,
		Layer:         layer,
		Stage:         stage,
		Kind:          semanticKey,
		Title:         path,
		Content:       string(data),
		SourceChapter: chapter,
	}
	markLegacyMandatory(&item, semanticKey)
	result[layer] = append(result[layer], item)
}

func legacyLayer(key string) (Layer, RetrievalStage) {
	lower := strings.ToLower(key)
	switch {
	case strings.Contains(lower, "rule"), strings.Contains(lower, "character"), strings.Contains(lower, "knowledge"), strings.Contains(lower, "inventory"):
		return LayerTruth, StageNone
	case strings.Contains(lower, "plan"), strings.Contains(lower, "outline"), strings.Contains(lower, "compass"), strings.Contains(lower, "volume"), strings.Contains(lower, "arc"):
		return LayerNarrative, StageNone
	case strings.Contains(lower, "recent"), strings.Contains(lower, "previous_tail"):
		return LayerRecent, StageNone
	case strings.Contains(lower, "timeline"):
		return LayerHistorical, StageTimeline
	case strings.Contains(lower, "foreshadow"):
		return LayerHistorical, StageForeshadow
	case strings.Contains(lower, "relation"):
		return LayerHistorical, StageRelation
	case strings.Contains(lower, "selected_memory"), strings.Contains(lower, "related_chapter"):
		return LayerHistorical, StageStructured
	case strings.Contains(lower, "style"), strings.Contains(lower, "voice"), strings.Contains(lower, "simulation"), strings.Contains(lower, "reference"):
		return LayerStyle, StageNone
	default:
		return LayerTruth, StageNone
	}
}

func markLegacyMandatory(item *Item, key string) {
	lower := strings.ToLower(key)
	switch {
	case strings.Contains(lower, "chapter_plan") || strings.Contains(lower, "current_plan"):
		item.Requirement = RequirementCurrentChapterPlan
	case strings.Contains(lower, "pov") && strings.Contains(lower, "state"):
		item.Requirement = RequirementPOVCharacterState
	case strings.Contains(lower, "critical_world_rule"):
		item.Requirement = RequirementCriticalWorldRule
	case strings.Contains(lower, "critical_foreshadow"):
		item.Requirement = RequirementCriticalForeshadow
	case strings.Contains(lower, "knowledge_boundary"):
		item.Requirement = RequirementKnowledgeBoundary
	case strings.Contains(lower, "contract_beat"):
		item.Requirement = RequirementRequiredContractBeat
	}
	item.Mandatory = item.Requirement != ""
	if item.Mandatory {
		item.Priority = 1000
	}
}

func historicalByStage(items []Item, stage RetrievalStage) []Item {
	out := make([]Item, 0, len(items))
	for _, item := range items {
		if item.Stage == stage {
			out = append(out, item)
		}
	}
	return out
}

// DiagnosticsJSON is a stable helper for the legacy tool/API surfaces.
func DiagnosticsJSON(result Result) (json.RawMessage, error) {
	data, err := json.Marshal(struct {
		ContextSHA  string      `json:"context_sha"`
		Diagnostics Diagnostics `json:"diagnostics"`
	}{result.ContextSHA, result.Diagnostics})
	if err != nil {
		return nil, fmt.Errorf("marshal context diagnostics: %w", err)
	}
	return data, nil
}
