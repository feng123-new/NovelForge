package project

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/chapterversion"
	"github.com/voocel/ainovel-cli/internal/db/migrate"
	"github.com/voocel/ainovel-cli/internal/lifecycle"
)

func init() { projectMigrations = append(projectMigrations, lifecycle.Migration()) }

// All mutations require the caller to hold AcquireExecution. The HTTP transport
// and Autopilot already share that lease with the retained Host.
func (r *Repository) lifecycleRoot(id string) (entry, error) {
	e, err := r.find(id)
	if err != nil {
		return e, err
	}
	resolved, err := filepath.EvalSymlinks(r.workspace)
	if err != nil || resolved != r.resolvedWorkspace {
		return e, ErrUnsafePath
	}
	for _, p := range []string{e.Root, filepath.Join(e.Root, ".novelforge"), filepath.Join(e.Root, projectMetadataRelative), filepath.Join(e.Root, projectDatabaseRelative)} {
		st, err := os.Lstat(p)
		if err != nil {
			return e, err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return e, ErrUnsafePath
		}
	}
	if e.Metadata.FormatVersion < 1 || e.Metadata.FormatVersion > CurrentFormatVersion {
		return e, lifecycle.ErrInvalid
	}
	return e, nil
}
func (r *Repository) OpenLifecycle(ctx context.Context, id string) (lifecycle.Store, error) {
	e, err := r.lifecycleRoot(id)
	if err != nil {
		return lifecycle.Store{}, err
	}
	if err = r.initializeProjectDatabase(ctx, e.Root); err != nil {
		return lifecycle.Store{}, err
	}
	db, err := migrate.Open(filepath.Join(e.Root, projectDatabaseRelative), 5*time.Second)
	return lifecycle.Store{DB: db, ProjectID: id}, err
}
func (r *Repository) StageManuscript(ctx context.Context, id, name string, data []byte, start int) (lifecycle.Import, error) {
	if start < 1 || start > 1000 {
		return lifecycle.Import{}, lifecycle.ErrInvalid
	}
	b, err := lifecycle.Parse(name, data)
	if err != nil {
		return lifecycle.Import{}, err
	}
	s, err := r.OpenLifecycle(ctx, id)
	if err != nil {
		return lifecycle.Import{}, err
	}
	defer s.DB.Close()
	batch := "imp_" + lifecycle.SHA([]byte(id + "\n" + name + "\n" + lifecycle.SHA(data) + fmt.Sprint(start)))[:32]
	if prior, e := s.Get(ctx, batch); e == nil {
		return prior, nil
	} else if !errors.Is(e, lifecycle.ErrNotFound) {
		return lifecycle.Import{}, e
	}
	// Raw/unversioned prose is evidence too; never overwrite it through import.
	for offset := 0; ; offset += 100 {
		page, err := r.ListChapters(ctx, id, 100, offset)
		if err != nil {
			return lifecycle.Import{}, err
		}
		for _, c := range page.Chapters {
			if c.Chapter >= start && c.Chapter < start+len(b.Chapters) {
				return lifecycle.Import{}, lifecycle.ErrConflict
			}
		}
		if len(page.Chapters) < 100 {
			break
		}
	}
	return s.Begin(ctx, name, data, start)
}

type ImportEvaluator func(context.Context, int, string, string) error

func (r *Repository) StepManuscript(ctx context.Context, id, batch string, n int, analyze bool, evaluate ImportEvaluator) (lifecycle.ImportChapter, error) {
	s, err := r.OpenLifecycle(ctx, id)
	if err != nil {
		return lifecycle.ImportChapter{}, err
	}
	defer s.DB.Close()
	c, err := s.Chapter(ctx, batch, n)
	if err != nil {
		return c, err
	}
	if c.VersionID == "" {
		versions, err := r.OpenChapterVersionStore(ctx, id)
		if err != nil {
			return c, err
		}
		defer versions.Close()
		active, err := versions.ActiveFinal(ctx, n, false)
		if err != nil {
			return c, err
		}
		if active != nil {
			return c, lifecycle.ErrConflict
		}
		v, err := (&chapterversion.Service{Store: versions}).SaveHuman(ctx, n, "import:"+batch+fmt.Sprintf(":%d", n), c.Text)
		if err != nil {
			return c, err
		}
		c.VersionID = v.ID
		if err = s.Progress(ctx, batch, n, v.ID, "saved", ""); err != nil {
			return c, err
		}
		c.State = "saved"
	}
	if analyze && c.State != "analyzed" {
		if evaluate == nil {
			return c, lifecycle.ErrInvalid
		}
		if err = evaluate(ctx, n, "import-check:"+c.VersionID, c.VersionID); err != nil {
			if persistErr := s.Progress(context.WithoutCancel(ctx), batch, n, c.VersionID, "saved", "IMPORT_ANALYSIS_FAILED"); persistErr != nil {
				return c, persistErr
			}
			return c, err
		}
		if err = s.Progress(ctx, batch, n, c.VersionID, "analyzed", ""); err != nil {
			return c, err
		}
		c.State = "analyzed"
		c.ErrorCode = ""
	}
	return c, nil
}

func (r *Repository) ImportedChapterPending(ctx context.Context, id string, n int) (bool, error) {
	s, err := r.OpenLifecycle(ctx, id)
	if err != nil {
		return false, err
	}
	defer s.DB.Close()
	var count int
	err = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM manuscript_import_chapters WHERE project_id=? AND chapter=?`, id, n).Scan(&count)
	return count > 0, err
}

// Only an explicit contiguous range of proved Finals is exported. Drafts,
// rejected candidates, gaps and unsynchronized external edits never leak in.
func (r *Repository) ExportManuscript(ctx context.Context, id, format string, from, to int) ([]byte, string, error) {
	e, err := r.lifecycleRoot(id)
	if err != nil {
		return nil, "", err
	}
	versions, err := r.OpenChapterVersionStore(ctx, id)
	if err != nil {
		return nil, "", err
	}
	defer versions.Close()
	if from == 0 {
		from = 1
	}
	if to == 0 {
		err = versions.Database().QueryRowContext(ctx, `SELECT coalesce(max(chapter),0) FROM chapter_active_finals WHERE project_id=?`, id).Scan(&to)
		if err != nil {
			return nil, "", err
		}
	}
	if from < 1 || to < from || to > 1000 {
		return nil, "", lifecycle.ErrInvalid
	}
	b := lifecycle.Book{Title: e.Project.Title, Language: e.Project.Language, Chapters: []lifecycle.Chapter{}}
	for n := from; n <= to; n++ {
		complete, err := r.AutopilotFinalComplete(ctx, id, n)
		if err != nil {
			return nil, "", err
		}
		if !complete {
			return nil, "", lifecycle.ErrConflict
		}
		status, err := versions.DetectExternal(ctx, n)
		if err != nil {
			return nil, "", err
		}
		if status.SyncRequired {
			return nil, "", lifecycle.ErrConflict
		}
		v, err := versions.ActiveFinal(ctx, n, true)
		if err != nil {
			return nil, "", err
		}
		if v == nil {
			return nil, "", lifecycle.ErrConflict
		}
		title := fmt.Sprintf("第%d章", n)
		var imported string
		err = versions.Database().QueryRowContext(ctx, `SELECT title FROM manuscript_import_chapters WHERE project_id=? AND chapter=?`, id, n).Scan(&imported)
		if err == nil && strings.TrimSpace(imported) != "" {
			title = imported
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, "", err
		}
		b.Chapters = append(b.Chapters, lifecycle.Chapter{Number: n, Title: title, Text: v.Content})
	}
	return lifecycle.Export(b, format)
}
