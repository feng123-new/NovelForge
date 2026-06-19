package bootstrap

import "testing"

func TestConfigResolveThinking(t *testing.T) {
	cfg := Config{
		Thinking: "low", // 顶层默认
		Roles: map[string]RoleConfig{
			"writer":    {Provider: "p", Model: "m", Thinking: "high"}, // 角色覆盖
			"architect": {Provider: "p", Model: "m"},                   // 无 thinking，应回落默认
		},
	}

	cases := []struct {
		role string
		want string
	}{
		{"writer", "high"},     // 角色覆盖优先
		{"architect", "low"},   // 角色未配 → 回落顶层默认
		{"editor", "low"},      // 角色不存在 → 顶层默认
		{"", "low"},            // 空 → 顶层默认
		{"default", "low"},     // default → 顶层默认
		{"coordinator", "low"}, // 未配 → 顶层默认
	}
	for _, c := range cases {
		if got := cfg.ResolveThinking(c.role); got != c.want {
			t.Errorf("ResolveThinking(%q) = %q, want %q", c.role, got, c.want)
		}
	}

	// 顶层默认也为空时，未覆盖角色返回 ""（不覆盖）。
	empty := Config{Roles: map[string]RoleConfig{"writer": {Thinking: "xhigh"}}}
	if got := empty.ResolveThinking("editor"); got != "" {
		t.Errorf("空默认下 editor 应返回 \"\"，得 %q", got)
	}
	if got := empty.ResolveThinking("writer"); got != "xhigh" {
		t.Errorf("空默认下 writer 覆盖应生效，得 %q", got)
	}
}
