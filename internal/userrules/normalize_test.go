package userrules

import (
	"encoding/json"
	"testing"
)

func TestExtractJSON_StripsCodeFences(t *testing.T) {
	cases := []struct{ in, wantHas string }{
		{"```json\n{\"a\":1}\n```", `"a":1`},
		{"```\n{\"a\":1}\n```", `"a":1`},
		{"前缀解释\n{\"a\":1}\n后缀", `"a":1`},
		{"{\"a\":1}", `"a":1`},
	}
	for _, c := range cases {
		got := extractJSON(c.in)
		if got == "" {
			t.Fatalf("extractJSON(%q) 返回空", c.in)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(got), &m); err != nil {
			t.Fatalf("extractJSON(%q)=%q 不是合法 JSON: %v", c.in, got, err)
		}
	}
	if extractJSON("没有任何 JSON") != "" {
		t.Fatal("无 JSON 时应返回空串")
	}
}

func TestCoerceUncertain_HandlesAllDriftForms(t *testing.T) {
	// 原型实测：uncertain 时而字符串、时而 []string、时而 [{item,reason}]。
	cases := []struct {
		name string
		raw  string
		want int // 期望条目数（>0 即可，验证不丢）
	}{
		{"array_of_string", `["少用比喻：无阈值"]`, 1},
		{"plain_string", `"chapter_words 太模糊未提升"`, 1},
		{"array_of_object", `[{"item":"少用比喻","reason":"无明确阈值"}]`, 1},
		{"array_of_field_object", `[{"field":"chapter_words.min","reason":"未给下限"}]`, 1},
		{"empty_array", `[]`, 0},
		{"empty_string", `""`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := coerceUncertain(json.RawMessage(c.raw))
			if len(got) != c.want {
				t.Fatalf("coerceUncertain(%s)=%v，期望 %d 条", c.raw, got, c.want)
			}
		})
	}
}

func TestParseNormalizerJSON_FullOutput(t *testing.T) {
	raw := "```json\n" + `{
  "structured": {"chapter_words": {"min": 1200, "max": 1600}, "forbidden_phrases": ["某种程度上"]},
  "preferences": "主角冷静克制",
  "uncertain": [{"item": "少用比喻", "reason": "无阈值"}]
}` + "\n```"
	out, ok := parseNormalizerJSON(raw)
	if !ok {
		t.Fatal("应解析成功")
	}
	if out.Structured.ChapterWords == nil || out.Structured.ChapterWords.Min != 1200 {
		t.Fatalf("chapter_words 解析错误：%+v", out.Structured.ChapterWords)
	}
	if len(out.Structured.ForbiddenPhrases) != 1 || out.Structured.ForbiddenPhrases[0] != "某种程度上" {
		t.Fatalf("forbidden_phrases 解析错误：%v", out.Structured.ForbiddenPhrases)
	}
	if out.Preferences != "主角冷静克制" {
		t.Fatalf("preferences 解析错误：%q", out.Preferences)
	}
	if got := coerceUncertain(out.Uncertain); len(got) != 1 {
		t.Fatalf("uncertain 应有 1 条，得到 %v", got)
	}
}

func TestParseNormalizerJSON_GarbageFails(t *testing.T) {
	if _, ok := parseNormalizerJSON("模型只回了一句话，没有 JSON"); ok {
		t.Fatal("无 JSON 应解析失败（触发降级）")
	}
	if _, ok := parseNormalizerJSON("{ 不完整"); ok {
		t.Fatal("残缺 JSON 应解析失败")
	}
}

func TestNormalize_NilModelDegrades(t *testing.T) {
	// 无模型可用：整体降级为 raw preferences，不产 structured，永不 panic/返错。
	var n *Normalizer = NewNormalizer(nil)
	cand := n.Normalize(t.Context(), "startup_prompt", "每章1200字，主角冷静")
	if !cand.Degraded {
		t.Fatal("无模型应降级")
	}
	if cand.Preferences == "" {
		t.Fatal("降级应保留原文为 preferences")
	}
	if cand.Structured.ChapterWords != nil {
		t.Fatal("降级不应产出 structured")
	}
}
