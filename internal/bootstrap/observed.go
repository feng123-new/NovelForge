package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"unicode/utf8"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/observability"
)

// ForRoleTracked uses the same primary/fallback selection as the retained router,
// with admission and accounting at each explicit non-streaming Generate boundary.
// The retained TUI/stream path is unchanged and is not reported as fully metered.
func (ms *ModelSet) ForRoleTracked(role string, reporters ...FailoverReporter) agentcore.ChatModel {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	primary := ms.models[role]
	if primary == nil {
		primary = ms.Default
	}
	var report FailoverReporter
	if len(reporters) > 0 {
		report = reporters[0]
	}
	return &failoverModel{role: role, primary: primary, set: ms, report: report}
}
func observedGenerate(ctx context.Context, target modelTarget, messages []agentcore.Message, tools []agentcore.ToolSpec, opts ...agentcore.CallOption) (*agentcore.LLMResponse, error) {
	encoded, _ := json.Marshal(struct {
		Messages []agentcore.Message
		Tools    []agentcore.ToolSpec
	}{messages, tools})
	// A conservative rune-based admission estimate, never provider billing usage.
	ticket, err := observability.Start(ctx, target.provider, target.name, utf8.RuneCount(encoded)+256, "sdk_generate")
	if err != nil {
		return nil, err
	}
	if ticket != nil {
		opts = append(append([]agentcore.CallOption{}, opts...), agentcore.WithMaxTokens(ticket.OutputLimit()))
	}
	response, callErr := target.model.Generate(ctx, messages, tools, opts...)
	in, out, known := 0, 0, false
	code := ObservedErrorCode(callErr)
	if response != nil {
		if u := response.Message.Usage; u != nil {
			in, out, known = u.Input, u.Output, true
		}
		switch response.Message.StopReason {
		case agentcore.StopReasonLength, agentcore.StopReasonError, agentcore.StopReasonSafety, agentcore.StopReasonAborted:
			code = "RESPONSE_INCOMPLETE"
		}
	}
	// Persistence failures must not cause failover/retry of a completed response.
	if err = ticket.Finish(ctx, in, out, known, code); err != nil {
		if callErr == nil && response != nil {
			return response, nil
		}
		return response, err
	}
	return response, callErr
}
func ObservedErrorCode(err error) string {
	if err == nil {
		return ""
	}
	if code := observability.ControlCode(err); code != "" {
		return code
	}
	if errors.Is(err, context.Canceled) {
		return "REQUEST_CANCELLED"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "PROVIDER_TIMEOUT"
	}
	var n net.Error
	if errors.As(err, &n) && n.Timeout() {
		return "PROVIDER_TIMEOUT"
	}
	switch agentcore.FailoverReason(err) {
	case "rate_limit":
		return "PROVIDER_RATE_LIMIT"
	case "timeout", "stream_idle":
		return "PROVIDER_TIMEOUT"
	case "network":
		return "PROVIDER_NETWORK"
	}
	// Raw provider text is only inspected to classify it; it is never persisted.
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "401") || strings.Contains(text, "403") || strings.Contains(text, "unauthorized") {
		return "PROVIDER_AUTH"
	}
	if strings.Contains(text, "429") {
		return "PROVIDER_RATE_LIMIT"
	}
	return "PROVIDER_FAILED"
}
