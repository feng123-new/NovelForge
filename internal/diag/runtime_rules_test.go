package diag

import "testing"

// TestRuntimeFindings_Classify 证明重复签名按形态分类、阈值升降级正确，
// 且运行时 Finding 全部 AutoNone（观察者纪律：只诊断不产 Action）。
func TestRuntimeFindings_Classify(t *testing.T) {
	rc := RuntimeCapture{
		Repeats: []RepeatStat{
			{Sig: "coordinator · err: InputValidationError", Count: 14}, // 错误循环 critical
			{Sig: "coordinator · commit_chapter", Count: 5},             // 工具空转 warning
			{Sig: "writer · save_plan (args invalid)", Count: 4},        // 参数无效 warning
		},
		StuckStep:  "writing.commit_ch07",
		StuckCount: 9, // 卡住 critical
		LogKinds:   map[string]int{"stream_idle": 4},
		LogErrors:  12,
	}

	fs := runtimeFindings(&rc)
	sev := map[string]Severity{}
	for _, f := range fs {
		sev[f.Rule] = f.Severity
		if f.AutoLevel != AutoNone {
			t.Errorf("%s 应为 AutoNone（观察者纪律），got %s", f.Rule, f.AutoLevel)
		}
	}

	want := map[string]Severity{
		"RepeatedToolError": SevCritical,
		"RepeatedToolCall":  SevWarning,
		"ArgsInvalidLoop":   SevWarning,
		"StuckStep":         SevCritical,
		"StreamIdleStorm":   SevWarning,
		"LogErrorBurst":     SevWarning,
	}
	for rule, w := range want {
		if sev[rule] != w {
			t.Errorf("%s: got %q want %q", rule, sev[rule], w)
		}
	}
}

// TestRuntimeFindings_Quiet 证明无异常信号时不产任何运行时 Finding（零误报）。
func TestRuntimeFindings_Quiet(t *testing.T) {
	rc := RuntimeCapture{
		LogKinds:  map[string]int{"stream_idle": 1}, // 低于阈值
		LogErrors: 2,
	}
	if fs := runtimeFindings(&rc); len(fs) != 0 {
		t.Errorf("安静态不应产 Finding，got %d: %+v", len(fs), fs)
	}
}
