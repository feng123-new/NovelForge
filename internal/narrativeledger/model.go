package narrativeledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/truthstore"
)

// Status is the persisted Foreshadow lifecycle state. OVERDUE is deliberately
// not a Status; it is computed for a Chapter-N query.
type Status string

const (
	StatusPlanned      Status = "planned"
	StatusPlanted      Status = "planted"
	StatusProgressing  Status = "progressing"
	StatusResolved     Status = "resolved"
	StatusAbandoned    Status = "abandoned"
	StatusContradicted Status = "contradicted"
)

type Importance string

const (
	ImportanceLow      Importance = "low"
	ImportanceMedium   Importance = "medium"
	ImportanceHigh     Importance = "high"
	ImportanceCritical Importance = "critical"
)

type Urgency string

const (
	UrgencyLow      Urgency = "low"
	UrgencyNormal   Urgency = "normal"
	UrgencyHigh     Urgency = "high"
	UrgencyCritical Urgency = "critical"
)

type PublicStatus string

const (
	PublicPrivate PublicStatus = "private"
	PublicPublic  PublicStatus = "public"
)

type Foreshadow struct {
	ID                  string               `json:"id"`
	ProjectID           string               `json:"project_id"`
	Title               string               `json:"title"`
	Description         string               `json:"description"`
	Importance          Importance           `json:"importance"`
	PlantedChapter      int                  `json:"planted_chapter"`
	ExpectedPayoffMin   int                  `json:"expected_payoff_min"`
	ExpectedPayoffMax   int                  `json:"expected_payoff_max"`
	ActualPayoff        *int                 `json:"actual_payoff,omitempty"`
	Status              Status               `json:"status"`
	RelatedEntities     []string             `json:"related_entities"`
	RelatedArcs         []string             `json:"related_arcs"`
	LastProgressChapter int                  `json:"last_progress_chapter"`
	Urgency             Urgency              `json:"urgency"`
	SourceVersion       string               `json:"source_version"`
	Authority           truthstore.Authority `json:"authority"`
	CreatedAt           time.Time            `json:"created_at"`
	UpdatedAt           time.Time            `json:"updated_at"`
	Overdue             bool                 `json:"overdue"`
	OverdueByChapters   int                  `json:"overdue_by_chapters"`
}

type ForeshadowInput struct {
	ID                  string               `json:"id,omitempty"`
	Title               string               `json:"title"`
	Description         string               `json:"description"`
	Importance          Importance           `json:"importance"`
	PlantedChapter      int                  `json:"planted_chapter"`
	ExpectedPayoffMin   int                  `json:"expected_payoff_min"`
	ExpectedPayoffMax   int                  `json:"expected_payoff_max"`
	ActualPayoff        *int                 `json:"actual_payoff,omitempty"`
	Status              Status               `json:"status"`
	RelatedEntities     []string             `json:"related_entities"`
	RelatedArcs         []string             `json:"related_arcs"`
	LastProgressChapter int                  `json:"last_progress_chapter"`
	Urgency             Urgency              `json:"urgency"`
	SourceVersion       string               `json:"source_version"`
	Authority           truthstore.Authority `json:"authority"`
}

type ForeshadowPatch struct {
	Title               *string     `json:"title,omitempty"`
	Description         *string     `json:"description,omitempty"`
	Importance          *Importance `json:"importance,omitempty"`
	PlantedChapter      *int        `json:"planted_chapter,omitempty"`
	ExpectedPayoffMin   *int        `json:"expected_payoff_min,omitempty"`
	ExpectedPayoffMax   *int        `json:"expected_payoff_max,omitempty"`
	ActualPayoff        *int        `json:"actual_payoff,omitempty"`
	ClearActualPayoff   bool        `json:"clear_actual_payoff,omitempty"`
	RelatedEntities     *[]string   `json:"related_entities,omitempty"`
	RelatedArcs         *[]string   `json:"related_arcs,omitempty"`
	LastProgressChapter *int        `json:"last_progress_chapter,omitempty"`
	Urgency             *Urgency    `json:"urgency,omitempty"`
	SourceVersion       *string     `json:"source_version,omitempty"`
	Status              *Status     `json:"status,omitempty"`
	Chapter             int         `json:"chapter"`
	Reason              string      `json:"reason"`
}

type ForeshadowQuery struct {
	CurrentChapter int
	Status         Status
	Overdue        *bool
	Importance     Importance
	Urgency        Urgency
	Arc            string
	Entity         string
	Query          string
	Limit          int
	Offset         int
}

type ForeshadowPage struct {
	Foreshadows []Foreshadow `json:"foreshadows"`
	Total       int          `json:"total"`
	Limit       int          `json:"limit"`
	Offset      int          `json:"offset"`
	NextOffset  *int         `json:"next_offset,omitempty"`
}

type Secret struct {
	ID                string               `json:"id"`
	ProjectID         string               `json:"project_id"`
	Description       string               `json:"description"`
	Truth             string               `json:"truth,omitempty"`
	CreatedChapter    int                  `json:"created_chapter"`
	RevealedChapter   *int                 `json:"revealed_chapter,omitempty"`
	PublicStatus      PublicStatus         `json:"public_status"`
	RelatedForeshadow string               `json:"related_foreshadow,omitempty"`
	SourceVersion     string               `json:"source_version"`
	Authority         truthstore.Authority `json:"authority"`
	Holders           []KnowledgeHolder    `json:"holders"`
	PublicAtChapter   bool                 `json:"public_at_chapter"`
	CreatedAt         time.Time            `json:"created_at"`
	UpdatedAt         time.Time            `json:"updated_at"`
}

type SecretInput struct {
	ID                string               `json:"id,omitempty"`
	Description       string               `json:"description"`
	Truth             string               `json:"truth"`
	CreatedChapter    int                  `json:"created_chapter"`
	RevealedChapter   *int                 `json:"revealed_chapter,omitempty"`
	PublicStatus      PublicStatus         `json:"public_status"`
	RelatedForeshadow string               `json:"related_foreshadow,omitempty"`
	SourceVersion     string               `json:"source_version"`
	Authority         truthstore.Authority `json:"authority"`
	Holders           []HolderInput        `json:"holders,omitempty"`
}

type SecretPatch struct {
	Description       *string       `json:"description,omitempty"`
	Truth             *string       `json:"truth,omitempty"`
	RevealedChapter   *int          `json:"revealed_chapter,omitempty"`
	ClearReveal       bool          `json:"clear_revealed_chapter,omitempty"`
	PublicStatus      *PublicStatus `json:"public_status,omitempty"`
	RelatedForeshadow *string       `json:"related_foreshadow,omitempty"`
	SourceVersion     *string       `json:"source_version,omitempty"`
	Chapter           int           `json:"chapter"`
	Reason            string        `json:"reason"`
}

type KnowledgeHolder struct {
	SecretID         string               `json:"secret_id"`
	EntityID         string               `json:"entity_id"`
	ValidFromChapter int                  `json:"valid_from_chapter"`
	ValidToChapter   *int                 `json:"valid_to_chapter,omitempty"`
	SourceVersion    string               `json:"source_version"`
	Authority        truthstore.Authority `json:"authority"`
	Provenance       truthstore.Source    `json:"provenance"`
}

type HolderInput struct {
	EntityID         string               `json:"entity_id"`
	ValidFromChapter int                  `json:"valid_from_chapter"`
	ValidToChapter   *int                 `json:"valid_to_chapter,omitempty"`
	SourceVersion    string               `json:"source_version"`
	Authority        truthstore.Authority `json:"authority"`
	Provenance       truthstore.Source    `json:"provenance"`
}

type SecretQuery struct {
	CurrentChapter int
	PublicStatus   PublicStatus
	Holder         string
	Query          string
	IncludeTruth   bool
	Limit          int
	Offset         int
}

type SecretPage struct {
	Secrets    []Secret `json:"secrets"`
	Total      int      `json:"total"`
	Limit      int      `json:"limit"`
	Offset     int      `json:"offset"`
	NextOffset *int     `json:"next_offset,omitempty"`
}

type Dashboard struct {
	Chapter                   int `json:"chapter"`
	ActiveForeshadows         int `json:"active_foreshadows"`
	OverdueCount              int `json:"overdue_count"`
	CriticalOverdue           int `json:"critical_overdue"`
	UpcomingPayoffs           int `json:"upcoming_payoffs"`
	UnrevealedSecrets         int `json:"unrevealed_secrets"`
	KnowledgeBoundaryWarnings int `json:"knowledge_boundary_warnings"`
}

type Diagnostic struct {
	ID        string          `json:"id"`
	Code      string          `json:"code"`
	Severity  string          `json:"severity"`
	ProjectID string          `json:"project"`
	Chapter   int             `json:"chapter"`
	Entity    string          `json:"entity"`
	Message   string          `json:"message"`
	Retryable bool            `json:"retryable"`
	Evidence  json.RawMessage `json:"evidence"`
}

type PlannerItem struct {
	ID            string               `json:"id"`
	Kind          string               `json:"kind"`
	Title         string               `json:"title"`
	Summary       string               `json:"summary"`
	Mandatory     bool                 `json:"mandatory"`
	Importance    Importance           `json:"importance,omitempty"`
	Urgency       Urgency              `json:"urgency,omitempty"`
	SourceChapter int                  `json:"source_chapter"`
	SourceVersion string               `json:"source_version"`
	Authority     truthstore.Authority `json:"authority"`
	Metadata      map[string]any       `json:"metadata"`
}

type PlannerContext struct {
	ProjectID      string        `json:"project_id"`
	Chapter        int           `json:"chapter"`
	POV            string        `json:"pov,omitempty"`
	Foreshadows    []PlannerItem `json:"foreshadows"`
	KnownSecrets   []PlannerItem `json:"known_secrets"`
	UnknownSecrets []PlannerItem `json:"unknown_secret_boundaries"`
}

type AcceptedFinalInput struct {
	ProjectID         string
	TransactionID     string
	ProposalID        string
	CandidateID       string
	Chapter           int
	SourceVersion     string
	IdempotencyKey    string
	ForeshadowUpdates []AcceptedChange
	Secrets           []AcceptedChange
}

type AcceptedChange struct {
	Subject          string
	Predicate        string
	Object           json.RawMessage
	SourceChapter    int
	SourceVersion    string
	SourceSHA        string
	Extractor        string
	Confidence       float64
	Authority        truthstore.Authority
	ValidFromChapter int
	ValidToChapter   *int
	KnownFromChapter int
	KnownToChapter   *int
	Reason           string
}

// AcceptedFinalCommitter is the only production write boundary exposed to the
// Phase 5 chapter coordinator.
type AcceptedFinalCommitter interface {
	CommitAcceptedFinal(context.Context, AcceptedFinalInput) (CommitResult, error)
}

type CommitResult struct {
	CommitID        string `json:"commit_id"`
	TransactionID   string `json:"transaction_id"`
	ForeshadowCount int    `json:"foreshadow_count"`
	SecretCount     int    `json:"secret_count"`
	Replayed        bool   `json:"replayed"`
}

var (
	ErrNotFound            = errors.New("narrative ledger record not found")
	ErrValidation          = errors.New("narrative ledger validation failed")
	ErrIdempotencyConflict = errors.New("narrative ledger idempotency conflict")
	ErrStateConflict       = errors.New("narrative ledger state conflict")
)

func (f ForeshadowInput) Validate() error {
	if strings.TrimSpace(f.Title) == "" || len([]rune(f.Title)) > 200 || len([]rune(f.Description)) > 20_000 {
		return fmt.Errorf("%w: title and bounded description are required", ErrValidation)
	}
	if !validStatus(f.Status) || !validImportance(f.Importance) || !validUrgency(f.Urgency) {
		return fmt.Errorf("%w: status, importance or urgency is invalid", ErrValidation)
	}
	if f.PlantedChapter < 0 || f.ExpectedPayoffMin < f.PlantedChapter || f.ExpectedPayoffMax < f.ExpectedPayoffMin {
		return fmt.Errorf("%w: invalid payoff range", ErrValidation)
	}
	if f.LastProgressChapter < f.PlantedChapter {
		return fmt.Errorf("%w: last progress chapter precedes plant chapter", ErrValidation)
	}
	if f.ActualPayoff != nil {
		if f.Status != StatusResolved || *f.ActualPayoff < f.PlantedChapter {
			return fmt.Errorf("%w: actual payoff is only valid for a resolved foreshadow", ErrValidation)
		}
	}
	if strings.TrimSpace(f.SourceVersion) == "" || !validAuthority(f.Authority) {
		return fmt.Errorf("%w: source version and authority are required", ErrValidation)
	}
	if err := validateIDs(f.RelatedEntities, "related entity"); err != nil {
		return err
	}
	return validateIDs(f.RelatedArcs, "related arc")
}

func (s SecretInput) Validate() error {
	if strings.TrimSpace(s.Description) == "" || strings.TrimSpace(s.Truth) == "" || len([]rune(s.Description)) > 20_000 || len([]rune(s.Truth)) > 50_000 {
		return fmt.Errorf("%w: secret description and truth are required", ErrValidation)
	}
	if s.CreatedChapter < 0 || strings.TrimSpace(s.SourceVersion) == "" || !validAuthority(s.Authority) {
		return fmt.Errorf("%w: invalid secret provenance", ErrValidation)
	}
	if s.PublicStatus != PublicPrivate && s.PublicStatus != PublicPublic {
		return fmt.Errorf("%w: invalid public status", ErrValidation)
	}
	if s.RevealedChapter != nil && *s.RevealedChapter < s.CreatedChapter {
		return fmt.Errorf("%w: reveal precedes create", ErrValidation)
	}
	if s.PublicStatus == PublicPublic && s.RevealedChapter == nil {
		return fmt.Errorf("%w: public secret requires revealed chapter", ErrValidation)
	}
	for _, holder := range s.Holders {
		if err := holder.Validate(s.CreatedChapter); err != nil {
			return err
		}
	}
	return nil
}

func (h HolderInput) Validate(createdChapter int) error {
	if strings.TrimSpace(h.EntityID) == "" || h.ValidFromChapter < createdChapter || strings.TrimSpace(h.SourceVersion) == "" || !validAuthority(h.Authority) {
		return fmt.Errorf("%w: invalid secret holder provenance or range", ErrValidation)
	}
	if h.ValidToChapter != nil && *h.ValidToChapter < h.ValidFromChapter {
		return fmt.Errorf("%w: invalid secret holder range", ErrValidation)
	}
	if strings.TrimSpace(h.Provenance.Type) == "" || strings.TrimSpace(h.Provenance.ID) == "" || strings.TrimSpace(h.Provenance.Version) == "" {
		return fmt.Errorf("%w: holder provenance is required", ErrValidation)
	}
	return nil
}

func validStatus(value Status) bool {
	switch value {
	case StatusPlanned, StatusPlanted, StatusProgressing, StatusResolved, StatusAbandoned, StatusContradicted:
		return true
	}
	return false
}

func validImportance(value Importance) bool {
	switch value {
	case ImportanceLow, ImportanceMedium, ImportanceHigh, ImportanceCritical:
		return true
	}
	return false
}

func validUrgency(value Urgency) bool {
	switch value {
	case UrgencyLow, UrgencyNormal, UrgencyHigh, UrgencyCritical:
		return true
	}
	return false
}

func validAuthority(value truthstore.Authority) bool {
	switch value {
	case truthstore.AuthorityLLMSuggestion, truthstore.AuthorityStoryCompass,
		truthstore.AuthorityVolumePlan, truthstore.AuthorityArcPlan,
		truthstore.AuthorityChapterPlan, truthstore.AuthorityGeneratedFinal,
		truthstore.AuthorityHumanFinal:
		return true
	}
	return false
}

func validateIDs(values []string, label string) error {
	if len(values) > 100 {
		return fmt.Errorf("%w: at most 100 %ss are allowed", ErrValidation, label)
	}
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 200 {
			return fmt.Errorf("%w: invalid %s", ErrValidation, label)
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("%w: duplicate %s", ErrValidation, label)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func transitionAllowed(from, to Status) bool {
	if from == to && to == StatusProgressing {
		return true
	}
	switch from {
	case StatusPlanned:
		return to == StatusPlanted || to == StatusAbandoned
	case StatusPlanted:
		return to == StatusProgressing || to == StatusResolved || to == StatusAbandoned || to == StatusContradicted
	case StatusProgressing:
		return to == StatusProgressing || to == StatusResolved || to == StatusAbandoned || to == StatusContradicted
	case StatusContradicted:
		return to == StatusProgressing || to == StatusResolved || to == StatusAbandoned
	}
	return false
}
