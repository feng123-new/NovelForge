package host

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
)

const coCreateSystemPrompt = `你是一个小说共创助手。你的任务不是直接开始写小说，而是通过多轮简短对话帮助用户澄清创作需求，并持续整理出一段可直接交给创作引擎的中文创作指令。

每一轮回复严格按以下格式输出，包含三个标记，依次出现：

[REPLY]
（给用户看的中文自然回复：先回应用户的输入，再最多提出 1 到 2 个当前最关键的问题。如果信息已足够开始创作，告诉用户可以按 Ctrl+S 开始。）

[DRAFT]
（当前完整的创作指令草稿，使用 Markdown：直接从二级标题开始，例如 "## 主题"、"## 关键要素"、"## 待澄清信息"；用项目符号列出要点。每一轮都要在已有结论上**累积更新**，吸收用户最新意图；即使本轮没有新增也要把完整草稿原样再写一次——不要省略、不要写"（保持上一轮）"之类的占位。）

[READY]
（只写 true 或 false：信息是否已足够开始创作。）

输出规范：
- 三个标记 [REPLY] / [DRAFT] / [READY] 必须依次完整出现，每个标记独占一行。
- 三个标记之外不要添加任何说明、思考或代码围栏。
- [DRAFT] 段落允许多行 Markdown，直接换行书写，不需要任何转义。`

// CoCreateProgressKind 标识流式回调的内容类型。
const (
	CoCreateProgressThinking = "thinking"
	CoCreateProgressReply    = "reply"
)

// 三段式输出标记。token-based 协议比 JSON 鲁棒：无引号/无转义/允许多行 Markdown，
// 模型几乎不会写错；解析就是三段 split。
const (
	markerReply = "[REPLY]"
	markerDraft = "[DRAFT]"
	markerReady = "[READY]"
)

func coCreateStream(ctx context.Context, models *bootstrap.ModelSet, history []CoCreateMessage, onProgress func(kind, text string)) (CoCreateReply, error) {
	if len(history) == 0 {
		return CoCreateReply{}, fmt.Errorf("cocreate history is empty")
	}

	model := models.ForRole("thinking")
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()

	msgs := []agentcore.Message{agentcore.SystemMsg(coCreateSystemPrompt)}
	for _, item := range history {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(item.Role)) {
		case "assistant":
			msgs = append(msgs, assistantMsg(content))
		default:
			msgs = append(msgs, agentcore.UserMsg(content))
		}
	}

	streamCh, err := model.GenerateStream(ctx, msgs, nil, agentcore.WithMaxTokens(2048))
	if err != nil {
		return CoCreateReply{}, fmt.Errorf("cocreate generate: %w", err)
	}

	var raw, thinking strings.Builder
	var streamed bool
	for ev := range streamCh {
		switch ev.Type {
		case agentcore.StreamEventThinkingDelta:
			thinking.WriteString(ev.Delta)
			if onProgress != nil {
				onProgress(CoCreateProgressThinking, thinking.String())
			}
		case agentcore.StreamEventTextDelta:
			streamed = true
			raw.WriteString(ev.Delta)
			if onProgress != nil {
				onProgress(CoCreateProgressReply, extractReplyPreview(raw.String()))
			}
		case agentcore.StreamEventDone:
			if !streamed {
				raw.WriteString(ev.Message.TextContent())
			}
		case agentcore.StreamEventError:
			if ev.Err != nil {
				return CoCreateReply{}, fmt.Errorf("cocreate generate: %w", ev.Err)
			}
			return CoCreateReply{}, fmt.Errorf("cocreate generate failed")
		}
	}
	return parseCoCreateResponse(raw.String())
}

func assistantMsg(text string) agentcore.Message {
	return agentcore.Message{
		Role:      agentcore.RoleAssistant,
		Content:   []agentcore.ContentBlock{agentcore.TextBlock(text)},
		Timestamp: time.Now(),
	}
}

// parseCoCreateResponse 解析三段式输出。模型若没遵守标记（直接说自然语言），
// 整段作为 reply 显示，draft 留空让 session 保留上一轮。
func parseCoCreateResponse(raw string) (CoCreateReply, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return CoCreateReply{}, fmt.Errorf("cocreate empty response")
	}

	reply, draft, ready := splitCoCreateMarkers(raw)
	if reply == "" {
		// 模型没遵守标记协议：整段作为 reply。
		return CoCreateReply{Message: raw, Prompt: "", Ready: false, Raw: raw}, nil
	}
	return CoCreateReply{Message: reply, Prompt: draft, Ready: ready, Raw: raw}, nil
}

// splitCoCreateMarkers 按 [REPLY] / [DRAFT] / [READY] 三个标记切分文本。
// 标记可能缺失（流式中段或模型遗漏），缺失部分对应字段为空 / false。
// 标记的相对顺序不强制——找哪个标记后的文本到下一个出现的标记之前。
func splitCoCreateMarkers(s string) (reply, draft string, ready bool) {
	rIdx := strings.Index(s, markerReply)
	dIdx := strings.Index(s, markerDraft)
	yIdx := strings.Index(s, markerReady)

	cut := func(start int, marker string, ends ...int) string {
		if start < 0 {
			return ""
		}
		from := start + len(marker)
		end := len(s)
		for _, e := range ends {
			if e > from && e < end {
				end = e
			}
		}
		return strings.TrimSpace(s[from:end])
	}

	reply = cut(rIdx, markerReply, dIdx, yIdx)
	draft = cut(dIdx, markerDraft, rIdx, yIdx)
	readyStr := strings.ToLower(cut(yIdx, markerReady, rIdx, dIdx))
	ready = readyStr == "true" || readyStr == "yes"
	return
}

// extractReplyPreview 流式预览：raw 还在生长时给 UI 一段可显示的文本。
// 看到 [REPLY] 之后到 [DRAFT] 之前的内容；标记还没出现就先回原始文本。
func extractReplyPreview(raw string) string {
	trimmed := strings.TrimSpace(raw)
	rIdx := strings.Index(trimmed, markerReply)
	if rIdx < 0 {
		// [REPLY] 标记还没流出来 → 暂时整段做预览，标记到达后会被切掉。
		return trimmed
	}
	rest := trimmed[rIdx+len(markerReply):]
	if dIdx := strings.Index(rest, markerDraft); dIdx >= 0 {
		rest = rest[:dIdx]
	}
	return strings.TrimSpace(rest)
}
