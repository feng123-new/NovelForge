package contextcompiler

import (
	"context"
	"database/sql/driver"
	"fmt"
	"strings"
	"unicode"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
	"modernc.org/sqlite"
)

// CharacterSearchMigration adds a rebuildable character index without changing
// the existing word index, source text, or any earlier migration checksum.
func CharacterSearchMigration() migrate.Migration {
	return migrate.Migration{Version: 8, Name: "context_character_search", SQL: characterSearchSQL}
}

const characterSearchSQL = `
CREATE VIRTUAL TABLE context_documents_fts_characters USING fts5(
    title, content, tokenize='unicode61'
);
INSERT INTO context_documents_fts_characters(rowid,title,content)
SELECT rowid,novelforge_search_characters(title),novelforge_search_characters(content)
FROM context_documents;
CREATE TRIGGER context_documents_characters_ai AFTER INSERT ON context_documents BEGIN
    INSERT INTO context_documents_fts_characters(rowid,title,content)
    VALUES(new.rowid,novelforge_search_characters(new.title),novelforge_search_characters(new.content));
END;
CREATE TRIGGER context_documents_characters_ad AFTER DELETE ON context_documents BEGIN
    DELETE FROM context_documents_fts_characters WHERE rowid=old.rowid;
END;
CREATE TRIGGER context_documents_characters_au AFTER UPDATE ON context_documents BEGIN
    DELETE FROM context_documents_fts_characters WHERE rowid=old.rowid;
    INSERT INTO context_documents_fts_characters(rowid,title,content)
    VALUES(new.rowid,novelforge_search_characters(new.title),novelforge_search_characters(new.content));
END;
`

func containsHan(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) { return true }
	}
	return false
}

// Quoted character phrases preserve order, including two-character names.
// User input is never interpreted as FTS operators.
func quoteCharacterFTSQuery(query string) string {
	phrases := make([]string, 0)
	for _, field := range strings.Fields(query) {
		chars := make([]string, 0)
		for _, r := range field {
			if unicode.IsLetter(r) || unicode.IsNumber(r) { chars = append(chars, string(r)) }
		}
		if len(chars) > 0 { phrases = append(phrases, `"`+strings.Join(chars, " ")+`"`) }
	}
	return strings.Join(phrases, " AND ")
}

func (s *FTSStore) collectCharacters(ctx context.Context, request Request) ([]Item, error) {
	query := quoteCharacterFTSQuery(request.Query)
	if query == "" { return nil, nil }
	rows, err := s.db.QueryContext(ctx, `
SELECT d.id, d.kind, d.title, d.content, d.source_chapter, d.source_version, d.priority,
       bm25(context_documents_fts_characters)
FROM context_documents_fts_characters
JOIN context_documents d ON d.rowid=context_documents_fts_characters.rowid
WHERE context_documents_fts_characters MATCH ?
  AND d.project_id=? AND d.source_chapter<=?
ORDER BY bm25(context_documents_fts_characters), d.source_chapter DESC, d.id
LIMIT 20`, query, request.ProjectID, request.Chapter)
	if err != nil { return nil, fmt.Errorf("contextcompiler: character search: %w", err) }
	defer rows.Close()
	var items []Item
	for rows.Next() {
		var item Item
		var rank float64
		if err := rows.Scan(&item.ID, &item.Kind, &item.Title, &item.Content, &item.SourceChapter, &item.SourceVersion, &item.Priority, &rank); err != nil {
			return nil, fmt.Errorf("contextcompiler: character result: %w", err)
		}
		item.Layer, item.Stage = LayerHistorical, StageFTS5
		item.Metadata = map[string]string{"bm25": fmt.Sprintf("%.8f", rank), "index": "characters"}
		items = append(items, item)
	}
	return items, rows.Err()
}

// Linear transformation is registered before any project database is opened,
// ensuring backfill and every subsequent SQLite connection use the same rules.
func init() {
	sqlite.MustRegisterDeterministicScalarFunction("novelforge_search_characters", 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			if len(args) != 1 { return nil, fmt.Errorf("character search expects one argument") }
			if args[0] == nil { return "", nil }
			text, ok := args[0].(string)
			if !ok { return nil, fmt.Errorf("character search expects text") }
			return characterSearchText(text), nil
		})
}

func characterSearchText(text string) string {
	var out strings.Builder
	out.Grow(len(text) * 2)
	first := true
	for _, r := range text {
		if !first { out.WriteByte(' ') }
		out.WriteRune(r)
		first = false
	}
	return out.String()
}
