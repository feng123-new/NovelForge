// Package userrules 是用户规则归一化的服务层：把各来源的自然语言规则经 LLM 单次调用
// 归一化成候选结构化字段，再由 rules.BuildSnapshot 确定性合并成本书快照。
//
// 分层职责：
//   - rules 包：纯数据 + 确定性合并（Snapshot / Candidate / BuildSnapshot / SystemDefaults）
//   - 本包：LLM 归一化 + 编排 + 落盘（依赖 agentcore + store + rules）
//
// 归一化是增强路径，不是主创作的前置条件：任何来源失败都降级为 raw preferences，主创作必须继续。
package userrules

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/rules"
)

// normalizeMaxTokens 单次归一化的输出上限。规则文本通常很短，2048 足够。
const normalizeMaxTokens = 2048

// normalizeMaxAttempts 归一化总尝试次数（1 次重试后降级，不做无界重试，见设计 §失败与降级）。
// LLM 输出有随机性，一次解析失败重试一次常能拿到合法 JSON；瞬时网络抖动同理。
const normalizeMaxAttempts = 2

// Normalizer 把单个来源的自然语言规则归一化成 rules.Candidate（单次 LLM 调用）。
type Normalizer struct {
	model agentcore.ChatModel
}

// NewNormalizer 用一个 ChatModel 构造归一化器。归一化是一次性启动工具，
// 应传入能力较强的模型（如 ModelSet 的默认模型），不必跟随写作的弱模型。
func NewNormalizer(model agentcore.ChatModel) *Normalizer {
	return &Normalizer{model: model}
}

// Normalize 归一化一个来源。永不返回 error——失败时返回 degraded Candidate
// （原文作 raw preferences、不产 structured），由调用方继续合并。
func (n *Normalizer) Normalize(ctx context.Context, source, text string) rules.Candidate {
	text = strings.TrimSpace(text)
	if text == "" {
		return rules.Candidate{Source: source}
	}
	if n == nil || n.model == nil {
		return degraded(source, text)
	}

	messages := []agentcore.Message{
		{Role: agentcore.RoleSystem, Content: []agentcore.ContentBlock{agentcore.TextBlock(normalizerSystemPrompt)}},
		{Role: agentcore.RoleUser, Content: []agentcore.ContentBlock{agentcore.TextBlock(text)}},
	}

	// 有限重试后降级：技术错误（网络/模型/非法 JSON）进日志、不进快照，
	// 快照只留 status=degraded + 来源标注（见设计 §失败与降级 / §回显）。
	var lastErr string
	for attempt := 1; attempt <= normalizeMaxAttempts; attempt++ {
		resp, err := n.model.Generate(ctx, messages, nil, agentcore.WithMaxTokens(normalizeMaxTokens))
		switch {
		case err != nil:
			lastErr = err.Error()
		case resp == nil:
			lastErr = "模型返回空响应"
		default:
			if out, ok := parseNormalizerJSON(resp.Message.TextContent()); ok {
				return rules.Candidate{
					Source:      source,
					Structured:  out.Structured,
					Preferences: strings.TrimSpace(out.Preferences),
					Uncertain:   coerceUncertain(out.Uncertain),
				}
			}
			lastErr = "返回非合法 JSON"
		}
		slog.Warn("规则归一化失败",
			"module", "rules", "source", source, "attempt", attempt, "err", lastErr)
		if ctx.Err() != nil {
			break // ctx 取消则重试也必失败，直接降级
		}
	}
	return degraded(source, text)
}

// degraded 构造一个降级候选：归一化失败时把原文当作风格偏好，不提炼任何机械规则。
// uncertain 标注来源（便于回显"哪些来源未能解析"），但不含技术错误细节——技术错误只进日志。
func degraded(source, text string) rules.Candidate {
	return rules.Candidate{
		Source:      source,
		Preferences: text,
		Uncertain:   []string{source + "：归一化失败，已按原文作为风格偏好处理（未提炼机械规则）"},
		Degraded:    true,
	}
}

// normalizerOutput 是归一化器约定的 JSON 形态。
// Structured 直接复用 rules.Structured（JSON 形状一致）；Uncertain 用 RawMessage 容忍
// 模型回的多种形态（string / []string / [{item,reason}]，原型实测均出现过）。
type normalizerOutput struct {
	Structured  rules.Structured `json:"structured"`
	Preferences string           `json:"preferences"`
	Uncertain   json.RawMessage  `json:"uncertain"`
}

func parseNormalizerJSON(raw string) (normalizerOutput, bool) {
	s := extractJSON(raw)
	if s == "" {
		return normalizerOutput{}, false
	}
	var out normalizerOutput
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return normalizerOutput{}, false
	}
	return out, true
}

// extractJSON 从模型回复里抠出 JSON 对象：剥 ```json 围栏，取首个 { 到末个 }。
func extractJSON(raw string) string {
	s := strings.TrimSpace(raw)
	if after, ok := strings.CutPrefix(s, "```"); ok {
		s = after
		s = strings.TrimPrefix(s, "json")
		s = strings.TrimPrefix(s, "JSON")
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < 0 || end < start {
		return ""
	}
	return s[start : end+1]
}

// coerceUncertain 把模型回的 uncertain 统一成 []string，容忍 string / []string / []object 三种形态。
func coerceUncertain(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return nonEmpty(list)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s = strings.TrimSpace(s); s != "" {
			return []string{s}
		}
		return nil
	}
	var objs []map[string]any
	if err := json.Unmarshal(raw, &objs); err == nil {
		var out []string
		for _, o := range objs {
			if str := stringifyUncertainObj(o); str != "" {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

func stringifyUncertainObj(o map[string]any) string {
	item, _ := o["item"].(string)
	if item == "" {
		item, _ = o["field"].(string)
	}
	reason, _ := o["reason"].(string)
	switch {
	case item != "" && reason != "":
		return item + "：" + reason
	case item != "":
		return item
	case reason != "":
		return reason
	default:
		b, _ := json.Marshal(o)
		return string(b)
	}
}

func nonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// normalizerSystemPrompt 是归一化器的系统提示词。
// 已用 10 条真实例子（含阈值发明陷阱）验证保守提升成立（10/10）。
const normalizerSystemPrompt = `你是 AI 小说写作系统的「规则归一化器」。你读取用户某一个来源的长期写作规则(自然语言),抽取成结构化形式。只输出一个 JSON 对象,不要任何解释文字。

输出 JSON 三个字段:structured / preferences / uncertain。

structured 只允许以下字段(没有别的字段):
- genre: 字符串(题材)
- chapter_words: {min:整数, max:整数}(每章字数区间)
- forbidden_chars: [字符串](禁止出现的字符)
- forbidden_phrases: [字符串](禁止出现的短语,字面精确匹配)
- fatigue_words: {词:整数}(疲劳词→每章出现次数上限)

【保守提升——最重要】
- 只有用户明确、无歧义时才写入 structured。
- forbidden_chars/forbidden_phrases 是 error 级:只有「不要出现X/禁用X/别写X」这类明确禁止才提升。
- fatigue_words:只有同时给出「明确的词」和「明确的次数阈值」才提升;「少用X/别老用X」没给数字的放进 preferences,绝不自己发明阈值。
- chapter_words:只有给出明确区间/上限/下限/目标字数才提升;「短一点/节奏快点」放进 preferences。
- 不可机械检查、无明确阈值、依赖语境的,一律放 preferences。
- 原则:宁可漏进 structured,也不要错误提升(那会每章误报)。

preferences:自然语言风格/人物/审美偏好,一段可读文本。
uncertain:你故意没提升到 structured 的项+原因(字符串数组)。`
