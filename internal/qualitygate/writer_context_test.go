package qualitygate

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type contextProbe struct{ err error }
func (p contextProbe) CompileWriterContext(context.Context, WriterRequest, int) (json.RawMessage, error) {
	if p.err != nil { return nil, p.err }
	return json.RawMessage(`{"text":"PINNED-CONTEXT","context_sha":"fixture"}`), nil
}

type contextCallProbe struct { calls int; payload string }
func (p *contextCallProbe) Invoke(_ context.Context, _ string, payload []byte) ([]byte, ModelUsage, error) {
	p.calls++; p.payload = string(payload)
	return []byte("chapter prose"), ModelUsage{}, nil
}

type contextRepoProbe struct { call ModelCall; response string }
func (p *contextRepoProbe) GetModelCall(context.Context, string) (ModelCall, string, error) {
	if p.call.IdempotencyKey == "" { return ModelCall{}, "", ErrNotFound }
	return p.call, p.response, nil
}
func (p *contextRepoProbe) SaveModelCall(_ context.Context, c ModelCall, response string) error {
	p.call = c; p.response = response; return nil
}

func TestWriterContextPrecedesModelCallAndReplay(t *testing.T) {
	invoker := &contextCallProbe{}
	repo := &contextRepoProbe{}
	writer := ModelWriterService{Caller: &IdempotentModelCaller{Repository: repo, Invoker: invoker}, Context: contextProbe{}}
	req := WriterRequest{ProjectID: "p", Chapter: 1, TransactionID: "tx", Plan: ChapterPlan{Chapter: 1}}
	if _, err := writer.Write(context.Background(), req); err != nil { t.Fatal(err) }
	if !strings.Contains(invoker.payload, "PINNED-CONTEXT") { t.Fatal("compiler output absent from model payload") }
	if _, err := writer.Write(context.Background(), req); err != nil { t.Fatal(err) }
	if invoker.calls != 1 { t.Fatal("completed call did not replay") }
	writer.Context = contextProbe{err: errors.New("mandatory overflow")}
	req.TransactionID = "another"
	if _, err := writer.Write(context.Background(), req); err == nil { t.Fatal("compiler failure was ignored") }
	if invoker.calls != 1 { t.Fatal("compiler failure reached provider") }
}
