package chapterversion

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/narrativeledger"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
	"github.com/voocel/ainovel-cli/internal/truthstore"
)

type FinalChapterWriter interface {
	WriteFinalChapter(context.Context, string, int, string, string) error
}

type FaultInjector interface {
	Fail(string) error
}

type CandidateConflict struct {
	SubjectType       string               `json:"subject_type"`
	SubjectID         string               `json:"subject_id"`
	Predicate         string               `json:"predicate"`
	ExistingEventID   string               `json:"existing_event_id"`
	ExistingValue     json.RawMessage      `json:"existing_value"`
	ExistingAuthority truthstore.Authority `json:"existing_authority"`
	ProposedValue     json.RawMessage      `json:"proposed_value"`
	Blocking          bool                 `json:"blocking"`
	Reason            string               `json:"reason"`
}

type Evaluation struct {
	Proposal    qualitygate.FactProposal     `json:"proposal"`
	Continuity qualitygate.ContinuityResult `json:"continuity"`
	Review     *qualitygate.EditorReview    `json:"review,omitempty"`
	Conflicts  []CandidateConflict          `json:"conflicts"`
	EvaluatedAt time.Time                   `json:"evaluated_at"`
}

type Coordinator struct {
	Store       *Store
	Truth       truthstore.Repository
	Ledger      *narrativeledger.Store
	Librarian   qualitygate.LibrarianService
	Continuity  qualitygate.ContinuityService
	Editor      qualitygate.EditorService
	FinalWriter FinalChapterWriter
	Faults      FaultInjector
}

func (c *Coordinator) validate() error {
	if c == nil || c.Store == nil {
		return newError(CodeStorage, "chapter version coordinator is not configured", false, nil)
	}
	return nil
}

func jsonEqual(left, right json.RawMessage) bool {
	var leftValue any
	var rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return bytes.Equal(bytes.TrimSpace(left), bytes.TrimSpace(right))
	}
	leftJSON, _ := json.Marshal(leftValue)
	rightJSON, _ := json.Marshal(rightValue)
	return bytes.Equal(leftJSON, rightJSON)
}

func splitSubject(subject string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(subject), ":", 2)
	if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "entity", strings.TrimSpace(subject)
}

func mustJSON(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
