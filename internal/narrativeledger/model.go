package narrativeledger

import "time"

// Authority identifies who is allowed to create an authoritative ledger event.
type Authority string

const (
	AuthorityAcceptedFinal Authority = "accepted_final"
	AuthorityHuman         Authority = "human"
	AuthorityRetrieval     Authority = "retrieval"
)

// Priority controls deterministic planner ordering.
type Priority string

const (
	PriorityCritical Priority = "critical"
	PriorityHigh     Priority = "high"
	PriorityNormal   Priority = "normal"
	PriorityLow      Priority = "low"
)

// ForeshadowStatus is the stored lifecycle state. OVERDUE is computed, never stored.
type ForeshadowStatus string

const (
	ForeshadowPlanned    ForeshadowStatus = "planned"
	ForeshadowPlanted    ForeshadowStatus = "planted"
	ForeshadowReinforced ForeshadowStatus = "reinforced"
	ForeshadowRevealed   ForeshadowStatus = "revealed"
	ForeshadowAbandoned  ForeshadowStatus = "abandoned"
	ForeshadowOverdue    ForeshadowStatus = "overdue"
)

// SecretStatus is the stored secret lifecycle state.
type SecretStatus string

const (
	SecretHidden   SecretStatus = "hidden"
	SecretHinted   SecretStatus = "hinted"
	SecretRevealed SecretStatus = "revealed"
	SecretRetired  SecretStatus = "retired"
)

// Source binds every ledger mutation to immutable provenance.
type Source struct {
	TransactionID string            `json:"transaction_id"`
	CandidateID   string            `json:"candidate_id,omitempty"`
	Chapter       int               `json:"chapter"`
	Authority     Authority         `json:"authority"`
	Provenance    map[string]string `json:"provenance,omitempty"`
}

// ForeshadowChange describes one deterministic lifecycle mutation.
type ForeshadowChange struct {
	Action         string            `json:"action"`
	Key            string            `json:"key"`
	Title          string            `json:"title,omitempty"`
	Description    string            `json:"description,omitempty"`
	Priority       *Priority         `json:"priority,omitempty"`
	Status         *ForeshadowStatus `json:"status,omitempty"`
	PlantedChapter *int              `json:"planted_chapter,omitempty"`
	DueChapter     *int              `json:"due_chapter,omitempty"`
	RevealChapter  *int              `json:"reveal_chapter,omitempty"`
}

// SecretKnowledgeChange opens or closes a holder's Chapter-N knowledge interval.
type SecretKnowledgeChange struct {
	Holder            string `json:"holder"`
	KnownFromChapter  int    `json:"known_from_chapter"`
	KnownUntilChapter *int   `json:"known_until_chapter,omitempty"`
}

// SecretChange describes one secret mutation and its temporal knowledge boundary.
type SecretChange struct {
	Action            string                  `json:"action"`
	Key               string                  `json:"key"`
	Title             string                  `json:"title,omitempty"`
	Description       string                  `json:"description,omitempty"`
	Status            *SecretStatus           `json:"status,omitempty"`
	PublicFromChapter *int                    `json:"public_from_chapter,omitempty"`
	Knowledge         []SecretKnowledgeChange `json:"knowledge,omitempty"`
}

// ChangeSet is the only accepted input for an authoritative chapter commit.
type ChangeSet struct {
	Source      Source             `json:"source"`
	Foreshadows []ForeshadowChange `json:"foreshadows,omitempty"`
	Secrets     []SecretChange     `json:"secrets,omitempty"`
}

// Commit records exactly one accepted source transaction.
type Commit struct {
	TransactionID string    `json:"transaction_id"`
	CandidateID   string    `json:"candidate_id,omitempty"`
	Chapter       int       `json:"chapter"`
	Authority     Authority `json:"authority"`
	ContentHash   string    `json:"content_hash"`
	CommittedAt   time.Time `json:"committed_at"`
}

// ApplyResult distinguishes an idempotent replay from a new durable commit.
type ApplyResult struct {
	Replay bool   `json:"replay"`
	Commit Commit `json:"commit"`
}

// Foreshadow is the Chapter-N view returned to callers.
type Foreshadow struct {
	ID               string            `json:"id"`
	Key              string            `json:"key"`
	Title            string            `json:"title"`
	Description      string            `json:"description,omitempty"`
	Priority         Priority          `json:"priority"`
	Status           ForeshadowStatus  `json:"status"`
	EffectiveStatus  ForeshadowStatus  `json:"effective_status"`
	PlantedChapter   *int              `json:"planted_chapter,omitempty"`
	DueChapter       *int              `json:"due_chapter,omitempty"`
	RevealChapter    *int              `json:"reveal_chapter,omitempty"`
	SourceTransaction string           `json:"source_transaction_id"`
	UpdatedChapter   int               `json:"updated_chapter"`
	CreatedAt        time.Time          `json:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at"`
}

// Secret is a redaction-aware Chapter-N secret view.
type Secret struct {
	ID                string       `json:"id"`
	Key               string       `json:"key"`
	Title             string       `json:"title"`
	Description       string       `json:"description,omitempty"`
	Status            SecretStatus `json:"status"`
	PublicFromChapter *int         `json:"public_from_chapter,omitempty"`
	Public            bool         `json:"public"`
	Holders           []string     `json:"holders"`
	SourceTransaction string       `json:"source_transaction_id"`
	UpdatedChapter    int          `json:"updated_chapter"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

// ListOptions applies bounded deterministic pagination.
type ListOptions struct {
	AsOfChapter int
	Status      string
	Priority    string
	Query       string
	Limit       int
	Offset      int
}

// ForeshadowPage is a stable page of foreshadows.
type ForeshadowPage struct {
	Items      []Foreshadow `json:"items"`
	Total      int          `json:"total"`
	Limit      int          `json:"limit"`
	Offset     int          `json:"offset"`
	NextOffset *int         `json:"next_offset,omitempty"`
}

// SecretPage is a stable Chapter-N page of secrets.
type SecretPage struct {
	Items      []Secret `json:"items"`
	Total      int      `json:"total"`
	Limit      int      `json:"limit"`
	Offset     int      `json:"offset"`
	NextOffset *int     `json:"next_offset,omitempty"`
}

// PlannerItem is an explicitly classified planner obligation.
type PlannerItem struct {
	Category  string   `json:"category"`
	Mandatory bool     `json:"mandatory"`
	Key       string   `json:"key"`
	Title     string   `json:"title"`
	Priority  Priority `json:"priority"`
	DueChapter *int    `json:"due_chapter,omitempty"`
}

// PlannerContext is Phase 6's deterministic ledger injection. Phase 7 owns token compilation.
type PlannerContext struct {
	Chapter int           `json:"chapter"`
	Items   []PlannerItem `json:"items"`
	Text    string        `json:"text"`
}

// Dashboard contains real counts derived from authoritative rows.
type Dashboard struct {
	Chapter              int `json:"chapter"`
	ForeshadowsTotal     int `json:"foreshadows_total"`
	ForeshadowsActive    int `json:"foreshadows_active"`
	ForeshadowsCritical  int `json:"foreshadows_critical"`
	ForeshadowsOverdue   int `json:"foreshadows_overdue"`
	ForeshadowsUpcoming  int `json:"foreshadows_upcoming"`
	SecretsTotal         int `json:"secrets_total"`
	SecretsPublic        int `json:"secrets_public"`
	SecretsHidden        int `json:"secrets_hidden"`
}

// Diagnostic exposes stable machine-readable ledger findings.
type Diagnostic struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	EntityType string `json:"entity_type"`
	EntityKey  string `json:"entity_key"`
	Chapter    int    `json:"chapter"`
	Message    string `json:"message"`
}
