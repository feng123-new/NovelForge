package host

import (
	"errors"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
)

func testObserver(events *[]Event) *observer {
	return &observer{
		emitEv: func(ev Event) {
			*events = append(*events, ev)
		},
		emitD:               func(string) {},
		emitC:               func() {},
		agents:              make(map[string]*agentState),
		lastThinkingByAgent: make(map[string]string),
		dispatchStarts:      make(map[string]*activeCall),
		toolStarts:          make(map[string]*activeCall),
		streamExtractors:    make(map[string]*agentExtractor),
		streamArgPrefixes:   make(map[string]string),
		streamArgLabels:     make(map[string]string),
		retryEvents:         make(map[string]string),
	}
}

func TestObserverSubagentRetryEventsUpdateSameLinePerAgent(t *testing.T) {
	var events []Event
	o := testObserver(&events)

	for i := 1; i <= 2; i++ {
		o.handleToolUpdate(agentcore.Event{
			Type: agentcore.EventToolExecUpdate,
			Progress: &agentcore.ProgressPayload{
				Kind:       agentcore.ProgressRetry,
				Agent:      "writer",
				Attempt:    i,
				MaxRetries: 7,
				Message:    "stream read error: INTERNAL_ERROR; received from peer [network, openai]",
			},
		})
	}

	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 raw update events", len(events))
	}
	if events[0].ID == "" || events[1].ID != events[0].ID {
		t.Fatalf("writer retry events should share ID: %+v", events)
	}
	// Summary 不嵌静态延时（UI 依 RetryAt 倒计时）；延时以截止时刻形式携带，静态快照留在 Detail 供日志。
	if events[1].Agent != "writer" || !strings.Contains(events[1].Summary, "重试 (2/7)") {
		t.Fatalf("event = %+v, want writer retry 2/7 without inline delay", events[1])
	}
	if events[1].RetryAt.IsZero() || !strings.Contains(events[1].Detail, "重试 (2/7，2s后)") {
		t.Fatalf("event = %+v, want RetryAt deadline + static delay in Detail", events[1])
	}
	if events[1].Kind != "network" {
		t.Fatalf("event kind = %q, want network", events[1].Kind)
	}
}

func TestObserverDispatchErrorUpdatesSingleEventWithDetail(t *testing.T) {
	var events []Event
	o := testObserver(&events)

	o.dispatchStart("architect_long", "规划长篇小说")
	runErr := errors.New("stream read error: INTERNAL_ERROR; received from peer [network, openai]")
	o.dispatchFinish("architect_long", runErr)

	if len(events) != 2 {
		t.Fatalf("start + failed update 应只有 2 个原始事件，got %d: %+v", len(events), events)
	}
	start, failed := events[0], events[1]
	if failed.ID != start.ID || failed.Category != "DISPATCH" || !failed.Failed || failed.Level != "error" {
		t.Fatalf("失败应原地更新 DISPATCH: start=%+v failed=%+v", start, failed)
	}
	if failed.Detail != runErr.Error() || failed.Kind != "network" || !strings.Contains(failed.Summary, "INTERNAL_ERROR") {
		t.Fatalf("DISPATCH 应携带完整错误和分类: %+v", failed)
	}
}

func TestObserverSubagentToolDeltaUpdatesSaveFoundationType(t *testing.T) {
	var events []Event
	o := testObserver(&events)

	o.handleSubagentDelta(&agentcore.ProgressPayload{
		Kind:      agentcore.ProgressToolDelta,
		Agent:     "architect_long",
		Tool:      "save_foundation",
		DeltaKind: agentcore.DeltaToolCall,
		Delta:     `{"type":"premise","content":"# 书名`,
	})

	if len(events) < 2 {
		t.Fatalf("events = %d, want start + summary update", len(events))
	}
	if events[0].Category != "TOOL" || events[0].Summary != "save_foundation" || events[0].Depth != 1 {
		t.Fatalf("start event = %+v", events[0])
	}
	if events[1].ID != events[0].ID || events[1].Summary != "save_foundation[premise]" {
		t.Fatalf("summary update = %+v, start = %+v", events[1], events[0])
	}
}

func TestObserverSubagentToolDeltaUpdatesSaveFoundationTypeAcrossChunks(t *testing.T) {
	var events []Event
	o := testObserver(&events)

	for _, delta := range []string{`{"ty`, `pe":"premise","content":"# 书名`} {
		o.handleSubagentDelta(&agentcore.ProgressPayload{
			Kind:      agentcore.ProgressToolDelta,
			Agent:     "architect_long",
			Tool:      "save_foundation",
			DeltaKind: agentcore.DeltaToolCall,
			Delta:     delta,
		})
	}

	var summaries []string
	for _, ev := range events {
		summaries = append(summaries, ev.Summary)
	}
	if !strings.Contains(strings.Join(summaries, "\n"), "save_foundation[premise]") {
		t.Fatalf("summaries = %v, want save_foundation[premise]", summaries)
	}
}

func TestObserverToolErrorUpdatesSingleToolEventWithFullDetail(t *testing.T) {
	var events []Event
	o := testObserver(&events)

	o.handleToolUpdate(agentcore.Event{
		Type: agentcore.EventToolExecUpdate,
		Progress: &agentcore.ProgressPayload{
			Kind:  agentcore.ProgressToolStart,
			Agent: "writer",
			Tool:  "edit_chapter",
		},
	})
	o.handleToolUpdate(agentcore.Event{
		Type: agentcore.EventToolExecUpdate,
		Progress: &agentcore.ProgressPayload{
			Kind:    agentcore.ProgressToolError,
			Agent:   "writer",
			Tool:    "edit_chapter",
			Message: "old_string 无法精确匹配草稿",
		},
	})

	if len(events) != 2 {
		t.Fatalf("start + failed update 应只有 2 个原始事件，got %d: %+v", len(events), events)
	}
	start, failed := events[0], events[1]
	if failed.ID == "" || failed.ID != start.ID || !failed.Failed || failed.Category != "TOOL" || failed.Level != "error" {
		t.Fatalf("失败事件应原地更新 TOOL 行: start=%+v failed=%+v", start, failed)
	}
	if !strings.Contains(failed.Summary, "old_string") || !strings.Contains(failed.Detail, "无法精确匹配草稿") {
		t.Fatalf("失败事件应同时保留 UI 摘要和完整日志详情: %+v", failed)
	}
}
