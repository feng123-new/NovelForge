package contextcompiler

import (
	"database/sql/driver"
	"fmt"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
	"modernc.org/sqlite"
)

func init() {
	// Registered before any project connection opens, including connections
	// used by ChapterVersion rebuilds that update context_documents directly.
	sqlite.MustRegisterDeterministicScalarFunction("novelforge_cjk_v1", 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			if args[0] == nil {
				return "", nil
			}
			text, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("context index requires text")
			}
			return cjkIndexText(text), nil
		})
}

// CJKMigration is additive: never edit the already-applied Migration 5 checksum.
// The normal-content FTS table stores derived tokens only, not authoritative text.
func CJKMigration() migrate.Migration {
	return migrate.Migration{Version: 8, Name: "context_cjk_phrase_index", SQL: cjkMigrationSQL}
}

const cjkMigrationSQL = `
CREATE VIRTUAL TABLE context_documents_cjk_fts USING fts5(title, content, tokenize='unicode61');
INSERT INTO context_documents_cjk_fts(rowid, title, content)
SELECT rowid, novelforge_cjk_v1(title), novelforge_cjk_v1(content) FROM context_documents;
CREATE TRIGGER context_documents_cjk_ai AFTER INSERT ON context_documents BEGIN
    INSERT INTO context_documents_cjk_fts(rowid, title, content)
    VALUES (new.rowid, novelforge_cjk_v1(new.title), novelforge_cjk_v1(new.content));
END;
CREATE TRIGGER context_documents_cjk_ad AFTER DELETE ON context_documents BEGIN
    DELETE FROM context_documents_cjk_fts WHERE rowid=old.rowid;
END;
CREATE TRIGGER context_documents_cjk_au AFTER UPDATE ON context_documents BEGIN
    DELETE FROM context_documents_cjk_fts WHERE rowid=old.rowid;
    INSERT INTO context_documents_cjk_fts(rowid, title, content)
    VALUES (new.rowid, novelforge_cjk_v1(new.title), novelforge_cjk_v1(new.content));
END;
`
