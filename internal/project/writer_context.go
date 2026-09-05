package project

import (
	"context"
	"database/sql"
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

// CompileWriterContext supplies the actual Web Writer input. It never starts
// jobs, generates plans, or changes Truth/Ledger authority.
func (r *Repository) CompileWriterContext(ctx context.Context, req qualitygate.WriterRequest, totalTokens int) (json.RawMessage, error) {
	return r.compileWriterContext(ctx, req, totalTokens, nil)
}
func (r *Repository) compileWriterContext(ctx context.Context, req qualitygate.WriterRequest, totalTokens int, extra []contextcompiler.Item) (json.RawMessage, error) {
	if req.ProjectID == "" || req.Chapter < 1 || req.Plan.Chapter != req.Chapter {
		return nil, fmt.Errorf("writer context identity is invalid")
	}
	if err := req.Plan.Validate(); err != nil {
		return nil, err
	}
	if totalTokens <= 0 {
		totalTokens = 25000
	}
	entry, err := r.find(req.ProjectID)
	if err != nil {
		return nil, err
	}
	truth, err := r.OpenTruthStore(ctx, req.ProjectID)
	if err != nil {
		return nil, err
	}
	defer truth.Close()
	ledger, err := r.OpenNarrativeLedger(ctx, req.ProjectID)
	if err != nil {
		return nil, err
	}
	defer ledger.Close()
	database, err := migrate.Open(filepath.Join(entry.Root, projectDatabaseRelative), 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer database.Close()
	request := contextcompiler.Request{
		ProjectID: req.ProjectID, Chapter: req.Chapter, POVEntityID: strings.TrimSpace(req.Plan.POV),
		Query: strings.TrimSpace(req.Plan.POV), TotalTokens: totalTokens, RecentChapterCount: 3,
		Budget:               contextcompiler.DefaultBudgetConfig(),
		RequiredRequirements: []contextcompiler.Requirement{contextcompiler.RequirementCurrentChapterPlan, contextcompiler.RequirementKnowledgeBoundary},
	}
	role := "writing"
	if req.PreviousDraft != "" {
		role = "polish"
	}
	selected, selectErr := r.authoringSelection(ctx, req.ProjectID, req.TransactionID+":writer", role, req.Plan.POV+" "+strings.Join(req.Plan.RequiredBeats, " "), req.Chapter, req.Plan.POV)
	if selectErr != nil {
		return nil, selectErr
	}
	extra = append(extra, selectionItems(selected)...)
	planJSON, err := json.Marshal(req.Plan)
	if err != nil {
		return nil, err
	}
	narrative := []contextcompiler.Item{{ID: "current-plan", Layer: contextcompiler.LayerNarrative, Kind: "chapter_plan", Content: string(planJSON), SourceChapter: req.Chapter, Priority: 1000, Mandatory: true, Requirement: contextcompiler.RequirementCurrentChapterPlan}}
	for _, item := range extra {
		if item.Layer == contextcompiler.LayerNarrative {
			narrative = append(narrative, item)
		}
	}
	boundaryJSON, err := json.Marshal(req.Plan.KnowledgeBoundary)
	if err != nil {
		return nil, err
	}
	pinned := []contextcompiler.Item{{ID: "plan-knowledge-boundary", Layer: contextcompiler.LayerTruth, Kind: "knowledge_boundary", Content: "POV may use only knowledge available at this chapter. Plan constraints: " + string(boundaryJSON), SourceChapter: req.Chapter, Priority: 1000, Mandatory: true, Requirement: contextcompiler.RequirementKnowledgeBoundary}}
	for i, beat := range req.Plan.RequiredBeats {
		pinned = append(pinned, contextcompiler.Item{ID: fmt.Sprintf("required-beat:%d", i), Layer: contextcompiler.LayerNarrative, Kind: "contract_beat", Content: beat, SourceChapter: req.Chapter, Priority: 1000, Mandatory: true, Requirement: contextcompiler.RequirementRequiredContractBeat})
	}
	ledgerContext, err := ledger.PlannerContext(ctx, req.ProjectID, req.Chapter, request.POVEntityID, "", 3)
	if err != nil {
		return nil, err
	}
	for _, f := range ledgerContext.Foreshadows {
		item := contextcompiler.Item{ID: "ledger:foreshadow:" + f.ID, Layer: contextcompiler.LayerHistorical, Stage: contextcompiler.StageForeshadow, Kind: f.Kind, Title: f.Title, Content: f.Summary, SourceChapter: f.SourceChapter, SourceVersion: f.SourceVersion, Priority: 850, Mandatory: f.Mandatory}
		if f.Mandatory {
			item.Layer = contextcompiler.LayerTruth
			item.Requirement = contextcompiler.RequirementCriticalForeshadow
			item.Priority = 1000
		}
		pinned = append(pinned, item)
	}
	for _, secret := range ledgerContext.KnownSecrets {
		pinned = append(pinned, contextcompiler.Item{ID: "ledger:secret:" + secret.ID, Layer: contextcompiler.LayerTruth, Kind: "known_secret", Content: secret.Summary, SourceChapter: secret.SourceChapter, SourceVersion: secret.SourceVersion, Priority: 900})
	}
	for _, boundary := range ledgerContext.UnknownSecrets {
		// Do not copy even a management title into unknown-role context.
		pinned = append(pinned, contextcompiler.Item{ID: "ledger:boundary:" + boundary.ID, Layer: contextcompiler.LayerTruth, Kind: "knowledge_boundary", Content: boundary.Summary, SourceChapter: boundary.SourceChapter, SourceVersion: boundary.SourceVersion, Priority: 1000, Mandatory: true, Requirement: contextcompiler.RequirementKnowledgeBoundary})
	}
	reader := povTruthReader{store: truth, pov: request.POVEntityID}
	truthItems, err := (contextcompiler.TruthProvider{Store: reader}).Collect(ctx, request)
	if err != nil {
		return nil, err
	}
	hasPOV := false
	for _, item := range truthItems {
		if item.Requirement == contextcompiler.RequirementPOVCharacterState {
			hasPOV = true
		}
	}
	if request.POVEntityID != "" && req.Chapter > 1 && !hasPOV {
		return nil, fmt.Errorf("%w: accepted POV state was not found", contextcompiler.ErrMissingRequirement)
	}
	items := append(truthItems, pinned...)
	for _, item := range extra {
		if item.Layer != contextcompiler.LayerNarrative {
			items = append(items, item)
		}
	}
	all := func(layer contextcompiler.Layer, stage contextcompiler.RetrievalStage) contextcompiler.ItemProvider {
		return contextcompiler.ProviderFunc(func(context.Context, contextcompiler.Request) ([]contextcompiler.Item, error) {
			var out []contextcompiler.Item
			for _, item := range items {
				if item.Layer == layer && (stage == contextcompiler.StageNone || item.Stage == stage) {
					out = append(out, item)
				}
			}
			return out, nil
		})
	}
	providers := contextcompiler.Providers{
		Truth: all(contextcompiler.LayerTruth, contextcompiler.StageNone),
		Narrative: contextcompiler.ProviderFunc(func(context.Context, contextcompiler.Request) ([]contextcompiler.Item, error) {
			out := append([]contextcompiler.Item(nil), narrative...)
			for _, item := range pinned {
				if item.Layer == contextcompiler.LayerNarrative {
					out = append(out, item)
				}
			}
			return out, nil
		}),
		Structured: all(contextcompiler.LayerHistorical, contextcompiler.StageStructured),
		Foreshadow: all(contextcompiler.LayerHistorical, contextcompiler.StageForeshadow),
		Recent:     recentContextReader{db: database},
		FTS5:       contextcompiler.NewFTSStore(database),
		Style:      all(contextcompiler.LayerStyle, contextcompiler.StageNone),
	}
	compiled, err := contextcompiler.New(providers, nil).Compile(ctx, request)
	if err != nil {
		return nil, err
	}
	// Send selected text once, without the original untrimmed provider records.
	return json.Marshal(struct {
		Text       string `json:"text"`
		ContextSHA string `json:"context_sha"`
		UsedTokens int    `json:"used_tokens"`
	}{compiled.Text, compiled.ContextSHA, compiled.Diagnostics.UsedTokens})
}

// Global author Truth is not identical to character knowledge. Secrets must
// come from the role-filtered Ledger, not a parallel unfiltered Truth path.
type povTruthReader struct {
	store contextcompiler.TruthStateReader
	pov   string
}

func (p povTruthReader) State(ctx context.Context, q truthstore.StateQuery) (truthstore.StatePage, error) {
	page, err := p.store.State(ctx, q)
	if err != nil {
		return page, err
	}
	kept := make([]truthstore.Fact, 0, len(page.Facts))
	for _, fact := range page.Facts {
		predicate := strings.ToLower(fact.Predicate)
		subject := strings.ToLower(fact.SubjectType)
		if strings.Contains(subject, "secret") || strings.Contains(predicate, "secret") {
			continue
		}
		if (strings.Contains(subject, "knowledge") || strings.Contains(predicate, "knowledge") || strings.Contains(predicate, "knows")) && fact.SubjectID != p.pov {
			continue
		}
		kept = append(kept, fact)
	}
	page.Facts = kept
	return page, nil
}

type recentContextReader struct{ db *sql.DB }

func (p recentContextReader) Collect(ctx context.Context, req contextcompiler.Request) ([]contextcompiler.Item, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT id,kind,title,content,source_chapter,source_version FROM (
        SELECT id,kind,title,content,source_chapter,source_version,
        row_number() OVER (PARTITION BY source_chapter ORDER BY priority DESC,id) AS rank
        FROM context_documents WHERE project_id=? AND source_chapter>0 AND source_chapter<?
    ) WHERE rank=1 ORDER BY source_chapter DESC,id LIMIT ?`, req.ProjectID, req.Chapter, req.RecentChapterCount)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []contextcompiler.Item
	for rows.Next() {
		var item contextcompiler.Item
		if err := rows.Scan(&item.ID, &item.Kind, &item.Title, &item.Content, &item.SourceChapter, &item.SourceVersion); err != nil {
			return nil, err
		}
		item.ID = "recent:" + item.ID
		item.Layer = contextcompiler.LayerRecent
		item.Priority = 900
		items = append(items, item)
	}
	return items, rows.Err()
}
