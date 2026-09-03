package contextcompiler

import (
	"context"
	"fmt"
	"strings"
)

// LedgerReader is a read-only adapter boundary. The concrete Narrative Ledger
// can map its PlannerContext response into this stable representation without
// granting the compiler any mutation capability.
type LedgerReader interface {
	Context(context.Context, LedgerRequest) (LedgerContext, error)
}

type LedgerRequest struct {
	ProjectID   string
	Chapter     int
	POVEntityID string
}

type LedgerForeshadow struct {
	ID            string
	Title         string
	Summary       string
	SourceChapter int
	SourceVersion string
	Critical      bool
	Overdue       bool
	OverdueBy     int
}

type LedgerSecret struct {
	ID            string
	Summary       string
	SourceChapter int
	SourceVersion string
}

type LedgerBoundary struct {
	SecretID      string
	Description   string
	SourceChapter int
	SourceVersion string
}

type LedgerContext struct {
	MandatoryForeshadows []LedgerForeshadow
	KnownSecrets         []LedgerSecret
	UnknownBoundaries    []LedgerBoundary
}

type LedgerProvider struct{ Reader LedgerReader }

func (p LedgerProvider) Collect(ctx context.Context, request Request) ([]Item, error) {
	if p.Reader == nil {
		return nil, nil
	}
	snapshot, err := p.Reader.Context(ctx, LedgerRequest{ProjectID: request.ProjectID, Chapter: request.Chapter, POVEntityID: request.POVEntityID})
	if err != nil {
		return nil, fmt.Errorf("query Narrative Ledger context: %w", err)
	}
	items := make([]Item, 0, len(snapshot.MandatoryForeshadows)+len(snapshot.KnownSecrets)+len(snapshot.UnknownBoundaries))
	for _, foreshadow := range snapshot.MandatoryForeshadows {
		priority := 850
		if foreshadow.Critical {
			priority = 1000
		}
		if foreshadow.Overdue {
			priority += 50
		}
		content := strings.TrimSpace(foreshadow.Summary)
		if content == "" {
			content = strings.TrimSpace(foreshadow.Title)
		}
		if foreshadow.Overdue {
			content = fmt.Sprintf("%s (OVERDUE +%d)", content, foreshadow.OverdueBy)
		}
		items = append(items, Item{ID: "ledger:foreshadow:" + foreshadow.ID, Layer: LayerTruth, Kind: "foreshadow", Title: foreshadow.Title, Content: content, SourceChapter: foreshadow.SourceChapter, SourceVersion: foreshadow.SourceVersion, Priority: priority, Mandatory: true, Requirement: RequirementCriticalForeshadow})
	}
	for _, secret := range snapshot.KnownSecrets {
		items = append(items, Item{ID: "ledger:secret:" + secret.ID, Layer: LayerTruth, Kind: "known_secret", Content: strings.TrimSpace(secret.Summary), SourceChapter: secret.SourceChapter, SourceVersion: secret.SourceVersion, Priority: 820})
	}
	for _, boundary := range snapshot.UnknownBoundaries {
		// Description is metadata-only. The authority truth is deliberately not
		// represented by this type and therefore cannot leak into the prompt.
		items = append(items, Item{ID: "ledger:boundary:" + boundary.SecretID, Layer: LayerTruth, Kind: "knowledge_boundary", Title: "Unknown Secret", Content: strings.TrimSpace(boundary.Description), SourceChapter: boundary.SourceChapter, SourceVersion: boundary.SourceVersion, Priority: 1000, Mandatory: true, Requirement: RequirementKnowledgeBoundary})
	}
	return items, nil
}
