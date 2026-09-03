package contextcompiler

import (
	"context"
	"errors"
	"fmt"
)

// Layer is one of the five authoring context layers. System is budgeted
// separately and cannot be consumed by provider content.
type Layer string

const (
	LayerTruth      Layer = "truth"
	LayerNarrative  Layer = "narrative"
	LayerRecent     Layer = "recent"
	LayerHistorical Layer = "historical"
	LayerStyle      Layer = "style"
)

var layerOrder = []Layer{
	LayerTruth,
	LayerNarrative,
	LayerRecent,
	LayerHistorical,
	LayerStyle,
}

// RetrievalStage records the deterministic historical-retrieval pipeline.
type RetrievalStage string

const (
	StageNone       RetrievalStage = ""
	StageStructured RetrievalStage = "structured_query"
	StageTimeline   RetrievalStage = "timeline"
	StageForeshadow RetrievalStage = "foreshadow"
	StageRelation   RetrievalStage = "relation"
	StageRecent     RetrievalStage = "recent"
	StageFTS5       RetrievalStage = "fts5"
	StageVector     RetrievalStage = "vector"
)

var historicalStageOrder = []RetrievalStage{
	StageStructured,
	StageTimeline,
	StageForeshadow,
	StageRelation,
	StageRecent,
	StageFTS5,
	StageVector,
}

// Requirement identifies context that must survive every trim operation.
type Requirement string

const (
	RequirementCurrentChapterPlan   Requirement = "current_chapter_plan"
	RequirementPOVCharacterState    Requirement = "pov_character_state"
	RequirementCriticalWorldRule    Requirement = "critical_world_rule"
	RequirementCriticalForeshadow   Requirement = "critical_foreshadow"
	RequirementKnowledgeBoundary    Requirement = "knowledge_boundary"
	RequirementRequiredContractBeat Requirement = "required_contract_beat"
)

// Item is one bounded, independently trimmable context record.
type Item struct {
	ID            string            `json:"id"`
	Layer         Layer             `json:"layer"`
	Stage         RetrievalStage    `json:"stage,omitempty"`
	Kind          string            `json:"kind"`
	Title         string            `json:"title,omitempty"`
	Content       string            `json:"content"`
	SourceChapter int               `json:"source_chapter"`
	SourceVersion string            `json:"source_version,omitempty"`
	Priority      int               `json:"priority"`
	Mandatory     bool              `json:"mandatory"`
	Requirement   Requirement       `json:"requirement,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Tokens        int               `json:"tokens"`
}

// BudgetConfig uses whole percentages and must total 100.
type BudgetConfig struct {
	Truth      int `json:"truth"`
	Narrative  int `json:"narrative"`
	Recent     int `json:"recent"`
	Historical int `json:"historical"`
	Style      int `json:"style"`
	System     int `json:"system"`
}

func DefaultBudgetConfig() BudgetConfig {
	return BudgetConfig{
		Truth: 20, Narrative: 15, Recent: 25,
		Historical: 20, Style: 10, System: 10,
	}
}

func (b BudgetConfig) Validate() error {
	values := []int{b.Truth, b.Narrative, b.Recent, b.Historical, b.Style, b.System}
	total := 0
	for _, value := range values {
		if value < 0 || value > 100 {
			return fmt.Errorf("%w: percentages must be between 0 and 100", ErrInvalidBudget)
		}
		total += value
	}
	if total != 100 {
		return fmt.Errorf("%w: percentages total %d, want 100", ErrInvalidBudget, total)
	}
	return nil
}

func (b BudgetConfig) percentage(layer Layer) int {
	switch layer {
	case LayerTruth:
		return b.Truth
	case LayerNarrative:
		return b.Narrative
	case LayerRecent:
		return b.Recent
	case LayerHistorical:
		return b.Historical
	case LayerStyle:
		return b.Style
	default:
		return 0
	}
}

// Request is the deterministic compiler input. RequiredRequirements is
// fail-closed: every named requirement must be supplied by a mandatory item.
type Request struct {
	ProjectID            string        `json:"project_id"`
	Chapter              int           `json:"chapter"`
	POVEntityID          string        `json:"pov_entity_id,omitempty"`
	Query                string        `json:"query,omitempty"`
	TotalTokens          int           `json:"total_tokens"`
	RecentChapterCount   int           `json:"recent_chapter_count"`
	Budget               BudgetConfig  `json:"budget"`
	RequiredRequirements []Requirement `json:"required_requirements,omitempty"`
}

func (r Request) normalized() (Request, error) {
	if r.Chapter < 0 {
		return Request{}, fmt.Errorf("chapter must be non-negative")
	}
	if r.TotalTokens <= 0 {
		return Request{}, fmt.Errorf("%w: total_tokens must be positive", ErrInvalidBudget)
	}
	if r.RecentChapterCount == 0 {
		r.RecentChapterCount = 3
	}
	if r.RecentChapterCount < 2 || r.RecentChapterCount > 5 {
		return Request{}, fmt.Errorf("recent_chapter_count must be between 2 and 5")
	}
	if r.Budget == (BudgetConfig{}) {
		r.Budget = DefaultBudgetConfig()
	}
	if err := r.Budget.Validate(); err != nil {
		return Request{}, err
	}
	return r, nil
}

// ItemProvider supplies one logical source. Implementations must not mutate
// authoritative stores during collection.
type ItemProvider interface {
	Collect(context.Context, Request) ([]Item, error)
}

type ProviderFunc func(context.Context, Request) ([]Item, error)

func (f ProviderFunc) Collect(ctx context.Context, request Request) ([]Item, error) {
	if f == nil {
		return nil, nil
	}
	return f(ctx, request)
}

// VectorRetriever is optional. V1 never requires an external vector database.
type VectorRetriever interface {
	Retrieve(context.Context, Request) ([]Item, error)
}

type Providers struct {
	Truth            ItemProvider
	Narrative        ItemProvider
	Recent           ItemProvider
	Structured       ItemProvider
	Timeline         ItemProvider
	Foreshadow       ItemProvider
	Relation         ItemProvider
	HistoricalRecent ItemProvider
	FTS5             ItemProvider
	Vector           VectorRetriever
	Style            ItemProvider
}

// TrimReason is persisted in diagnostics rather than silently discarding data.
type TrimReason string

const (
	TrimLayerBudget  TrimReason = "layer_budget"
	TrimTotalBudget  TrimReason = "total_budget"
	TrimFutureState  TrimReason = "future_state"
	TrimDuplicate    TrimReason = "duplicate"
	TrimEmpty        TrimReason = "empty"
	TrimInvalidLayer TrimReason = "invalid_layer"
)

type TrimmedItem struct {
	ID     string     `json:"id"`
	Kind   string     `json:"kind"`
	Tokens int        `json:"tokens"`
	Reason TrimReason `json:"reason"`
}

type LayerDiagnostics struct {
	Layer           Layer         `json:"layer"`
	AllocatedTokens int           `json:"allocated_tokens"`
	InputTokens     int           `json:"input_tokens"`
	UsedTokens      int           `json:"used_tokens"`
	SelectedCount   int           `json:"selected_count"`
	Trimmed         []TrimmedItem `json:"trimmed,omitempty"`
}

type Diagnostics struct {
	TotalTokens     int                         `json:"total_tokens"`
	SystemTokens    int                         `json:"system_tokens"`
	ContentTokens   int                         `json:"content_tokens"`
	UsedTokens      int                         `json:"used_tokens"`
	RemainingTokens int                         `json:"remaining_tokens"`
	FutureItems     int                         `json:"future_items"`
	DuplicateItems  int                         `json:"duplicate_items"`
	Layers          map[Layer]*LayerDiagnostics `json:"layers"`
}

type Result struct {
	ProjectID   string      `json:"project_id"`
	Chapter     int         `json:"chapter"`
	Items       []Item      `json:"items"`
	Text        string      `json:"text"`
	ContextSHA  string      `json:"context_sha"`
	Diagnostics Diagnostics `json:"diagnostics"`
}

var (
	ErrInvalidBudget      = errors.New("invalid context budget")
	ErrMandatoryOverflow  = errors.New("mandatory context exceeds content budget")
	ErrMissingRequirement = errors.New("required context is missing")
	ErrFutureMandatory    = errors.New("mandatory context is from a future chapter")
)
