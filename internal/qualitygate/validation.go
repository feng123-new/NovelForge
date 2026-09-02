package qualitygate

import (
	"errors"
	"strings"
)

func (p ChapterPlan) Validate() error {
	if p.Chapter <= 0 {
		return errors.New("chapter must be positive")
	}
	if strings.TrimSpace(p.Title) == "" || strings.TrimSpace(p.POV) == "" || strings.TrimSpace(p.Location) == "" ||
		strings.TrimSpace(p.Objective) == "" || strings.TrimSpace(p.Conflict) == "" || strings.TrimSpace(p.EndingHook) == "" {
		return errors.New("chapter plan title, pov, location, objective, conflict and ending_hook are required")
	}
	if p.RequiredBeats == nil || p.ForbiddenOutcomes == nil || p.KnowledgeBoundary == nil || p.InventoryConstraints == nil || p.ForeshadowObligations == nil {
		return errors.New("chapter plan collections must be present")
	}
	return nil
}
