package project

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/voocel/ainovel-cli/internal/autopilot"
	"github.com/voocel/ainovel-cli/internal/contextcompiler"
	"github.com/voocel/ainovel-cli/internal/db/migrate"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
)

// AcquireExecution uses the exact OS lease filename used by the retained Host.
// API mutations and the new worker therefore exclude TUI/Headless writes too.
func (r *Repository) AcquireExecution(ctx context.Context, id string) (io.Closer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entry, err := r.find(id)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(entry.Root, ".ainovel.lock")
	if info, e := os.Lstat(path); e == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, ErrUnsafePath
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return nil, e
	}
	lock := flock.New(path, flock.SetPermissions(0600))
	ok, err := lock.TryLock()
	if err != nil || !ok {
		lock.Close()
		return nil, ErrConflict
	}
	return lock, nil
}

// PlanningContext is author planning data filtered for the selected POV. It
// cannot write Truth; a generated plan is never an accepted chapter.
func (r *Repository) PlanningContext(ctx context.Context, id string, chapter int, pov string) (json.RawMessage, error) {
	entry, err := r.find(id)
	if err != nil {
		return nil, err
	}
	truth, err := r.OpenTruthStore(ctx, id)
	if err != nil {
		return nil, err
	}
	defer truth.Close()
	ledger, err := r.OpenNarrativeLedger(ctx, id)
	if err != nil {
		return nil, err
	}
	defer ledger.Close()
	db, err := migrate.Open(filepath.Join(entry.Root, projectDatabaseRelative), 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	request := contextcompiler.Request{ProjectID: id, Chapter: chapter, POVEntityID: pov, Query: pov, TotalTokens: 12000, RecentChapterCount: 3, Budget: contextcompiler.DefaultBudgetConfig()}
	truthProvider := contextcompiler.TruthProvider{Store: povTruthReader{store: truth, pov: pov}}
	snapshot, err := ledger.PlannerContext(ctx, id, chapter, pov, "", 3)
	if err != nil {
		return nil, err
	}
	items, err := truthProvider.Collect(ctx, request)
	if err != nil {
		return nil, err
	}
	for _, f := range snapshot.Foreshadows {
		items = append(items, contextcompiler.Item{ID: "plan:foreshadow:" + f.ID, Layer: contextcompiler.LayerTruth, Kind: f.Kind, Title: f.Title, Content: f.Summary, SourceChapter: f.SourceChapter, Priority: 900, Mandatory: f.Mandatory})
	}
	for _, b := range snapshot.UnknownSecrets {
		items = append(items, contextcompiler.Item{ID: "plan:boundary:" + b.ID, Layer: contextcompiler.LayerTruth, Kind: "knowledge_boundary", Content: b.Summary, SourceChapter: b.SourceChapter, Priority: 1000, Mandatory: true})
	}
	for _, b := range snapshot.KnownSecrets {
		items = append(items, contextcompiler.Item{ID: "plan:known:" + b.ID, Layer: contextcompiler.LayerTruth, Kind: "known_secret", Content: b.Summary, SourceChapter: b.SourceChapter, Priority: 900})
	}
	result, err := contextcompiler.New(contextcompiler.Providers{Truth: contextcompiler.ProviderFunc(func(context.Context, contextcompiler.Request) ([]contextcompiler.Item, error) { return items, nil }), Recent: recentContextReader{db: db}, FTS5: contextcompiler.NewFTSStore(db)}, nil).Compile(ctx, request)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"text": result.Text, "context_sha": result.ContextSHA})
}

// FoundationContext adds planning-only foundation/style to the same budgeted
// compiler, never concatenating a second unbounded prompt after compilation.
type FoundationContext struct {
	Repository *Repository
	Foundation *autopilot.Foundation
	Style      string
}

func (p FoundationContext) CompileWriterContext(ctx context.Context, req qualitygate.WriterRequest, budget int) (json.RawMessage, error) {
	extra := []contextcompiler.Item{}
	if p.Foundation != nil {
		// Ending and private initial-state prose are author planning, not POV knowledge.
		data, err := json.Marshal(map[string]any{"story_compass": p.Foundation.StoryCompass, "arcs": p.Foundation.Arcs, "authority": "story_compass_plan_not_final"})
		if err != nil {
			return nil, err
		}
		extra = append(extra, contextcompiler.Item{ID: "autopilot:foundation", Layer: contextcompiler.LayerNarrative, Kind: "story_compass", Content: string(data), SourceChapter: 0, Priority: 850})
		for n, rule := range p.Foundation.WorldRules {
			extra = append(extra, contextcompiler.Item{ID: fmtID(n), Layer: contextcompiler.LayerTruth, Kind: "world_rule", Content: "Planning constraint (does not override accepted Final): " + rule, SourceChapter: 0, Priority: 1000, Mandatory: true, Requirement: contextcompiler.RequirementCriticalWorldRule})
		}
	}
	if p.Style != "" {
		extra = append(extra, contextcompiler.Item{ID: "autopilot:style", Layer: contextcompiler.LayerStyle, Kind: "user_style", Content: p.Style, Priority: 900})
	}
	return p.Repository.compileWriterContext(ctx, req, budget, extra)
}
func fmtID(n int) string { data, _ := json.Marshal(n); return "autopilot:rule:" + string(data) }
