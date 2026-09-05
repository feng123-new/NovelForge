package project

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
)

// Queries use the accepted version/checkpoint pair, never legacy file counters.
// A pointed-to Final without a completed saga is not a completed chapter.
const autopilotCompletedFinalSQL = `SELECT a.chapter,a.version_id,v.content_sha
 FROM chapter_active_finals a
 JOIN chapter_versions v ON v.id=a.version_id AND v.project_id=a.project_id AND v.chapter=a.chapter
 WHERE a.project_id=? AND EXISTS (
 SELECT 1 FROM chapter_version_checkpoints c
 JOIN chapter_finalize_sagas s ON s.operation_id=c.operation_id
 WHERE c.project_id=a.project_id AND c.chapter=a.chapter AND c.version_id=a.version_id
 AND c.final_sha=v.content_sha AND s.project_id=a.project_id AND s.chapter=a.chapter
 AND s.final_version_id=a.version_id AND s.state='completed')`

func (r *Repository) autopilotDatabase(ctx context.Context, id string) (*sql.DB, error) {
	versions, err := r.OpenChapterVersionStore(ctx, id)
	if err != nil {
		return nil, err
	}
	if err = versions.Close(); err != nil {
		return nil, err
	}
	entry, err := r.find(id)
	if err != nil {
		return nil, err
	}
	return migrate.Open(filepath.Join(entry.Root, projectDatabaseRelative), 5*time.Second)
}

func (r *Repository) AutopilotFinalComplete(ctx context.Context, id string, chapter int) (bool, error) {
	if chapter < 1 {
		return false, ErrValidation
	}
	db, err := r.autopilotDatabase(ctx, id)
	if err != nil {
		return false, err
	}
	defer db.Close()
	var n int
	var version, hash string
	err = db.QueryRowContext(ctx, autopilotCompletedFinalSQL+` AND a.chapter=?`, id, chapter).Scan(&n, &version, &hash)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// AutopilotNextChapter finds the first gap in the proved contiguous prefix.
// It does not pretend that a raw .md file or a future Final fills that gap.
func (r *Repository) AutopilotNextChapter(ctx context.Context, id string) (int, error) {
	db, err := r.autopilotDatabase(ctx, id)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, autopilotCompletedFinalSQL+` ORDER BY a.chapter LIMIT 1001`, id)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	next := 1
	for rows.Next() {
		var n int
		var version, hash string
		if err = rows.Scan(&n, &version, &hash); err != nil {
			return 0, err
		}
		if n != next {
			break
		}
		next++
	}
	return next, rows.Err()
}

// AutopilotFingerprint is an opaque invalidation token, not story context.
// All authority-event high-water marks are included conservatively: even an
// unrelated later edit may require review, but can never leak future content.
// Acquiring the project execution lease is the caller's responsibility.
func (r *Repository) AutopilotFingerprint(ctx context.Context, id string, chapter int) (string, error) {
	if chapter < 1 || chapter > 1000 {
		return "", ErrValidation
	}
	db, err := r.autopilotDatabase(ctx, id)
	if err != nil {
		return "", err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var truth, foreshadows, secrets int64
	err = tx.QueryRowContext(ctx, `SELECT
 (SELECT coalesce(max(sequence),0) FROM truth_events),
 (SELECT coalesce(max(sequence),0) FROM foreshadow_events),
 (SELECT coalesce(max(sequence),0) FROM secret_events)`).Scan(&truth, &foreshadows, &secrets)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	encoder := json.NewEncoder(h)
	if err = encoder.Encode([]any{id, chapter, truth, foreshadows, secrets}); err != nil {
		return "", err
	}
	rows, err := tx.QueryContext(ctx, `SELECT chapter,version_id,authority FROM chapter_active_finals WHERE project_id=? AND chapter<? ORDER BY chapter LIMIT 1001`, id, chapter)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var n int
		var version, authority string
		if err = rows.Scan(&n, &version, &authority); err != nil {
			rows.Close()
			return "", err
		}
		if err = encoder.Encode([]any{n, version, authority}); err != nil {
			rows.Close()
			return "", err
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	var incomplete int
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM derived_state_rebuilds WHERE project_id=? AND boundary_chapter<=? AND state!='completed'`, id, chapter).Scan(&incomplete)
	if err != nil {
		return "", err
	}
	if incomplete > 0 {
		return "", fmt.Errorf("accepted state has an incomplete rebuild")
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
