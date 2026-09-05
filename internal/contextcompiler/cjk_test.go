package contextcompiler

import (
	"strings"
	"testing"
)

func TestCJKPhraseNormalization(t *testing.T) {
	for _, text := range []string{"张三", "青云宗", "玄铁剑", "𠮷野", "AI张三"} {
		if !containsHan(text) {
			t.Fatalf("Han text not recognized: %q", text)
		}
		query := cjkFTSQuery(text)
		if !strings.HasPrefix(query, `"`) || !strings.HasSuffix(query, `"`) {
			t.Fatalf("not a phrase: %q", query)
		}
	}
	if got := strings.Fields(cjkIndexText("张三在青云宗获得玄铁剑")); strings.Join(got, "|") != "张|三|在|青|云|宗|获|得|玄|铁|剑" {
		t.Fatalf("tokens: %v", got)
	}
	if got := cjkIndexText("sealed gate"); got != "sealed gate" {
		t.Fatalf("English changed: %q", got)
	}
	if got := cjkFTSQuery(`张三 "青云宗"`); strings.Count(got, " AND ") != 1 || !strings.Contains(got, `""`) {
		t.Fatalf("query escaping: %q", got)
	}
}
