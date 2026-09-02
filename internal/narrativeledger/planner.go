package narrativeledger

import (
	"context"
	"fmt"
	"strings"
)

// ContextProvider is the stable Phase 6 boundary consumed by Phase 7. It
// returns bounded, metadata-only records; Secret truth is never included in an
// unknown boundary item.
type ContextProvider interface {
	PlannerContext(context.Context, string, int, string, string, int) (PlannerContext, error)
}

func (s *Store) PlannerContext(ctx context.Context, projectID string, chapter int, pov, currentArc string, upcomingWindow int) (PlannerContext, error) {
	if chapter < 0 {
		return PlannerContext{}, fmt.Errorf("%w: chapter must not be negative", ErrValidation)
	}
	if upcomingWindow <= 0 {
		upcomingWindow = 3
	}
	if upcomingWindow > 20 {
		upcomingWindow = 20
	}
	result := PlannerContext{ProjectID: projectID, Chapter: chapter, POV: strings.TrimSpace(pov), Foreshadows: []PlannerItem{}, KnownSecrets: []PlannerItem{}, UnknownSecrets: []PlannerItem{}}
	page, err := s.ListForeshadows(ctx, projectID, ForeshadowQuery{CurrentChapter: chapter, Limit: 100})
	if err != nil {
		return PlannerContext{}, err
	}
	for _, item := range page.Foreshadows {
		active := item.Status != StatusResolved && item.Status != StatusAbandoned
		arcRelated := strings.TrimSpace(currentArc) != "" && contains(item.RelatedArcs, strings.TrimSpace(currentArc))
		upcoming := active && item.ExpectedPayoffMin <= chapter+upcomingWindow && item.ExpectedPayoffMax >= chapter
		mandatory := item.Overdue || (active && item.Importance == ImportanceCritical)
		if !mandatory && !arcRelated && !upcoming {
			continue
		}
		kind := "upcoming_foreshadow"
		if arcRelated {
			kind = "arc_foreshadow"
		}
		if item.Importance == ImportanceCritical && active {
			kind = "critical_foreshadow"
		}
		if item.Overdue {
			kind = "overdue_foreshadow"
		}
		result.Foreshadows = append(result.Foreshadows, PlannerItem{
			ID: item.ID, Kind: kind, Title: item.Title, Summary: item.Description,
			Mandatory: mandatory, Importance: item.Importance, Urgency: item.Urgency,
			SourceChapter: item.LastProgressChapter, SourceVersion: item.SourceVersion,
			Authority: item.Authority, Metadata: map[string]any{
				"status": item.Status, "expected_payoff_min": item.ExpectedPayoffMin,
				"expected_payoff_max": item.ExpectedPayoffMax, "overdue": item.Overdue,
				"overdue_by_chapters": item.OverdueByChapters, "related_arcs": item.RelatedArcs,
			},
		})
	}
	secrets, err := s.ListSecrets(ctx, projectID, SecretQuery{CurrentChapter: chapter, IncludeTruth: true, Limit: 100})
	if err != nil {
		return PlannerContext{}, err
	}
	for _, secret := range secrets.Secrets {
		known := secret.PublicAtChapter || holderContains(secret.Holders, pov)
		if known {
			result.KnownSecrets = append(result.KnownSecrets, PlannerItem{
				ID: secret.ID, Kind: "known_secret", Title: secret.Description, Summary: secret.Truth,
				Mandatory: false, SourceChapter: secret.CreatedChapter, SourceVersion: secret.SourceVersion,
				Authority: secret.Authority, Metadata: map[string]any{"public": secret.PublicAtChapter, "holders": holderIDs(secret.Holders)},
			})
			continue
		}
		// Deliberately omit Secret.Truth. This item states the boundary without
		// leaking future/private knowledge into Writer or POV context.
		result.UnknownSecrets = append(result.UnknownSecrets, PlannerItem{
			ID: secret.ID, Kind: "unknown_secret_boundary", Title: secret.Description,
			Summary:   "POV does not know this secret at the requested chapter",
			Mandatory: true, SourceChapter: secret.CreatedChapter, SourceVersion: secret.SourceVersion,
			Authority: secret.Authority, Metadata: map[string]any{"public": false, "pov": pov},
		})
	}
	return result, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func holderContains(values []KnowledgeHolder, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if value.EntityID == target {
			return true
		}
	}
	return false
}

func holderIDs(values []KnowledgeHolder) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.EntityID)
	}
	return out
}
