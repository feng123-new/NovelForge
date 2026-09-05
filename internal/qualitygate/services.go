package qualitygate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/voocel/ainovel-cli/internal/observability"
	"strings"
	"time"
)

type ArchitectService interface {
	BuildArchitecture(context.Context, string) (json.RawMessage, error)
}

type PlannerService interface {
	PlanChapter(context.Context, string, int) (ChapterPlan, error)
}

type WriterRequest struct {
	ProjectID     string
	Chapter       int
	TransactionID string
	Attempt       int
	Plan          ChapterPlan
	PreviousDraft string
	Feedback      []string
}

type WriterResult struct {
	Text          string
	SourceVersion string
}

type WriterService interface {
	Write(context.Context, WriterRequest) (WriterResult, error)
}

type LibrarianRequest struct {
	ProjectID     string
	Chapter       int
	TransactionID string
	Candidate     Candidate
}

type LibrarianService interface {
	Propose(context.Context, LibrarianRequest) (FactProposal, error)
}

type ContinuityRequest struct {
	ProjectID     string
	Chapter       int
	TransactionID string
	Candidate     Candidate
	Proposal      FactProposal
}

type ContinuityService interface {
	Check(context.Context, ContinuityRequest) (ContinuityResult, error)
}

type EditorRequest struct {
	ProjectID     string
	Chapter       int
	TransactionID string
	Candidate     Candidate
	Continuity    ContinuityResult
}

type EditorService interface {
	Review(context.Context, EditorRequest) (EditorReview, error)
}

type StructuredOutputDecoder interface {
	Decode(context.Context, string, []byte, any) (repairs int, err error)
}

type ModelCallRepository interface {
	GetModelCall(context.Context, string) (ModelCall, string, error)
	SaveModelCall(context.Context, ModelCall, string) error
}

type ChapterTransactionRepository interface {
	Begin(context.Context, string, int, Policy) (Transaction, bool, error)
	Transaction(context.Context, string) (Transaction, error)
	Transition(context.Context, string, TransactionState, string, string, int) (Transaction, error)
	SaveCandidate(context.Context, string, string, string, int) (Candidate, error)
	SaveProposal(context.Context, string, string, FactProposal) error
	SaveContinuity(context.Context, string, string, ContinuityResult) error
	SaveEditor(context.Context, string, string, EditorReview) error
	SelectFinal(context.Context, string, string, string) error
	BestSafeCandidate(context.Context, string, Policy) (Candidate, string, error)
	Snapshot(context.Context, string, int) (Snapshot, error)
}

type ChapterCommitCoordinator interface {
	Finalize(context.Context, string, int, string) (Snapshot, error)
}

type QualityPolicy interface {
	Policy() Policy
	Allows(ContinuityResult) bool
}

type StaticQualityPolicy struct{ Value Policy }

func (p StaticQualityPolicy) Policy() Policy {
	if p.Value.MaxRewrites == 0 && p.Value.QualityThreshold == 0 {
		return DefaultPolicy()
	}
	return p.Value
}

func (p StaticQualityPolicy) Allows(result ContinuityResult) bool { return p.Policy().Allows(result) }

type ModelUsage struct {
	Provider     string
	Model        string
	InputTokens  int
	OutputTokens int
}

type ModelInvoker interface {
	Invoke(context.Context, string, []byte) ([]byte, ModelUsage, error)
}

type ModelCallError struct {
	Code      string
	Retryable bool
	Err       error
}

func (e *ModelCallError) Error() string {
	if e == nil || e.Err == nil {
		return "model call failed"
	}
	return e.Err.Error()
}

func (e *ModelCallError) Unwrap() error { return e.Err }

type IdempotentModelCaller struct {
	Observer   *observability.Store
	Repository ModelCallRepository
	Invoker    ModelInvoker
	Now        func() time.Time
	NewID      func() string
}

type CallRequest struct {
	IdempotencyKey string
	ProjectID      string
	Chapter        int
	TransactionID  string
	Agent          string
	Operation      string
	Attempt        int
	Payload        []byte
}

func (c IdempotentModelCaller) Call(ctx context.Context, request CallRequest) ([]byte, ModelCall, bool, error) {
	if c.Repository == nil || c.Invoker == nil {
		return nil, ModelCall{}, false, errors.New("model caller is not configured")
	}
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.IdempotencyKey == "" || request.TransactionID == "" || request.Agent == "" || request.Operation == "" {
		return nil, ModelCall{}, false, errors.New("model call identity is incomplete")
	}
	hash := sha256.Sum256(request.Payload)
	requestHash := hex.EncodeToString(hash[:])
	if existing, response, err := c.Repository.GetModelCall(ctx, request.IdempotencyKey); err == nil {
		if existing.RequestHash != requestHash {
			return nil, existing, false, ErrIdempotencyConflict
		}
		if existing.Status == "completed" {
			if c.Observer != nil {
				c.Observer.Replay(ctx, request.IdempotencyKey)
			}
			return []byte(response), existing, true, nil
		}
	} else if !errors.Is(err, ErrNotFound) {
		return nil, ModelCall{}, false, err
	}
	now := c.Now
	if now == nil {
		now = time.Now
	}
	newID := c.NewID
	if newID == nil {
		newID = func() string { return "call_" + requestHash[:24] }
	}
	if c.Observer != nil {
		ctx = observability.WithCall(ctx, c.Observer, observability.Identity{LogicalKey: request.IdempotencyKey, RequestHash: requestHash, TransactionID: request.TransactionID, Chapter: request.Chapter, Agent: request.Agent, Operation: request.Operation})
	}
	started := now().UTC()
	response, usage, invokeErr := c.Invoker.Invoke(ctx, request.Agent+":"+request.Operation, request.Payload)
	ended := now().UTC()
	if observability.ControlCode(invokeErr) != "" {
		return response, ModelCall{}, false, invokeErr
	}
	responseHash := ""
	if len(response) > 0 {
		sum := sha256.Sum256(response)
		responseHash = hex.EncodeToString(sum[:])
	}
	call := ModelCall{
		ID:             newID(),
		IdempotencyKey: request.IdempotencyKey,
		ProjectID:      request.ProjectID,
		Chapter:        request.Chapter,
		TransactionID:  request.TransactionID,
		Agent:          request.Agent,
		Operation:      request.Operation,
		Provider:       usage.Provider,
		Model:          usage.Model,
		RequestHash:    requestHash,
		ResponseHash:   responseHash,
		Status:         "completed",
		Attempt:        request.Attempt,
		InputTokens:    usage.InputTokens,
		OutputTokens:   usage.OutputTokens,
		StartedAt:      started,
		EndedAt:        ended,
	}
	if invokeErr != nil {
		call.Status = "failed"
		var modelErr *ModelCallError
		if errors.As(invokeErr, &modelErr) {
			call.ErrorCode = modelErr.Code
		} else {
			call.ErrorCode = "MODEL_CALL_FAILED"
		}
	}
	if err := c.Repository.SaveModelCall(ctx, call, string(response)); err != nil {
		if existing, stored, getErr := c.Repository.GetModelCall(ctx, request.IdempotencyKey); getErr == nil {
			if existing.RequestHash != requestHash {
				return nil, existing, false, ErrIdempotencyConflict
			}
			return []byte(stored), existing, true, invokeErr
		}
		return nil, ModelCall{}, false, fmt.Errorf("record model call: %w", err)
	}
	return response, call, false, invokeErr
}

// RetryingModelInvoker bounds transient provider/network retries. It deliberately
// retries only errors explicitly marked retryable and never loops indefinitely.
type RetryingModelInvoker struct {
	Invoker    ModelInvoker
	MaxRetries int
}

func (r RetryingModelInvoker) Invoke(ctx context.Context, operation string, payload []byte) ([]byte, ModelUsage, error) {
	if r.Invoker == nil {
		return nil, ModelUsage{}, errors.New("model invoker is required")
	}
	maxRetries := r.MaxRetries
	if maxRetries < 0 {
		return nil, ModelUsage{}, errors.New("max model retries must not be negative")
	}
	if maxRetries > 5 {
		maxRetries = 5
	}
	var response []byte
	var usage ModelUsage
	var err error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, usage, ctxErr
		}
		response, usage, err = r.Invoker.Invoke(ctx, operation, payload)
		if err == nil {
			return response, usage, nil
		}
		var modelErr *ModelCallError
		if !errors.As(err, &modelErr) || !modelErr.Retryable {
			return response, usage, err
		}
	}
	return response, usage, err
}
