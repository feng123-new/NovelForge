package contextcompiler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type Compiler struct {
	providers Providers
	counter   TokenCounter
}

func New(providers Providers, counter TokenCounter) *Compiler {
	if counter == nil {
		counter = HeuristicTokenCounter{}
	}
	return &Compiler{providers: providers, counter: counter}
}

func (c *Compiler) Compile(ctx context.Context, request Request) (Result, error) {
	request, err := request.normalized()
	if err != nil {
		return Result{}, err
	}
	diagnostics := newDiagnostics(request)
	collected, err := c.collect(ctx, request)
	if err != nil {
		return Result{}, err
	}
	items, err := c.prepare(collected, request, &diagnostics)
	if err != nil {
		return Result{}, err
	}
	selected, err := c.allocate(items, request, &diagnostics)
	if err != nil {
		return Result{}, err
	}
	if err := validateRequirements(selected, request.RequiredRequirements); err != nil {
		return Result{}, err
	}
	stableResultOrder(selected)
	text := render(selected)
	result := Result{
		ProjectID:   request.ProjectID,
		Chapter:     request.Chapter,
		Items:       selected,
		Text:        text,
		Diagnostics: diagnostics,
	}
	result.ContextSHA, err = resultHash(result)
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

func newDiagnostics(request Request) Diagnostics {
	d := Diagnostics{
		TotalTokens:  request.TotalTokens,
		SystemTokens: request.TotalTokens * request.Budget.System / 100,
		Layers:       make(map[Layer]*LayerDiagnostics, len(layerOrder)),
	}
	d.ContentTokens = request.TotalTokens - d.SystemTokens
	for _, layer := range layerOrder {
		d.Layers[layer] = &LayerDiagnostics{
			Layer:           layer,
			AllocatedTokens: request.TotalTokens * request.Budget.percentage(layer) / 100,
		}
	}
	return d
}

func (c *Compiler) collect(ctx context.Context, request Request) ([]Item, error) {
	var all []Item
	collect := func(provider ItemProvider, layer Layer, stage RetrievalStage) error {
		if provider == nil {
			return nil
		}
		items, err := provider.Collect(ctx, request)
		if err != nil {
			return err
		}
		for i := range items {
			if items[i].Layer == "" {
				items[i].Layer = layer
			}
			if items[i].Stage == "" {
				items[i].Stage = stage
			}
		}
		all = append(all, items...)
		return nil
	}
	if err := collect(c.providers.Truth, LayerTruth, StageNone); err != nil {
		return nil, fmt.Errorf("collect truth: %w", err)
	}
	if err := collect(c.providers.Narrative, LayerNarrative, StageNone); err != nil {
		return nil, fmt.Errorf("collect narrative: %w", err)
	}
	if err := collect(c.providers.Recent, LayerRecent, StageNone); err != nil {
		return nil, fmt.Errorf("collect recent: %w", err)
	}
	for _, entry := range []struct {
		provider ItemProvider
		stage    RetrievalStage
	}{
		{c.providers.Structured, StageStructured},
		{c.providers.Timeline, StageTimeline},
		{c.providers.Foreshadow, StageForeshadow},
		{c.providers.Relation, StageRelation},
		{c.providers.HistoricalRecent, StageRecent},
		{c.providers.FTS5, StageFTS5},
	} {
		if err := collect(entry.provider, LayerHistorical, entry.stage); err != nil {
			return nil, fmt.Errorf("collect historical %s: %w", entry.stage, err)
		}
	}
	if c.providers.Vector != nil {
		items, err := c.providers.Vector.Retrieve(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("collect historical vector: %w", err)
		}
		for i := range items {
			items[i].Layer = LayerHistorical
			items[i].Stage = StageVector
		}
		all = append(all, items...)
	}
	if err := collect(c.providers.Style, LayerStyle, StageNone); err != nil {
		return nil, fmt.Errorf("collect style: %w", err)
	}
	return all, nil
}

func (c *Compiler) prepare(input []Item, request Request, d *Diagnostics) ([]Item, error) {
	seen := make(map[string]struct{}, len(input))
	prepared := make([]Item, 0, len(input))
	for i, item := range input {
		item.ID = strings.TrimSpace(item.ID)
		item.Kind = strings.TrimSpace(item.Kind)
		item.Content = strings.TrimSpace(item.Content)
		if item.ID == "" {
			item.ID = fmt.Sprintf("%s-%s-%06d", item.Layer, item.Kind, i)
		}
		layerDiagnostics, ok := d.Layers[item.Layer]
		if !ok {
			appendTrim(d, item, TrimInvalidLayer)
			continue
		}
		if item.Content == "" {
			layerDiagnostics.Trimmed = append(layerDiagnostics.Trimmed, trimmed(item, TrimEmpty))
			continue
		}
		if item.Tokens <= 0 {
			item.Tokens = c.counter.Count(item.Title + "\n" + item.Content)
		}
		layerDiagnostics.InputTokens += item.Tokens
		key := string(item.Layer) + "\x00" + string(item.Stage) + "\x00" + item.ID
		if _, exists := seen[key]; exists {
			layerDiagnostics.Trimmed = append(layerDiagnostics.Trimmed, trimmed(item, TrimDuplicate))
			d.DuplicateItems++
			continue
		}
		seen[key] = struct{}{}
		if item.SourceChapter > request.Chapter {
			if item.Mandatory || item.Requirement != "" {
				return nil, fmt.Errorf("%w: %s chapter=%d request=%d", ErrFutureMandatory, item.ID, item.SourceChapter, request.Chapter)
			}
			layerDiagnostics.Trimmed = append(layerDiagnostics.Trimmed, trimmed(item, TrimFutureState))
			d.FutureItems++
			continue
		}
		if item.Requirement != "" {
			item.Mandatory = true
		}
		prepared = append(prepared, item)
	}
	stableCandidateOrder(prepared)
	return prepared, nil
}

func appendTrim(d *Diagnostics, item Item, reason TrimReason) {
	if layer, ok := d.Layers[item.Layer]; ok {
		layer.Trimmed = append(layer.Trimmed, trimmed(item, reason))
	}
}

func trimmed(item Item, reason TrimReason) TrimmedItem {
	return TrimmedItem{ID: item.ID, Kind: item.Kind, Tokens: item.Tokens, Reason: reason}
}

func (c *Compiler) allocate(items []Item, request Request, d *Diagnostics) ([]Item, error) {
	selected := make([]Item, 0, len(items))
	selectedKeys := make(map[string]struct{}, len(items))
	mandatoryTokens := 0
	for _, item := range items {
		if !item.Mandatory {
			continue
		}
		mandatoryTokens += item.Tokens
		if mandatoryTokens > d.ContentTokens {
			return nil, fmt.Errorf("%w: mandatory=%d content_budget=%d", ErrMandatoryOverflow, mandatoryTokens, d.ContentTokens)
		}
		selected = append(selected, item)
		selectedKeys[itemKey(item)] = struct{}{}
		markSelected(d, item)
	}

	// First honor each explicit layer allocation. Mandatory items are charged
	// against their own layer and may borrow from later unused capacity.
	for _, layer := range layerOrder {
		ld := d.Layers[layer]
		for _, item := range items {
			if item.Layer != layer || item.Mandatory {
				continue
			}
			if ld.UsedTokens+item.Tokens > ld.AllocatedTokens {
				ld.Trimmed = append(ld.Trimmed, trimmed(item, TrimLayerBudget))
				continue
			}
			if d.UsedTokens+item.Tokens > d.ContentTokens {
				ld.Trimmed = append(ld.Trimmed, trimmed(item, TrimTotalBudget))
				continue
			}
			selected = append(selected, item)
			selectedKeys[itemKey(item)] = struct{}{}
			markSelected(d, item)
		}
	}

	// Deterministically redistribute unused content tokens. This prevents an
	// empty layer from wasting budget while preserving stable priority order.
	for _, item := range items {
		if _, ok := selectedKeys[itemKey(item)]; ok {
			continue
		}
		if item.Mandatory {
			continue
		}
		if d.UsedTokens+item.Tokens > d.ContentTokens {
			continue
		}
		selected = append(selected, item)
		selectedKeys[itemKey(item)] = struct{}{}
		markSelected(d, item)
		removeTrimRecord(d.Layers[item.Layer], item.ID, TrimLayerBudget)
	}

	// Any still-unselected candidate receives a final explicit reason.
	for _, item := range items {
		if _, ok := selectedKeys[itemKey(item)]; ok {
			continue
		}
		ld := d.Layers[item.Layer]
		if !hasTrimRecord(ld, item.ID) {
			ld.Trimmed = append(ld.Trimmed, trimmed(item, TrimTotalBudget))
		}
	}
	d.RemainingTokens = d.ContentTokens - d.UsedTokens
	return selected, nil
}

func markSelected(d *Diagnostics, item Item) {
	ld := d.Layers[item.Layer]
	ld.UsedTokens += item.Tokens
	ld.SelectedCount++
	d.UsedTokens += item.Tokens
}

func itemKey(item Item) string {
	return string(item.Layer) + "\x00" + string(item.Stage) + "\x00" + item.ID
}

func hasTrimRecord(ld *LayerDiagnostics, id string) bool {
	for _, record := range ld.Trimmed {
		if record.ID == id {
			return true
		}
	}
	return false
}

func removeTrimRecord(ld *LayerDiagnostics, id string, reason TrimReason) {
	out := ld.Trimmed[:0]
	for _, record := range ld.Trimmed {
		if record.ID == id && record.Reason == reason {
			continue
		}
		out = append(out, record)
	}
	ld.Trimmed = out
}

func stableCandidateOrder(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Mandatory != b.Mandatory {
			return a.Mandatory
		}
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if layerRank(a.Layer) != layerRank(b.Layer) {
			return layerRank(a.Layer) < layerRank(b.Layer)
		}
		if stageRank(a.Stage) != stageRank(b.Stage) {
			return stageRank(a.Stage) < stageRank(b.Stage)
		}
		if a.SourceChapter != b.SourceChapter {
			return a.SourceChapter > b.SourceChapter
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.ID < b.ID
	})
}

func stableResultOrder(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if layerRank(a.Layer) != layerRank(b.Layer) {
			return layerRank(a.Layer) < layerRank(b.Layer)
		}
		if stageRank(a.Stage) != stageRank(b.Stage) {
			return stageRank(a.Stage) < stageRank(b.Stage)
		}
		if a.Mandatory != b.Mandatory {
			return a.Mandatory
		}
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if a.SourceChapter != b.SourceChapter {
			return a.SourceChapter > b.SourceChapter
		}
		return a.ID < b.ID
	})
}

func layerRank(layer Layer) int {
	for i, candidate := range layerOrder {
		if candidate == layer {
			return i
		}
	}
	return len(layerOrder)
}

func stageRank(stage RetrievalStage) int {
	if stage == StageNone {
		return -1
	}
	for i, candidate := range historicalStageOrder {
		if candidate == stage {
			return i
		}
	}
	return len(historicalStageOrder)
}

func validateRequirements(items []Item, required []Requirement) error {
	if len(required) == 0 {
		return nil
	}
	seen := make(map[Requirement]bool, len(required))
	for _, item := range items {
		if item.Requirement != "" && item.Mandatory {
			seen[item.Requirement] = true
		}
	}
	for _, requirement := range required {
		if !seen[requirement] {
			return fmt.Errorf("%w: %s", ErrMissingRequirement, requirement)
		}
	}
	return nil
}

func render(items []Item) string {
	var b strings.Builder
	currentLayer := Layer("")
	currentStage := StageNone
	for _, item := range items {
		if item.Layer != currentLayer {
			currentLayer = item.Layer
			currentStage = StageNone
			fmt.Fprintf(&b, "## %s\n", strings.ToUpper(string(item.Layer)))
		}
		if item.Layer == LayerHistorical && item.Stage != currentStage {
			currentStage = item.Stage
			fmt.Fprintf(&b, "### %s\n", strings.ToUpper(string(item.Stage)))
		}
		if item.Title != "" {
			fmt.Fprintf(&b, "- %s: %s\n", item.Title, item.Content)
		} else {
			fmt.Fprintf(&b, "- %s\n", item.Content)
		}
	}
	return strings.TrimSpace(b.String())
}

func resultHash(result Result) (string, error) {
	payload := struct {
		ProjectID string `json:"project_id"`
		Chapter   int    `json:"chapter"`
		Items     []Item `json:"items"`
		Text      string `json:"text"`
	}{result.ProjectID, result.Chapter, result.Items, result.Text}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
