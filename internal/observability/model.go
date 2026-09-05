// Package observability records bounded, non-secret model-attempt metadata.
// It is independent of story authority and never accepts or rewrites a chapter.
package observability

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid observation request")
var ErrConflict = errors.New("observation revision conflict")
var ErrNotFound = errors.New("observation not found")

type ControlError struct{ Code string }

func (e *ControlError) Error() string { return e.Code }
func ControlCode(err error) string {
	var c *ControlError
	if errors.As(err, &c) {
		return c.Code
	}
	return ""
}
func gate(code string) error { return &ControlError{Code: code} }

type Price struct {
	Provider               string `json:"provider"`
	Model                  string `json:"model"`
	InputMicrosPerMillion  int64  `json:"input_micros_per_million"`
	OutputMicrosPerMillion int64  `json:"output_micros_per_million"`
	Note                   string `json:"note,omitempty"`
}

// Monetary values are integer millionths of the configured currency. They are
// estimates from a user-maintained rate card, not claims about provider invoices.
type Policy struct {
	Currency            string   `json:"currency"`
	ProjectBudgetMicros int64    `json:"project_budget_micros"`
	TaskBudgetMicros    int64    `json:"task_budget_micros"`
	ProjectMaxCalls     int      `json:"project_max_calls"`
	TaskMaxCalls        int      `json:"task_max_calls"`
	MaxOutputTokens     int      `json:"max_output_tokens"`
	MaxInputEstimate    int      `json:"max_input_estimate"`
	RequireKnownPrice   bool     `json:"require_known_price"`
	BlockUnknownCost    bool     `json:"block_unknown_cost"`
	FailureThreshold    int      `json:"failure_threshold"`
	CooldownSeconds     int      `json:"cooldown_seconds"`
	PausedProviders     []string `json:"paused_providers"`
	Prices              []Price  `json:"prices"`
}

func DefaultPolicy() Policy {
	return Policy{Currency: "USD", MaxOutputTokens: 8192, MaxInputEstimate: 100000, FailureThreshold: 3, CooldownSeconds: 60, PausedProviders: []string{}, Prices: []Price{}}
}

var safeLabel = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)
var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

func Label(s string) string {
	if safeLabel.MatchString(s) && !strings.Contains(s, "://") {
		return s
	}
	if s == "" {
		return "unknown"
	}
	h := sha256.Sum256([]byte(s))
	return "label-" + hex.EncodeToString(h[:6])
}
func (p *Policy) Normalize() {
	if p.Prices == nil {
		p.Prices = []Price{}
	}
	if p.PausedProviders == nil {
		p.PausedProviders = []string{}
	}
}
func (p Policy) Validate() error {
	if !currencyPattern.MatchString(p.Currency) || p.ProjectBudgetMicros < 0 || p.TaskBudgetMicros < 0 || p.ProjectBudgetMicros > 1000000000000 || p.TaskBudgetMicros > 1000000000000 || p.ProjectMaxCalls < 0 || p.ProjectMaxCalls > 1000000 || p.TaskMaxCalls < 0 || p.TaskMaxCalls > 1000000 || p.MaxOutputTokens < 128 || p.MaxOutputTokens > 65536 || p.MaxInputEstimate < 256 || p.MaxInputEstimate > 1000000 || p.FailureThreshold < 1 || p.FailureThreshold > 20 || p.CooldownSeconds < 1 || p.CooldownSeconds > 86400 || len(p.Prices) > 100 || len(p.PausedProviders) > 100 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, r := range p.Prices {
		k := r.Provider + "\x00" + r.Model
		if Label(r.Provider) != r.Provider || Label(r.Model) != r.Model || seen[k] || r.InputMicrosPerMillion < 0 || r.OutputMicrosPerMillion < 0 || r.InputMicrosPerMillion > 1000000000 || r.OutputMicrosPerMillion > 1000000000 || len(r.Note) > 256 {
			return ErrInvalid
		}
		seen[k] = true
	}
	for _, v := range p.PausedProviders {
		if Label(v) != v {
			return ErrInvalid
		}
	}
	return nil
}
func (p Policy) price(provider, model string) *Price {
	for _, r := range p.Prices {
		if r.Provider == provider && r.Model == model {
			copy := r
			return &copy
		}
	}
	return nil
}
func estimateCost(input, output int, price *Price) *int64 {
	if price == nil || input < 0 || output < 0 {
		return nil
	}
	n := new(big.Int).Mul(big.NewInt(int64(input)), big.NewInt(price.InputMicrosPerMillion))
	n.Add(n, new(big.Int).Mul(big.NewInt(int64(output)), big.NewInt(price.OutputMicrosPerMillion)))
	n.Add(n, big.NewInt(999999))
	n.Div(n, big.NewInt(1000000))
	if !n.IsInt64() {
		return nil
	}
	v := n.Int64()
	return &v
}

type Identity struct {
	LogicalKey    string
	RequestHash   string
	TransactionID string
	TaskID        string
	Chapter       int
	Agent         string
	Operation     string
}
type Attempt struct {
	ID             string     `json:"id"`
	LogicalID      string     `json:"logical_id"`
	RequestHash    string     `json:"request_hash"`
	TaskID         string     `json:"task_id,omitempty"`
	TransactionID  string     `json:"transaction_id"`
	Chapter        int        `json:"chapter"`
	Agent          string     `json:"agent"`
	Operation      string     `json:"operation"`
	Provider       string     `json:"provider"`
	Model          string     `json:"model"`
	State          string     `json:"state"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at"`
	InputTokens    *int       `json:"input_tokens"`
	OutputTokens   *int       `json:"output_tokens"`
	UsageSource    string     `json:"usage_source"`
	InputEstimate  int        `json:"input_estimate"`
	OutputLimit    int        `json:"output_limit"`
	Currency       string     `json:"currency"`
	PriceRevision  int64      `json:"price_revision"`
	Price          *Price     `json:"price"`
	CostMicros     *int64     `json:"cost_micros"`
	ReservedMicros int64      `json:"reserved_micros"`
	CostSource     string     `json:"cost_source"`
	ErrorCode      string     `json:"error_code,omitempty"`
	Boundary       string     `json:"boundary"`
}
type State struct {
	Revision int64  `json:"revision"`
	Policy   Policy `json:"policy"`
}
type Totals struct {
	Calls          int   `json:"calls"`
	Completed      int   `json:"completed"`
	Failed         int   `json:"failed"`
	Pending        int   `json:"pending"`
	UnknownCost    int   `json:"unknown_cost"`
	UnknownUsage   int   `json:"unknown_usage"`
	InputTokens    int64 `json:"input_tokens"`
	OutputTokens   int64 `json:"output_tokens"`
	CostMicros     int64 `json:"known_cost_micros"`
	ReservedMicros int64 `json:"reserved_micros"`
}
type Group struct {
	Agent    string `json:"agent"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Totals   Totals `json:"totals"`
}
type Page struct {
	Health        []Health  `json:"health"`
	State         State     `json:"settings"`
	Totals        Totals    `json:"totals"`
	Groups        []Group   `json:"groups"`
	Attempts      []Attempt `json:"attempts"`
	Replays       int       `json:"replays"`
	LegacyCalls   int       `json:"legacy_untracked_calls"`
	Limit         int       `json:"limit"`
	Offset        int       `json:"offset"`
	Total         int       `json:"total"`
	FilterTask    string    `json:"filter_task"`
	FilterChapter int       `json:"filter_chapter"`
}

type Store struct {
	DB        *sql.DB
	ProjectID string
	Notify    func(string, string)
	Now       func() time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func opaqueID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "obs_" + hex.EncodeToString(b), nil
}
func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

type taskKey struct{}
type callKey struct{}
type callContext struct {
	Store    *Store
	Identity Identity
}

func WithTask(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, taskKey{}, id)
}
func WithCall(ctx context.Context, s *Store, id Identity) context.Context {
	if task, _ := ctx.Value(taskKey{}).(string); task != "" {
		id.TaskID = task
	}
	return context.WithValue(ctx, callKey{}, callContext{s, id})
}

type Ticket struct {
	store   *Store
	Attempt Attempt
}

func (t *Ticket) OutputLimit() int {
	if t == nil {
		return 0
	}
	return t.Attempt.OutputLimit
}

// Start is called immediately before an SDK Generate attempt, including fallback.
func Start(ctx context.Context, provider, model string, inputEstimate int, boundary string) (*Ticket, error) {
	c, ok := ctx.Value(callKey{}).(callContext)
	if !ok || c.Store == nil {
		return nil, nil
	}
	return c.Store.begin(ctx, c.Identity, provider, model, inputEstimate, boundary)
}

// Finish deliberately detaches cancellation only for bounded metadata persistence.
// Failure leaves a pending reservation, so a subsequent paid call is blocked.
func (t *Ticket) Finish(ctx context.Context, input, output int, known bool, code string) error {
	if t == nil {
		return nil
	}
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	a := t.Attempt
	ended := t.store.now()
	a.EndedAt = &ended
	a.State = "completed"
	a.ErrorCode = code
	if code != "" {
		a.State = "failed"
	}
	if known && input >= 0 && output >= 0 && input <= 100000000 && output <= 100000000 {
		a.InputTokens = &input
		a.OutputTokens = &output
		a.UsageSource = "provider"
		a.CostMicros = estimateCost(input, output, a.Price)
		if a.CostMicros != nil {
			a.CostSource = "rate_card_estimate"
		}
	} else {
		a.UsageSource = "unknown"
	}
	a.ReservedMicros = 0
	raw, err := json.Marshal(a)
	if err != nil {
		return gate("OBSERVATION_STORAGE_FAILED")
	}
	result, err := t.store.DB.ExecContext(persist, `UPDATE observation_attempts SET state=?,cost_micros=?,reserved_micros=0,error_code=?,ended_at=?,payload_json=? WHERE id=? AND project_id=? AND state='pending'`, a.State, a.CostMicros, code, ended.Format(time.RFC3339Nano), string(raw), a.ID, t.store.ProjectID)
	if err != nil {
		return gate("OBSERVATION_STORAGE_FAILED")
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return gate("OBSERVATION_STORAGE_FAILED")
	}
	if t.store.Notify != nil {
		t.store.Notify("attempt_finished", a.ID)
	}
	return nil
}
