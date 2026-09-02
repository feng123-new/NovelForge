package qualitygate

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type TransactionState string

const (
	StatePlanned            TransactionState = "planned"
	StateDrafting           TransactionState = "drafting"
	StateDraftReady         TransactionState = "draft_ready"
	StateLibrarianPending   TransactionState = "librarian_pending"
	StateFactsProposed      TransactionState = "facts_proposed"
	StateContinuityPending  TransactionState = "continuity_pending"
	StateContinuityPass     TransactionState = "continuity_pass"
	StateContinuityWarn     TransactionState = "continuity_warn"
	StateContinuityFail     TransactionState = "continuity_fail"
	StateEditorPending      TransactionState = "editor_pending"
	StateReviewed           TransactionState = "reviewed"
	StateRewritePending     TransactionState = "rewrite_pending"
	StateFinalCandidate     TransactionState = "final_candidate"
	StateTruthCommitPending TransactionState = "truth_commit_pending"
	StateCheckpointPending  TransactionState = "checkpoint_pending"
	StateCompleted          TransactionState = "completed"
	StateHold               TransactionState = "hold"
	StateFailed             TransactionState = "failed"
)

type ContinuityStatus string

const (
	ContinuityPass ContinuityStatus = "PASS"
	ContinuityWarn ContinuityStatus = "WARN"
	ContinuityFail ContinuityStatus = "FAIL"
)

type Severity string

const (
	SeverityInfo     Severity = "INFO"
	SeverityWarning  Severity = "WARNING"
	SeverityBlocking Severity = "BLOCKING"
)

type ChapterPlan struct {
	Chapter               int      `json:"chapter"`
	Title                 string   `json:"title"`
	POV                   string   `json:"pov"`
	Location              string   `json:"location"`
	Objective             string   `json:"objective"`
	Conflict              string   `json:"conflict"`
	RequiredBeats         []string `json:"required_beats"`
	ForbiddenOutcomes     []string `json:"forbidden_outcomes"`
	KnowledgeBoundary     []string `json:"knowledge_boundary"`
	InventoryConstraints  []string `json:"inventory_constraints"`
	ForeshadowObligations []string `json:"foreshadow_obligations"`
	EndingHook            string   `json:"ending_hook"`
}

type FactChange struct {
	Subject           string          `json:"subject"`
	Predicate         string          `json:"predicate"`
	Object            json.RawMessage `json:"object"`
	SourceChapter     int             `json:"source_chapter"`
	SourceVersion     string          `json:"source_version"`
	SourceSHA         string          `json:"source_sha"`
	Extractor         string          `json:"extractor"`
	Confidence        float64         `json:"confidence"`
	ProposedAuthority string          `json:"proposed_authority"`
	ValidFromChapter  int             `json:"valid_from_chapter"`
	ValidToChapter    *int            `json:"valid_to_chapter"`
	KnownFromChapter  int             `json:"known_from_chapter"`
	KnownToChapter    *int            `json:"known_to_chapter"`
	Reason            string          `json:"reason"`
}

type FactProposal struct {
	ProposalID          string       `json:"proposal_id"`
	ProjectID           string       `json:"project_id"`
	Chapter             int          `json:"chapter"`
	SourceVersion       string       `json:"source_version"`
	SourceSHA           string       `json:"source_sha"`
	Extractor           string       `json:"extractor"`
	Authority           string       `json:"authority"`
	EntityChanges       []FactChange `json:"entity_changes"`
	CharacterChanges    []FactChange `json:"character_changes"`
	RelationshipChanges []FactChange `json:"relationship_changes"`
	LocationChanges     []FactChange `json:"location_changes"`
	InventoryChanges    []FactChange `json:"inventory_changes"`
	KnowledgeChanges    []FactChange `json:"knowledge_changes"`
	TimelineEvents      []FactChange `json:"timeline_events"`
	WorldFacts          []FactChange `json:"world_facts"`
	ForeshadowUpdates   []FactChange `json:"foreshadow_updates"`
	Secrets             []FactChange `json:"secrets"`
	Injuries            []FactChange `json:"injuries"`
	CultivationChanges  []FactChange `json:"cultivation_changes"`
	Diagnostics         []string     `json:"diagnostics"`
}

func (p FactProposal) AllChanges() []FactChange {
	groups := [][]FactChange{p.EntityChanges, p.CharacterChanges, p.RelationshipChanges, p.LocationChanges, p.InventoryChanges, p.KnowledgeChanges, p.TimelineEvents, p.WorldFacts, p.ForeshadowUpdates, p.Secrets, p.Injuries, p.CultivationChanges}
	var out []FactChange
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}

func (p FactProposal) Validate() error {
	if strings.TrimSpace(p.ProposalID) == "" || strings.TrimSpace(p.ProjectID) == "" || p.Chapter < 0 || strings.TrimSpace(p.SourceVersion) == "" || strings.TrimSpace(p.SourceSHA) == "" || strings.TrimSpace(p.Extractor) == "" {
		return errors.New("proposal metadata is incomplete")
	}
	for i, change := range p.AllChanges() {
		if err := change.Validate(p.Chapter, p.SourceVersion, p.SourceSHA, p.Extractor); err != nil {
			return fmt.Errorf("fact change %d: %w", i, err)
		}
	}
	return nil
}

func (c FactChange) Validate(chapter int, version, sha, extractor string) error {
	if strings.TrimSpace(c.Subject) == "" || strings.TrimSpace(c.Predicate) == "" || len(c.Object) == 0 || !json.Valid(c.Object) {
		return errors.New("subject, predicate and valid JSON object are required")
	}
	if c.SourceChapter != chapter || c.SourceVersion != version || c.SourceSHA != sha || c.Extractor != extractor {
		return errors.New("fact provenance must match proposal source")
	}
	if c.Confidence < 0 || c.Confidence > 1 || strings.TrimSpace(c.ProposedAuthority) == "" || strings.TrimSpace(c.Reason) == "" {
		return errors.New("confidence, proposed authority and reason are required")
	}
	if c.ValidFromChapter < 0 || c.KnownFromChapter < 0 || c.ValidToChapter != nil && *c.ValidToChapter < c.ValidFromChapter || c.KnownToChapter != nil && *c.KnownToChapter < c.KnownFromChapter {
		return errors.New("invalid temporal boundary")
	}
	return nil
}

type ContinuityIssue struct {
	IssueCode       string          `json:"issue_code"`
	Severity        Severity        `json:"severity"`
	Entity          string          `json:"entity"`
	Predicate       string          `json:"predicate"`
	Expected        json.RawMessage `json:"expected"`
	Actual          json.RawMessage `json:"actual"`
	Evidence        string          `json:"evidence"`
	SourceChapter   int             `json:"source_chapter"`
	SourceVersion   string          `json:"source_version"`
	SuggestedAction string          `json:"suggested_action"`
}

type ContinuityResult struct {
	Status   ContinuityStatus  `json:"status"`
	Blocking bool              `json:"blocking"`
	Issues   []ContinuityIssue `json:"issues"`
}

func (r ContinuityResult) Validate() error {
	if r.Status != ContinuityPass && r.Status != ContinuityWarn && r.Status != ContinuityFail {
		return errors.New("continuity status must be PASS, WARN or FAIL")
	}
	if r.Status == ContinuityFail && !r.Blocking {
		return errors.New("FAIL must be blocking")
	}
	if r.Status == ContinuityPass && r.Blocking {
		return errors.New("PASS cannot be blocking")
	}
	for _, issue := range r.Issues {
		if strings.TrimSpace(issue.IssueCode) == "" || strings.TrimSpace(issue.Predicate) == "" {
			return errors.New("continuity issue is incomplete")
		}
	}
	return nil
}

type EditorReview struct {
	Score              float64  `json:"score"`
	Strengths          []string `json:"strengths"`
	Weaknesses         []string `json:"weaknesses"`
	LineLevelIssues    []string `json:"line_level_issues"`
	Pacing             string   `json:"pacing"`
	Characterization   string   `json:"characterization"`
	Prose              string   `json:"prose"`
	Dialogue           string   `json:"dialogue"`
	Ending             string   `json:"ending"`
	RewriteRecommended bool     `json:"rewrite_recommended"`
	Summary            string   `json:"summary"`
}

func (r EditorReview) Validate() error {
	if r.Score < 0 || r.Score > 10 {
		return errors.New("editor score must be between 0 and 10")
	}
	return nil
}

type StateChange struct {
	TransactionID string           `json:"transaction_id"`
	Chapter       int              `json:"chapter"`
	FromState     TransactionState `json:"from_state"`
	ToState       TransactionState `json:"to_state"`
	Reason        string           `json:"reason"`
	Actor         string           `json:"actor"`
	Attempt       int              `json:"attempt"`
	CreatedAt     time.Time        `json:"created_at"`
}

type Candidate struct {
	ID               string           `json:"id"`
	TransactionID    string           `json:"transaction_id"`
	Chapter          int              `json:"chapter"`
	Attempt          int              `json:"attempt"`
	Text             string           `json:"-"`
	TextSHA          string           `json:"text_sha"`
	SourceVersion    string           `json:"source_version"`
	ContinuityStatus ContinuityStatus `json:"continuity_status"`
	EditorScore      *float64         `json:"editor_score"`
	Selected         bool             `json:"selected"`
	SelectionReason  string           `json:"selection_reason"`
	CreatedAt        time.Time        `json:"created_at"`
}

type Transaction struct {
	ID               string           `json:"transaction_id"`
	ProjectID        string           `json:"project_id"`
	Chapter          int              `json:"chapter"`
	State            TransactionState `json:"state"`
	Attempt          int              `json:"attempt"`
	MaxRewrites      int              `json:"max_rewrites"`
	QualityThreshold float64          `json:"quality_threshold"`
	FinalCandidateID string           `json:"final_candidate_id,omitempty"`
	HoldReason       string           `json:"hold_reason,omitempty"`
	LastReason       string           `json:"last_reason,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

type ModelCall struct {
	ID             string    `json:"id"`
	IdempotencyKey string    `json:"idempotency_key"`
	ProjectID      string    `json:"project"`
	Chapter        int       `json:"chapter"`
	TransactionID  string    `json:"transaction"`
	Agent          string    `json:"agent"`
	Operation      string    `json:"operation"`
	Provider       string    `json:"provider"`
	Model          string    `json:"model"`
	RequestHash    string    `json:"request_hash"`
	ResponseHash   string    `json:"response_hash"`
	Status         string    `json:"status"`
	Attempt        int       `json:"attempt"`
	InputTokens    int       `json:"input_tokens"`
	OutputTokens   int       `json:"output_tokens"`
	StartedAt      time.Time `json:"started_at"`
	EndedAt        time.Time `json:"ended_at"`
	ErrorCode      string    `json:"error_code,omitempty"`
}

type Snapshot struct {
	Transaction Transaction       `json:"transaction"`
	Candidates  []Candidate       `json:"candidates"`
	Proposal    *FactProposal     `json:"proposal,omitempty"`
	Continuity  *ContinuityResult `json:"continuity,omitempty"`
	Editor      *EditorReview     `json:"editor,omitempty"`
	States      []StateChange     `json:"state_changes"`
}
