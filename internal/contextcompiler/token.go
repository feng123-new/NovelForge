package contextcompiler

import (
	"strings"
	"unicode"
)

// TokenCounter allows provider/model-specific counters without coupling the
// deterministic compiler to an LLM SDK.
type TokenCounter interface {
	Count(string) int
}

// HeuristicTokenCounter is deterministic and language-aware. CJK runes count
// individually; contiguous Latin/digit text is charged in four-rune chunks.
type HeuristicTokenCounter struct{}

func (HeuristicTokenCounter) Count(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	tokens := 0
	latinRun := 0
	flushLatin := func() {
		if latinRun > 0 {
			tokens += (latinRun + 3) / 4
			latinRun = 0
		}
	}
	for _, r := range value {
		switch {
		case unicode.Is(unicode.Han, r), unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r), unicode.Is(unicode.Hangul, r):
			flushLatin()
			tokens++
		case unicode.IsLetter(r), unicode.IsDigit(r):
			latinRun++
		case unicode.IsSpace(r):
			flushLatin()
		default:
			flushLatin()
			tokens++
		}
	}
	flushLatin()
	if tokens == 0 {
		return 1
	}
	return tokens
}
