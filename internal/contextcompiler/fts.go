package contextcompiler

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
)

const migrationSQL = `
CREATE TABLE context_documents (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    source_chapter INTEGER NOT NULL CHECK(source_chapter >= 0),
    source_version TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_context_documents_project_chapter
    ON context_documents(project_id, source_chapter DESC, kind, id);
CREATE VIRTUAL TABLE context_documents_fts USING fts5(
    title,
    content,
    content='context_documents',
    content_rowid='rowid',
    tokenize='unicode61'
);
CREATE TRIGGER context_documents_ai AFTER INSERT ON context_documents BEGIN
    INSERT INTO context_documents_fts(rowid, title, content)
    VALUES (new.rowid, new.title, new.content);
END;
CREATE TRIGGER context_documents_ad AFTER DELETE ON context_documents BEGIN
    INSERT INTO context_documents_fts(context_documents_fts, rowid, title, content)
    VALUES ('delete', old.rowid, old.title, old.content);
END;
CREATE TRIGGER context_documents_au AFTER UPDATE ON context_documents BEGIN
    INSERT INTO context_documents_fts(context_documents_fts, rowid, title, content)
    VALUES ('delete', old.rowid, old.title, old.content);
    INSERT INTO context_documents_fts(rowid, title, content)
    VALUES (new.rowid, new.title, new.content);
END;
`

func Migration() migrate.Migration {
	return migrate.Migration{Version: 5, Name: "context_compiler_fts5", SQL: migrationSQL}
}

type Document struct {
	ID            string
	ProjectID     string
	Kind          string
	Title         string
	Content       string
	SourceChapter int
	SourceVersion string
	Priority      int
	CreatedAt     time.Time
}

type FTSStore struct{ db *sql.DB }

func NewFTSStore(db *sql.DB) *FTSStore { return &FTSStore{db: db} }

func (s *FTSStore) Upsert(ctx context.Context, document Document) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("contextcompiler: nil FTS store")
	}
	document.ID = strings.TrimSpace(document.ID)
	document.ProjectID = strings.TrimSpace(document.ProjectID)
	document.Kind = strings.TrimSpace(document.Kind)
	document.Title = strings.TrimSpace(document.Title)
	document.Content = strings.TrimSpace(document.Content)
	document.SourceVersion = strings.TrimSpace(document.SourceVersion)
	if document.ID == "" || document.ProjectID == "" || document.Kind == "" || document.Content == "" || document.SourceVersion == "" {
		return fmt.Errorf("contextcompiler: document id, project, kind, content and source_version are required")
	}
	if document.SourceChapter < 0 {
		return fmt.Errorf("contextcompiler: source_chapter must be non-negative")
	}
	if document.CreatedAt.IsZero() {
		document.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO context_documents(id, project_id, kind, title, content, source_chapter, source_version, priority, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    project_id=excluded.project_id,
    kind=excluded.kind,
    title=excluded.title,
    content=excluded.content,
    source_chapter=excluded.source_chapter,
    source_version=excluded.source_version,
    priority=excluded.priority,
    created_at=excluded.created_at`,
		document.ID, document.ProjectID, document.Kind, document.Title, document.Content,
		document.SourceChapter, document.SourceVersion, document.Priority,
		document.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("contextcompiler: upsert FTS document: %w", err)
	}
	return nil
}

func (s *FTSStore) Delete(ctx context.Context, projectID, id string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("contextcompiler: nil FTS store")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM context_documents WHERE project_id=? AND id=?`, strings.TrimSpace(projectID), strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("contextcompiler: delete FTS document: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *FTSStore) Collect(ctx context.Context, request Request) ([]Item, error) {
	if s == nil || s.db == nil || strings.TrimSpace(request.Query) == "" {
		return nil, nil
	}
	limit := 20
	query := quoteFTSQuery(request.Query)
	if query == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT d.id, d.kind, d.title, d.content, d.source_chapter, d.source_version, d.priority, bm25(context_documents_fts)
FROM context_documents_fts
JOIN context_documents d ON d.rowid=context_documents_fts.rowid
WHERE context_documents_fts MATCH ?
  AND d.project_id=?
  AND d.source_chapter<=?
ORDER BY bm25(context_documents_fts), d.source_chapter DESC, d.id
LIMIT ?`, query, request.ProjectID, request.Chapter, limit)
	if err != nil {
		return nil, fmt.Errorf("contextcompiler: FTS search: %w", err)
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		var item Item
		var rank float64
		if err := rows.Scan(&item.ID, &item.Kind, &item.Title, &item.Content, &item.SourceChapter, &item.SourceVersion, &item.Priority, &rank); err != nil {
			return nil, fmt.Errorf("contextcompiler: scan FTS result: %w", err)
		}
		item.Layer = LayerHistorical
		item.Stage = StageFTS5
		item.Metadata = map[string]string{"bm25": fmt.Sprintf("%.8f", rank)}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contextcompiler: iterate FTS results: %w", err)
	}
	return items, nil
}

func quoteFTSQuery(query string) string {
	fields := strings.Fields(query)
	quoted := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(strings.ReplaceAll(field, `"`, `""`))
		if field != "" {
			quoted = append(quoted, `"`+field+`"`)
		}
	}
	return strings.Join(quoted, " AND ")
}
