package project

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/contextcompiler"
	"github.com/voocel/ainovel-cli/internal/db/migrate"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
	"github.com/voocel/ainovel-cli/internal/truthstore"
)

// WriterContext connects the existing compiler to the actual Web Writer
// request. Collection is read-only; only the project repository resolves paths.
// The returned bytes participate in the model-call idempotency hash.
func (r *Repository) WriterContext(ctx context.Context, input qualitygate.WriterRequest) (json.RawMessage, error) {
	if input.Chapter <= 0 || input.Plan.Chapter != input.Chapter {
		return nil, fmt.Errorf("writer context chapter does not match plan")
	}
	if err := input.Plan.Validate(); err != nil {
		return nil, err
	}
	entry, err := r.find(input.ProjectID)
	if err != nil {
		return nil, err
	}
	truth, err := r.OpenTruthStore(ctx, input.ProjectID)
	if err != nil {
		return nil, err
	}
	defer truth.Close()
	ledger, err := r.OpenNarrativeLedger(ctx, input.ProjectID)
	if err != nil {
		return nil, err
	}
	defer ledger.Close()
	database, err := migrate.Open(filepath.Join(entry.Root, projectDatabaseRelative), 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer database.Close()

	planJSON, err := json.Marshal(input.Plan)
	if err != nil {
		return nil, err
	}
	boundaryJSON, err := json.Marshal(input.Plan.KnowledgeBoundary)
	if err != nil {
		return nil, err
	}
	request := contextcompiler.Request{
		ProjectID: input.ProjectID, Chapter: input.Chapter, POVEntityID: input.Plan.POV,
		TotalTokens: 12000, RecentChapterCount: 3, Budget: contextcompiler.DefaultBudgetConfig(),
		RequiredRequirements: []contextcompiler.Requirement{
			contextcompiler.RequirementCurrentChapterPlan, contextcompiler.RequirementKnowledgeBoundary,
		},
	}
	providers := contextcompiler.Providers{}
	providers.Narrative = contextcompiler.ProviderFunc(func(context.Context, contextcompiler.Request) ([]contextcompiler.Item, error) {
		return []contextcompiler.Item{{
			ID: "current-chapter-plan", Kind: "chapter_plan", Content: string(planJSON),
			SourceChapter: input.Chapter, Mandatory: true, Priority: 1000,
			Requirement: contextcompiler.RequirementCurrentChapterPlan,
		}}, nil
	})
	providers.Truth = contextcompiler.ProviderFunc(func(ctx context.Context, req contextcompiler.Request) ([]contextcompiler.Item, error) {
		reader := writerTruthReader{reader: truth, pov: input.Plan.POV}
		items, err := (contextcompiler.TruthProvider{Store: reader}).Collect(ctx, req)
		if err != nil {
			return nil, err
		}
		// An explicit boundary exists even for the first chapter with no Ledger.
		items = append(items, contextcompiler.Item{
			ID: "pov-knowledge-boundary", Kind: "knowledge_boundary", Layer: contextcompiler.LayerTruth,
			Content:       "POV=" + input.Plan.POV + "; only known_secret records grant secret knowledge. Historical evidence does not grant POV knowledge. Plan boundaries=" + string(boundaryJSON),
			SourceChapter: input.Chapter, Mandatory: true, Priority: 1000,
			Requirement: contextcompiler.RequirementKnowledgeBoundary,
		})
		snapshot, err := ledger.PlannerContext(ctx, req.ProjectID, req.Chapter, req.POVEntityID, "", 3)
		if err != nil {
			return nil, err
		}
		for _, record := range snapshot.Foreshadows {
			item := contextcompiler.Item{
				ID: "ledger:foreshadow:" + record.ID, Kind: record.Kind, Layer: contextcompiler.LayerTruth,
				Title: record.Title, Content: record.Summary, SourceChapter: record.SourceChapter,
				SourceVersion: record.SourceVersion, Mandatory: record.Mandatory, Priority: 850,
			}
			if item.Mandatory {
				item.Requirement = contextcompiler.RequirementCriticalForeshadow
			}
			items = append(items, item)
		}
		for _, record := range snapshot.KnownSecrets {
			items = append(items, contextcompiler.Item{
				ID: "ledger:known:" + record.ID, Kind: "known_secret", Layer: contextcompiler.LayerTruth,
				Content: record.Summary, SourceChapter: record.SourceChapter,
				SourceVersion: record.SourceVersion, Priority: 900,
			})
		}
		for _, record := range snapshot.UnknownSecrets {
			// Neither management truth nor its free-text title enters this record.
			items = append(items, contextcompiler.Item{
				ID: "ledger:unknown:" + record.ID, Kind: "knowledge_boundary", Layer: contextcompiler.LayerTruth,
				Content:       "POV does not know secret " + record.ID + " at this chapter.",
				SourceChapter: record.SourceChapter, SourceVersion: record.SourceVersion,
				Priority: 1000, Mandatory: true, Requirement: contextcompiler.RequirementKnowledgeBoundary,
			})
		}
		return items, nil
	})
	providers.Recent = contextcompiler.ProviderFunc(func(ctx context.Context, req contextcompiler.Request) ([]contextcompiler.Item, error) {
		rows, err := database.QueryContext(ctx, `SELECT id,title,content,source_chapter,source_version
			FROM context_documents WHERE project_id=? AND source_chapter<? AND kind='chapter_final'
			ORDER BY source_chapter DESC,id LIMIT ?`, req.ProjectID, req.Chapter, req.RecentChapterCount)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var items []contextcompiler.Item
		for rows.Next() {
			item := contextcompiler.Item{Layer: contextcompiler.LayerRecent, Kind: "recent_final", Priority: 700}
			if err := rows.Scan(&item.ID, &item.Title, &item.Content, &item.SourceChapter, &item.SourceVersion); err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return items, rows.Err()
	})
	providers.FTS5 = contextcompiler.ProviderFunc(func(ctx context.Context, req contextcompiler.Request) ([]contextcompiler.Item, error) {
		fts := contextcompiler.NewFTSStore(database)
		var items []contextcompiler.Item
		seen := map[string]bool{}
		// Short, separate queries avoid requiring an entire new chapter plan to
		// occur verbatim in historical prose. Compiler deduplicates documents.
		for _, query := range []string{input.Plan.POV, input.Plan.Location, input.Plan.Title} {
			query = strings.TrimSpace(query)
			if query == "" || seen[query] {
				continue
			}
			seen[query] = true
			req.Query = query
			part, err := fts.Collect(ctx, req)
			if err != nil {
				return nil, err
			}
			items = append(items, part...)
		}
		return items, nil
	})
	compiled, err := contextcompiler.New(providers, nil).Compile(ctx, request)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Text string `json:"text"`
		SHA  string `json:"context_sha"`
		Used int    `json:"estimated_tokens"`
	}{compiled.Text, compiled.ContextSHA, compiled.Diagnostics.UsedTokens})
}

// Secret authority remains available to deterministic Continuity checks, but
// not through the generic Writer Truth channel. Ledger supplies role-filtered
// secrets. The exact POV state is queried separately so generic pagination
// cannot silently omit it.
type writerTruthReader struct {
	reader contextcompiler.TruthStateReader
	pov    string
}

func (r writerTruthReader) State(ctx context.Context, query truthstore.StateQuery) (truthstore.StatePage, error) {
	page, err := r.reader.State(ctx, query)
	if err != nil {
		return page, err
	}
	if r.pov != "" {
		povQuery := query
		povQuery.SubjectID = r.pov
		pov, err := r.reader.State(ctx, povQuery)
		if err != nil {
			return page, err
		}
		if pov.NextOffset != nil {
			return page, fmt.Errorf("%w: POV state exceeds bounded query", contextcompiler.ErrMandatoryOverflow)
		}
		page.Facts = append(page.Facts, pov.Facts...)
	}
	filtered := make([]truthstore.Fact, 0, len(page.Facts))
	seen := map[string]bool{}
	for _, fact := range page.Facts {
		kind := strings.ToLower(fact.SubjectType + "." + fact.Predicate)
		if seen[fact.ID] || strings.Contains(kind, "secret") {
			continue
		}
		if (strings.Contains(kind, "knowledge") || strings.Contains(kind, "knows")) && fact.SubjectID != r.pov {
			continue
		}
		seen[fact.ID] = true
		filtered = append(filtered, fact)
	}
	page.Facts = filtered
	return page, nil
}
