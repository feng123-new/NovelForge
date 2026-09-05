package qualitygate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ModelWriterService adapts the bounded/idempotent model-call boundary to WriterService.
// It returns only prose; it has no Truth repository and cannot commit authoritative state.
type ModelWriterService struct {
	Caller        *IdempotentModelCaller
	Context       WriterContextCompiler
	ContextTokens int
}

func (s ModelWriterService) Write(ctx context.Context, req WriterRequest) (WriterResult, error) {
	if s.Caller == nil {
		return WriterResult{}, errors.New("model writer caller is required")
	}
	var compiled json.RawMessage
	if s.Context != nil {
		var err error
		compiled, err = s.Context.CompileWriterContext(ctx, req, s.ContextTokens)
		if err != nil {
			return WriterResult{}, fmt.Errorf("compile writer context: %w", err)
		}
	}
	payload, err := json.Marshal(struct {
		Plan            ChapterPlan     `json:"chapter_plan"`
		PreviousDraft   string          `json:"previous_draft,omitempty"`
		Feedback        []string        `json:"feedback,omitempty"`
		CompiledContext json.RawMessage `json:"compiled_context,omitempty"`
	}{req.Plan, req.PreviousDraft, req.Feedback, compiled})
	if err != nil {
		return WriterResult{}, err
	}
	response, call, _, err := s.Caller.Call(ctx, CallRequest{
		IdempotencyKey: fmt.Sprintf("%s:writer:%d", req.TransactionID, req.Attempt),
		ProjectID:      req.ProjectID, Chapter: req.Chapter, TransactionID: req.TransactionID,
		Agent: "writer", Operation: "draft", Attempt: req.Attempt, Payload: payload,
	})
	if err != nil {
		return WriterResult{}, err
	}
	text := strings.TrimSpace(string(response))
	if text == "" {
		return WriterResult{}, errors.New("writer returned empty draft")
	}
	version := call.ResponseHash
	if len(version) > 16 {
		version = version[:16]
	}
	return WriterResult{Text: text, SourceVersion: "model-" + version}, nil
}

// ModelLibrarianService strictly decodes a FactProposal. Provenance is validated
// against the durable candidate before the proposal can be persisted.
type ModelLibrarianService struct {
	Caller  *IdempotentModelCaller
	Decoder StructuredOutputDecoder
}

func (s ModelLibrarianService) Propose(ctx context.Context, req LibrarianRequest) (FactProposal, error) {
	if s.Caller == nil || s.Decoder == nil {
		return FactProposal{}, errors.New("model librarian is not configured")
	}
	payload, err := json.Marshal(struct {
		Schema    string    `json:"schema"`
		ProjectID string    `json:"project_id"`
		Chapter   int       `json:"chapter"`
		Candidate Candidate `json:"candidate"`
		Draft     string    `json:"draft"`
	}{"FactProposal", req.ProjectID, req.Chapter, req.Candidate, req.Candidate.Text})
	if err != nil {
		return FactProposal{}, err
	}
	response, _, _, err := s.Caller.Call(ctx, CallRequest{
		IdempotencyKey: req.TransactionID + ":librarian:" + req.Candidate.ID,
		ProjectID:      req.ProjectID, Chapter: req.Chapter, TransactionID: req.TransactionID,
		Agent: "librarian", Operation: "fact_proposal", Attempt: req.Candidate.Attempt, Payload: payload,
	})
	if err != nil {
		return FactProposal{}, err
	}
	var proposal FactProposal
	if _, err := s.Decoder.Decode(ctx, "FactProposal", response, &proposal); err != nil {
		return FactProposal{}, err
	}
	if proposal.ProjectID != req.ProjectID || proposal.Chapter != req.Chapter || proposal.SourceVersion != req.Candidate.SourceVersion || proposal.SourceSHA != req.Candidate.TextSHA {
		return FactProposal{}, errors.New("librarian proposal provenance does not match durable candidate")
	}
	return proposal, nil
}

// ModelEditorService strictly decodes literary review output. It receives the
// deterministic Continuity result but has no ability to override a blocking FAIL.
type ModelEditorService struct {
	Context EditorialContextProvider
	Caller  *IdempotentModelCaller
	Decoder StructuredOutputDecoder
}

func (s ModelEditorService) Review(ctx context.Context, req EditorRequest) (EditorReview, error) {
	var selected json.RawMessage
	var notes []string
	if s.Context != nil {
		var err error
		selected, notes, err = s.Context.CompileEditorialContext(ctx, req)
		if err != nil {
			return EditorReview{}, err
		}
	}
	if s.Caller == nil || s.Decoder == nil {
		return EditorReview{}, errors.New("model editor is not configured")
	}
	payload, err := json.Marshal(struct {
		AuthoringContext json.RawMessage  `json:"authoring_context,omitempty"`
		Schema           string           `json:"schema"`
		Candidate        Candidate        `json:"candidate"`
		Draft            string           `json:"draft"`
		Continuity       ContinuityResult `json:"continuity"`
	}{selected, "EditorReview", req.Candidate, req.Candidate.Text, req.Continuity})
	if err != nil {
		return EditorReview{}, err
	}
	response, _, _, err := s.Caller.Call(ctx, CallRequest{
		IdempotencyKey: req.TransactionID + ":editor:" + req.Candidate.ID,
		ProjectID:      req.ProjectID, Chapter: req.Chapter, TransactionID: req.TransactionID,
		Agent: "editor", Operation: "review", Attempt: req.Candidate.Attempt, Payload: payload,
	})
	if err != nil {
		return EditorReview{}, err
	}
	var review EditorReview
	if _, err := s.Decoder.Decode(ctx, "EditorReview", response, &review); err != nil {
		return EditorReview{}, err
	}
	review.Weaknesses = append(review.Weaknesses, notes...)
	return review, nil
}
