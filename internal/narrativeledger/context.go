package narrativeledger

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const defaultUpcomingHorizon = 3

// BuildPlannerContext injects every CRITICAL and OVERDUE item plus bounded UPCOMING items.
// Mandatory items are never trimmed in Phase 6; Phase 7 will account for their tokens.
func (s *Store) BuildPlannerContext(ctx context.Context, chapter, optionalLimit int) (PlannerContext, error) {
	if chapter < 0 {
		return PlannerContext{}, newError("LEDGER_CHAPTER_INVALID", "chapter must not be negative", ErrValidation)
	}
	if chapter == 0 {
		current, err := s.currentChapter(ctx)
		if err != nil {
			return PlannerContext{}, err
		}
		chapter = current
	}
	if optionalLimit <= 0 {
		optionalLimit = 20
	}
	items, err := s.allForeshadows(ctx, chapter)
	if err != nil {
		return PlannerContext{}, err
	}
	selected := make([]PlannerItem, 0, len(items))
	seen := map[string]struct{}{}
	appendItem := func(item Foreshadow, category string, mandatory bool) {
		if _, exists := seen[item.Key]; exists {
			return
		}
		seen[item.Key] = struct{}{}
		selected = append(selected, PlannerItem{
			Category:   category,
			Mandatory:  mandatory,
			Key:        item.Key,
			Title:      item.Title,
			Priority:   item.Priority,
			DueChapter: item.DueChapter,
		})
	}
	for _, item := range items {
		if item.EffectiveStatus == ForeshadowOverdue {
			appendItem(item, "OVERDUE", true)
		}
	}
	for _, item := range items {
		if item.Priority == PriorityCritical && activeForeshadow(item.Status) {
			appendItem(item, "CRITICAL", true)
		}
	}
	optional := 0
	for _, item := range items {
		if optional >= optionalLimit || item.DueChapter == nil || !activeForeshadow(item.Status) {
			continue
		}
		if *item.DueChapter >= chapter && *item.DueChapter <= chapter+defaultUpcomingHorizon {
			before := len(selected)
			appendItem(item, "UPCOMING", false)
			if len(selected) > before {
				optional++
			}
		}
	}
	var text strings.Builder
	text.WriteString("[NARRATIVE_LEDGER]\n")
	text.WriteString("Authority: accepted Final and explicit human ledger events only.\n")
	if len(selected) == 0 {
		text.WriteString("No active ledger obligations for this chapter.\n")
	}
	for _, item := range selected {
		mandatory := "optional"
		if item.Mandatory {
			mandatory = "MANDATORY"
		}
		due := "unscheduled"
		if item.DueChapter != nil {
			due = fmt.Sprintf("due=%d", *item.DueChapter)
		}
		fmt.Fprintf(&text, "- %s %s key=%s priority=%s %s title=%q\n", item.Category, mandatory, item.Key, item.Priority, due, item.Title)
	}
	return PlannerContext{Chapter: chapter, Items: selected, Text: text.String()}, nil
}

func (s *Store) allForeshadows(ctx context.Context, chapter int) ([]Foreshadow, error) {
	items := []Foreshadow{}
	for offset := 0; ; offset += maximumPageLimit {
		page, err := s.ListForeshadows(ctx, ListOptions{
			AsOfChapter: chapter,
			Limit:       maximumPageLimit,
			Offset:      offset,
		})
		if err != nil {
			return nil, err
		}
		items = append(items, page.Items...)
		if page.NextOffset == nil {
			break
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		if left.EffectiveStatus != right.EffectiveStatus {
			return left.EffectiveStatus == ForeshadowOverdue
		}
		leftRank, rightRank := priorityRank(left.Priority), priorityRank(right.Priority)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		leftDue, rightDue := int(^uint(0)>>1), int(^uint(0)>>1)
		if left.DueChapter != nil {
			leftDue = *left.DueChapter
		}
		if right.DueChapter != nil {
			rightDue = *right.DueChapter
		}
		if leftDue != rightDue {
			return leftDue < rightDue
		}
		return left.Key < right.Key
	})
	return items, nil
}

func activeForeshadow(status ForeshadowStatus) bool {
	return status == ForeshadowPlanned || status == ForeshadowPlanted || status == ForeshadowReinforced
}

func priorityRank(priority Priority) int {
	switch priority {
	case PriorityCritical:
		return 0
	case PriorityHigh:
		return 1
	case PriorityNormal:
		return 2
	default:
		return 3
	}
}

// Dashboard derives real workspace counts from the Chapter-N views.
func (s *Store) Dashboard(ctx context.Context, chapter int) (Dashboard, error) {
	if chapter < 0 {
		return Dashboard{}, newError("LEDGER_CHAPTER_INVALID", "chapter must not be negative", ErrValidation)
	}
	if chapter == 0 {
		current, err := s.currentChapter(ctx)
		if err != nil {
			return Dashboard{}, err
		}
		chapter = current
	}
	foreshadows, err := s.allForeshadows(ctx, chapter)
	if err != nil {
		return Dashboard{}, err
	}
	result := Dashboard{Chapter: chapter, ForeshadowsTotal: len(foreshadows)}
	for _, item := range foreshadows {
		if activeForeshadow(item.Status) {
			result.ForeshadowsActive++
		}
		if item.Priority == PriorityCritical && activeForeshadow(item.Status) {
			result.ForeshadowsCritical++
		}
		if item.EffectiveStatus == ForeshadowOverdue {
			result.ForeshadowsOverdue++
		}
		if item.DueChapter != nil && activeForeshadow(item.Status) && *item.DueChapter >= chapter && *item.DueChapter <= chapter+defaultUpcomingHorizon {
			result.ForeshadowsUpcoming++
		}
	}
	for offset := 0; ; offset += maximumPageLimit {
		page, err := s.ListSecrets(ctx, ListOptions{AsOfChapter: chapter, Limit: maximumPageLimit, Offset: offset})
		if err != nil {
			return Dashboard{}, err
		}
		result.SecretsTotal += len(page.Items)
		for _, item := range page.Items {
			if item.Public {
				result.SecretsPublic++
			} else {
				result.SecretsHidden++
			}
		}
		if page.NextOffset == nil {
			break
		}
	}
	return result, nil
}

// Diagnostics emits stable codes; it does not invent semantic facts.
func (s *Store) Diagnostics(ctx context.Context, chapter int) ([]Diagnostic, error) {
	if chapter < 0 {
		return nil, newError("LEDGER_CHAPTER_INVALID", "chapter must not be negative", ErrValidation)
	}
	if chapter == 0 {
		current, err := s.currentChapter(ctx)
		if err != nil {
			return nil, err
		}
		chapter = current
	}
	items, err := s.allForeshadows(ctx, chapter)
	if err != nil {
		return nil, err
	}
	result := []Diagnostic{}
	for _, item := range items {
		switch {
		case item.EffectiveStatus == ForeshadowOverdue:
			result = append(result, Diagnostic{
				Code:       "LEDGER_FORESHADOW_OVERDUE",
				Severity:   "error",
				EntityType: "foreshadow",
				EntityKey:  item.Key,
				Chapter:    chapter,
				Message:    "active foreshadow is past its due chapter",
			})
		case item.DueChapter != nil && activeForeshadow(item.Status) && *item.DueChapter >= chapter && *item.DueChapter <= chapter+defaultUpcomingHorizon:
			result = append(result, Diagnostic{
				Code:       "LEDGER_FORESHADOW_DUE_SOON",
				Severity:   "warning",
				EntityType: "foreshadow",
				EntityKey:  item.Key,
				Chapter:    chapter,
				Message:    "active foreshadow is due within three chapters",
			})
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Severity != result[j].Severity {
			return result[i].Severity == "error"
		}
		if result[i].Code != result[j].Code {
			return result[i].Code < result[j].Code
		}
		return result[i].EntityKey < result[j].EntityKey
	})
	return result, nil
}
