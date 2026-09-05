package authoring

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Finding struct {
	Code    string `json:"code"`
	Phrase  string `json:"phrase"`
	Count   int    `json:"count"`
	Limit   int    `json:"limit"`
	Message string `json:"message"`
}
type Report struct {
	Findings  []Finding `json:"findings"`
	Advisory  bool      `json:"advisory"`
	Truncated bool      `json:"truncated"`
}

func sentenceKey(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}
func sentences(text string, min int) map[string]int {
	out := map[string]int{}
	for _, s := range strings.FieldsFunc(text, func(r rune) bool { return strings.ContainsRune("。！？!?\n.;；", r) }) {
		key := sentenceKey(s)
		n := utf8.RuneCountInString(key)
		if n >= min && n <= 1000 {
			out[key]++
		}
	}
	return out
}
func (r Rules) Evaluate(text string, previous []string) Report {
	report := Report{Findings: []Finding{}, Advisory: true}
	if !r.Enabled {
		return report
	}
	add := func(code, phrase string, count, limit int) {
		if len(report.Findings) >= 64 {
			report.Truncated = true
			return
		}
		runes := []rune(phrase)
		if len(runes) > 100 {
			phrase = string(runes[:100]) + "…"
		}
		report.Findings = append(report.Findings, Finding{Code: code, Phrase: phrase, Count: count, Limit: limit, Message: fmt.Sprintf("%s: %q occurred %d times (configured limit %d); review intent before changing it", code, phrase, count, limit)})
	}
	lower := strings.ToLower(text)
	for _, p := range r.Phrases {
		n := strings.Count(lower, strings.ToLower(p))
		if n > r.MaxPhraseOccurrences {
			add("PHRASE_OVERUSE", p, n, r.MaxPhraseOccurrences)
		}
	}
	current := sentences(text, r.MinSentenceRunes)
	history := map[string]int{}
	for _, body := range previous {
		for p, n := range sentences(body, r.MinSentenceRunes) {
			history[p] += n
		}
	}
	keys := make([]string, 0, len(current))
	for p := range current {
		keys = append(keys, p)
	}
	sort.Strings(keys)
	for _, p := range keys {
		n := current[p]
		if n > r.MaxSentenceRepeats {
			add("SENTENCE_REPEATED", p, n, r.MaxSentenceRepeats)
		}
		if history[p] > 0 {
			add("RECENT_SENTENCE_REUSED", p, n+history[p], 1)
		}
	}
	return report
}
