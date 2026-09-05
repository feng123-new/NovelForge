package contextcompiler

import (
	"strings"
	"unicode"
)

// cjkIndexText gives each Han rune its own token without changing stored prose.
// Quoting the transformed query as a phrase preserves order and adjacency; a
// two-character name is therefore searchable without an unindexed LIKE scan.
func cjkIndexText(text string) string {
	var out strings.Builder
	out.Grow(len(text))
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			out.WriteByte(' ')
			out.WriteRune(r)
			out.WriteByte(' ')
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func containsHan(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func cjkFTSQuery(query string) string {
	fields := strings.Fields(query)
	quoted := make([]string, 0, len(fields))
	for _, field := range fields {
		quoted = append(quoted, `"`+strings.ReplaceAll(cjkIndexText(field), `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " AND ")
}
