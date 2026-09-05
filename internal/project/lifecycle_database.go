package project

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/lifecycle"
)

func lifecycleReadDB(filename string) (*sql.DB, error) {
	p, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}
	p = filepath.ToSlash(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	u := url.URL{Scheme: "file", Path: p}
	q := url.Values{}
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(1)")
	q.Add("_pragma", "trusted_schema(0)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}
func lifecycleSchema(ctx context.Context, db *sql.DB) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT type,name,tbl_name,coalesce(sql,'') FROM sqlite_schema ORDER BY type,name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var kind, name, table, statement string
		if err = rows.Scan(&kind, &name, &table, &statement); err != nil {
			return nil, err
		}
		out[kind+":"+name] = table + "\n" + statement
	}
	return out, rows.Err()
}
func lifecycleMigrationRows(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT version,name,checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n int
		var name, sum string
		if err = rows.Scan(&n, &name, &sum); err != nil {
			return nil, err
		}
		out = append(out, fmt.Sprintf("%d:%s:%s", n, name, sum))
	}
	return out, rows.Err()
}

// Uploaded SQL is never applied. Compare every application table, index,
// trigger and view with a pristine database built from our own migration code
// before querying application tables or publishing the restored project.
func verifyLifecycleDB(ctx context.Context, filename string, m lifecycle.Manifest) error {
	if m.Schema < 1 || m.Schema > 10 {
		return lifecycle.ErrInvalid
	}
	dir, err := os.MkdirTemp("", "novelforge-schema-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	var migrations []migrate.Migration
	for _, v := range projectMigrations {
		if v.Version <= m.Schema {
			migrations = append(migrations, v)
		}
	}
	expectedFile := filepath.Join(dir, "expected.db")
	if err = (migrate.Runner{Path: expectedFile, Migrations: migrations}).Run(ctx); err != nil {
		return err
	}
	expected, err := lifecycleReadDB(expectedFile)
	if err != nil {
		return err
	}
	defer expected.Close()
	actual, err := lifecycleReadDB(filename)
	if err != nil {
		return err
	}
	defer actual.Close()
	a, err := lifecycleSchema(ctx, actual)
	if err != nil {
		return lifecycle.ErrInvalid
	}
	e, err := lifecycleSchema(ctx, expected)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(a, e) {
		return lifecycle.ErrInvalid
	}
	am, err := lifecycleMigrationRows(ctx, actual)
	if err != nil {
		return lifecycle.ErrInvalid
	}
	em, err := lifecycleMigrationRows(ctx, expected)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(am, em) {
		return lifecycle.ErrInvalid
	}
	var check string
	if err = actual.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&check); err != nil || check != "ok" {
		return lifecycle.ErrInvalid
	}
	rows, err := actual.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return lifecycle.ErrInvalid
	}
	invalid := rows.Next()
	readErr := rows.Err()
	rows.Close()
	if invalid || readErr != nil {
		return lifecycle.ErrInvalid
	}
	var id, title string
	var count int
	if err = actual.QueryRowContext(ctx, `SELECT count(*) FROM project_metadata`).Scan(&count); err != nil || count != 1 {
		return lifecycle.ErrInvalid
	}
	if err = actual.QueryRowContext(ctx, `SELECT id,title FROM project_metadata`).Scan(&id, &title); err != nil || id != m.ProjectID || title != m.Title {
		return lifecycle.ErrInvalid
	}
	// A portable project cannot mix different project identities in its tables.
	for key := range e {
		if !strings.HasPrefix(key, "table:") {
			continue
		}
		table := strings.TrimPrefix(key, "table:")
		quoted := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
		cols, err := actual.QueryContext(ctx, "PRAGMA table_info("+quoted+")")
		if err != nil {
			return err
		}
		hasProject := false
		for cols.Next() {
			var cid, notnull, pk int
			var name, typ string
			var def any
			if err = cols.Scan(&cid, &name, &typ, &notnull, &def, &pk); err != nil {
				cols.Close()
				return err
			}
			if name == "project_id" {
				hasProject = true
			}
		}
		err = cols.Err()
		cols.Close()
		if err != nil {
			return err
		}
		if hasProject {
			if err = actual.QueryRowContext(ctx, "SELECT count(*) FROM "+quoted+" WHERE project_id != ?", m.ProjectID).Scan(&count); err != nil || count != 0 {
				return lifecycle.ErrInvalid
			}
		}
	}
	if _, ok := a["table:chapter_versions"]; ok {
		rows, err := actual.QueryContext(ctx, `SELECT content,content_sha FROM chapter_versions`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var text, hash string
			if err = rows.Scan(&text, &hash); err != nil {
				rows.Close()
				return err
			}
			if domain.ChapterContentSHA256(text) != hash {
				rows.Close()
				return lifecycle.ErrInvalid
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	if _, ok := a["table:chapter_finalize_sagas"]; ok {
		if err = actual.QueryRowContext(ctx, `SELECT count(*) FROM chapter_finalize_sagas WHERE state!='completed'`).Scan(&count); err != nil || count != 0 {
			return lifecycle.ErrConflict
		}
		if err = actual.QueryRowContext(ctx, `SELECT count(*) FROM derived_state_rebuilds WHERE state!='completed'`).Scan(&count); err != nil || count != 0 {
			return lifecycle.ErrConflict
		}
	}
	return nil
}

func verifyLifecycleFiles(ctx context.Context, filename string, m lifecycle.Manifest, files map[string][]byte) error {
	db, err := lifecycleReadDB(filename)
	if err != nil {
		return err
	}
	defer db.Close()
	schema, err := lifecycleSchema(ctx, db)
	if err != nil {
		return err
	}
	if _, ok := schema["table:chapter_active_finals"]; !ok {
		return nil
	}
	rows, err := db.QueryContext(ctx, autopilotCompletedFinalSQL+` ORDER BY a.chapter`, m.ProjectID)
	if err != nil {
		return err
	}
	count := 0
	for rows.Next() {
		var n int
		var id, hash string
		if err = rows.Scan(&n, &id, &hash); err != nil {
			rows.Close()
			return err
		}
		count++
		matches := 0
		for name, b := range files {
			if strings.HasPrefix(name, "chapters/") && !strings.Contains(strings.TrimPrefix(name, "chapters/"), "/") {
				if num, ok := chapterNumber(filepath.Base(name)); ok && num == n {
					matches++
					if domain.ChapterContentSHA256(string(b)) != hash {
						rows.Close()
						return lifecycle.ErrConflict
					}
				}
			}
		}
		if matches != 1 {
			rows.Close()
			return lifecycle.ErrConflict
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var all int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM chapter_active_finals WHERE project_id=?`, m.ProjectID).Scan(&all); err != nil {
		return err
	}
	if count != all {
		return lifecycle.ErrConflict
	}
	return nil
}
