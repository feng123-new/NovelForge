package qualitygate

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type capturedContextInvoker struct {
	calls int
	body  []byte
}

func (m *capturedContextInvoker) Invoke(_ context.Context, _ string, body []byte) ([]byte, ModelUsage, error) {
	m.calls++
	m.body = append([]byte(nil), body...)
	return []byte("A bounded chapter."), ModelUsage{}, nil
}

type contextCallMemory struct {
	calls map[string]ModelCall
	text  map[string]string
}

func (s *contextCallMemory) GetModelCall(_ context.Context, key string) (ModelCall, string, error) {
	call, ok := s.calls[key]
	if !ok {
		return ModelCall{}, "", ErrNotFound
	}
	return call, s.text[key], nil
}

func (s *contextCallMemory) SaveModelCall(_ context.Context, call ModelCall, text string) error {
	s.calls[call.IdempotencyKey], s.text[call.IdempotencyKey] = call, text
	return nil
}

type fixedWriterContext struct {
	text json.RawMessage
	err  error
}

func (c *fixedWriterContext) WriterContext(context.Context, WriterRequest) (json.RawMessage, error) {
	return c.text, c.err
}

func TestWriterContextParticipatesInActualRequestAndReplay(t *testing.T) {
	ctx := context.Background()
	model := &capturedContextInvoker{}
	memory := &contextCallMemory{calls: map[string]ModelCall{}, text: map[string]string{}}
	contextSource := &fixedWriterContext{text: json.RawMessage(`{"text":"PINNED POV STATE","context_sha":"one"}`)}
	writer := ModelWriterService{Caller: &IdempotentModelCaller{Repository: memory, Invoker: model}, Context: contextSource}
	request := WriterRequest{ProjectID: "p", Chapter: 1, TransactionID: "tx", Plan: ChapterPlan{Chapter: 1}}
	if _, err := writer.Write(ctx, request); err != nil {
		t.Fatal(err)
	}
	var sent struct {
		Context struct {
			Text string `json:"text"`
		} `json:"context"`
	}
	if err := json.Unmarshal(model.body, &sent); err != nil || sent.Context.Text != "PINNED POV STATE" {
		t.Fatalf("actual request omitted compiled context: %s (%v)", model.body, err)
	}
	if _, err := writer.Write(ctx, request); err != nil || model.calls != 1 {
		t.Fatalf("replay calls=%d err=%v", model.calls, err)
	}
	contextSource.text = json.RawMessage(`{"text":"CHANGED CONTEXT","context_sha":"two"}`)
	if _, err := writer.Write(ctx, request); !errors.Is(err, ErrIdempotencyConflict) || model.calls != 1 {
		t.Fatalf("context changed without conflict: calls=%d err=%v", model.calls, err)
	}
	contextSource.err = errors.New("mandatory context unavailable")
	if _, err := writer.Write(ctx, WriterRequest{TransactionID: "new"}); err == nil || model.calls != 1 {
		t.Fatalf("compiler error must prevent model invocation: calls=%d err=%v", model.calls, err)
	}
}
