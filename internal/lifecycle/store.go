package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
)

func Migration() migrate.Migration {
	return migrate.Migration{Version: 10, Name: "manuscript_lifecycle", SQL: `
 CREATE TABLE manuscript_imports (
 id TEXT PRIMARY KEY, project_id TEXT NOT NULL, filename TEXT NOT NULL,
 source_sha TEXT NOT NULL, source BLOB NOT NULL, start_chapter INTEGER NOT NULL,
 total INTEGER NOT NULL, created_at TEXT NOT NULL);
 CREATE TABLE manuscript_import_chapters (
 import_id TEXT NOT NULL REFERENCES manuscript_imports(id),project_id TEXT NOT NULL,
 chapter INTEGER NOT NULL,title TEXT NOT NULL,content TEXT NOT NULL,content_sha TEXT NOT NULL,
 version_id TEXT NOT NULL DEFAULT '',state TEXT NOT NULL DEFAULT 'pending'
 CHECK(state IN ('pending','saved','analyzed')), error_code TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(import_id,chapter),UNIQUE(project_id,chapter));
 CREATE INDEX idx_manuscript_import_project ON manuscript_imports(project_id,created_at,id);
 `}
}

type Store struct {
	DB        *sql.DB
	ProjectID string
}
type ImportChapter struct {
	Number    int    `json:"chapter"`
	Title     string `json:"title"`
	Words     int    `json:"characters"`
	VersionID string `json:"version_id"`
	State     string `json:"state"`
	ErrorCode string `json:"error_code,omitempty"`
	Text      string `json:"-"`
}
type Import struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	Filename     string `json:"filename"`
	SourceSHA    string `json:"source_sha"`
	Start        int    `json:"start_chapter"`
	Total        int    `json:"total"`
	Saved        int    `json:"saved"`
	Analyzed     int    `json:"analyzed"`
	NextSave     int    `json:"next_save"`
	NextAnalysis int    `json:"next_analysis"`
	Created      string `json:"created_at"`
}
type ImportPage struct {
	Import   Import          `json:"import"`
	Chapters []ImportChapter `json:"chapters"`
	Limit    int             `json:"limit"`
	Offset   int             `json:"offset"`
}

func (s Store) Begin(ctx context.Context, filename string, source []byte, start int) (Import, error) {
	if s.ProjectID == "" || start < 1 || start > 1000 || len(filename) > 200 || filename != path.Base(filename) || strings.ContainsAny(filename, "\\\x00") {
		return Import{}, ErrInvalid
	}
	b, err := Parse(filename, source)
	if err != nil {
		return Import{}, err
	}
	if start+len(b.Chapters)-1 > 1000 {
		return Import{}, ErrLimit
	}
	id := "imp_" + SHA([]byte(s.ProjectID + "\n" + filename + "\n" + SHA(source) + fmt.Sprint(start)))[:32]
	if old, err := s.Get(ctx, id); err == nil {
		return old, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Import{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Import{}, err
	}
	defer tx.Rollback()
	var occupied int
	err = tx.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM chapter_versions WHERE project_id=? AND chapter BETWEEN ? AND ?)+
 (SELECT count(*) FROM chapter_transactions WHERE project_id=? AND chapter BETWEEN ? AND ?)+
 (SELECT count(*) FROM manuscript_import_chapters WHERE project_id=? AND chapter BETWEEN ? AND ?)`, s.ProjectID, start, start+len(b.Chapters)-1, s.ProjectID, start, start+len(b.Chapters)-1, s.ProjectID, start, start+len(b.Chapters)-1).Scan(&occupied)
	if err != nil {
		return Import{}, err
	}
	if occupied > 0 {
		return Import{}, ErrConflict
	}
	created := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT INTO manuscript_imports VALUES(?,?,?,?,?,?,?,?)`, id, s.ProjectID, filename, SHA(source), source, start, len(b.Chapters), created)
	if err != nil {
		return Import{}, err
	}
	for i, c := range b.Chapters {
		_, err = tx.ExecContext(ctx, `INSERT INTO manuscript_import_chapters(import_id,project_id,chapter,title,content,content_sha) VALUES(?,?,?,?,?,?)`, id, s.ProjectID, start+i, c.Title, c.Text, SHA([]byte(c.Text)))
		if err != nil {
			return Import{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Import{}, err
	}
	return s.Get(ctx, id)
}
func (s Store) Get(ctx context.Context, id string) (Import, error) {
	var v Import
	err := s.DB.QueryRowContext(ctx, `SELECT id,project_id,filename,source_sha,start_chapter,total,created_at,
 (SELECT count(*) FROM manuscript_import_chapters c WHERE c.import_id=i.id AND c.version_id!=''),
 (SELECT count(*) FROM manuscript_import_chapters c WHERE c.import_id=i.id AND c.state='analyzed'),
 (SELECT coalesce(min(chapter),0) FROM manuscript_import_chapters c WHERE c.import_id=i.id AND c.version_id=''),
 (SELECT coalesce(min(chapter),0) FROM manuscript_import_chapters c WHERE c.import_id=i.id AND c.state!='analyzed')
 FROM manuscript_imports i WHERE id=? AND project_id=?`, id, s.ProjectID).Scan(&v.ID, &v.ProjectID, &v.Filename, &v.SourceSHA, &v.Start, &v.Total, &v.Created, &v.Saved, &v.Analyzed, &v.NextSave, &v.NextAnalysis)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return v, err
}
func (s Store) List(ctx context.Context, limit, offset int) ([]Import, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, ErrInvalid
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM manuscript_imports WHERE project_id=? ORDER BY created_at DESC,id LIMIT ? OFFSET ?`, s.ProjectID, limit, offset)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []Import{}
	for _, id := range ids {
		v, e := s.Get(ctx, id)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (s Store) Page(ctx context.Context, id string, limit, offset int) (ImportPage, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return ImportPage{}, ErrInvalid
	}
	v, err := s.Get(ctx, id)
	if err != nil {
		return ImportPage{}, err
	}
	out := ImportPage{Import: v, Chapters: []ImportChapter{}, Limit: limit, Offset: offset}
	rows, err := s.DB.QueryContext(ctx, `SELECT chapter,title,length(content),version_id,state,error_code FROM manuscript_import_chapters WHERE import_id=? AND project_id=? ORDER BY chapter LIMIT ? OFFSET ?`, id, s.ProjectID, limit, offset)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var c ImportChapter
		if err = rows.Scan(&c.Number, &c.Title, &c.Words, &c.VersionID, &c.State, &c.ErrorCode); err != nil {
			return out, err
		}
		out.Chapters = append(out.Chapters, c)
	}
	return out, rows.Err()
}
func (s Store) Chapter(ctx context.Context, id string, n int) (ImportChapter, error) {
	var c ImportChapter
	var hash string
	err := s.DB.QueryRowContext(ctx, `SELECT chapter,title,content,content_sha,version_id,state,error_code FROM manuscript_import_chapters WHERE import_id=? AND project_id=? AND chapter=?`, id, s.ProjectID, n).Scan(&c.Number, &c.Title, &c.Text, &hash, &c.VersionID, &c.State, &c.ErrorCode)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	if SHA([]byte(c.Text)) != hash {
		return c, ErrInvalid
	}
	c.Words = utf8.RuneCountInString(c.Text)
	return c, nil
}
func (s Store) Progress(ctx context.Context, id string, n int, version, state, code string) error {
	if state != "saved" && state != "analyzed" {
		return ErrInvalid
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE manuscript_import_chapters SET version_id=?,state=CASE WHEN state='analyzed' THEN state ELSE ? END,error_code=? WHERE import_id=? AND project_id=? AND chapter=? AND (version_id='' OR version_id=?)`, version, state, code, id, s.ProjectID, n, version)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrConflict
	}
	return nil
}
