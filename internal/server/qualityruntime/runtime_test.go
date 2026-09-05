package qualityruntime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
)

type probeModel struct {
	calls    int
	messages []agentcore.Message
	stop     agentcore.StopReason
}

func (m *probeModel) Generate(_ context.Context, msg []agentcore.Message, _ []agentcore.ToolSpec, _ ...agentcore.CallOption) (*agentcore.LLMResponse, error) {
	m.calls++
	m.messages = msg
	return &agentcore.LLMResponse{Message: agentcore.Message{Content: []agentcore.ContentBlock{agentcore.TextBlock("chapter prose")}, StopReason: m.stop, Usage: &agentcore.Usage{Input: 12, Output: 8, Provider: "fake", Model: "actual"}}}, nil
}
func (*probeModel) GenerateStream(context.Context, []agentcore.Message, []agentcore.ToolSpec, ...agentcore.CallOption) (<-chan agentcore.StreamEvent, error) {
	return nil, errors.New("unexpected stream")
}
func (*probeModel) SupportsTools() bool { return false }

func TestRuntimeOperationAndIncompleteResponse(t *testing.T) {
	fake := &probeModel{}
	runtime := &Runtime{models: &bootstrap.ModelSet{Default: bootstrap.NewSwappableModel("fake", "test", fake, nil)}}
	data, usage, err := runtime.Invoke(context.Background(), "writer:draft", []byte(`{"compiled_context":{"text":"PINNED-CONTEXT"}}`))
	if err != nil || string(data) != "chapter prose" || usage.InputTokens != 12 {
		t.Fatalf("%s %+v %v", data, usage, err)
	}
	if len(fake.messages) != 2 || !strings.Contains(fake.messages[1].TextContent(), "PINNED-CONTEXT") {
		t.Fatal("compiled context not sent")
	}
	if _, _, err := runtime.Invoke(context.Background(), "writer:delete", []byte(`{}`)); err == nil {
		t.Fatal("unknown operation accepted")
	}
	if fake.calls != 1 {
		t.Fatal("invalid operation called provider")
	}
	fake.stop = agentcore.StopReasonLength
	if _, _, err := runtime.Invoke(context.Background(), "writer:draft", []byte(`{}`)); err == nil {
		t.Fatal("truncated draft accepted")
	}
}
