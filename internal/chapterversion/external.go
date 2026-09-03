package chapterversion

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/voocel/ainovel-cli/internal/domain"
)

const maxExternalChapterBytes = 2 << 20

var chapterDigits = regexp.MustCompile(`\d+`)

func chapterNumberFromName(name string) (int, bool) {
	match := chapterDigits.FindString(name)
	if match == "" {
		return 0, false
	}
	var chapter int
	if _, err := fmt.Sscanf(match, "%d", &chapter); err != nil || chapter < 1 {
		return 0, false
	}
	return chapter, true
}

func (s *Store) chapterFile(chapter int) (string, error) {
	root := filepath.Join(s.root, "chapters")
	info, err := os.Lstat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", newError(CodeNotFound, "chapter file was not found", false, ErrNotFound)
		}
		return "", newError(CodeStorage, "chapter directory could not be inspected", true, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", newError(CodeUnsafePath, "chapter directory is unsafe", false, nil)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", newError(CodeStorage, "chapter directory could not be read", true, err)
	}
	matches := []string{}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || strings.HasSuffix(entry.Name(), ".quality-backup") {
			continue
		}
		if n, ok := chapterNumberFromName(entry.Name()); ok && n == chapter {
			matches = append(matches, entry.Name())
		}
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return "", newError(CodeNotFound, "chapter file was not found", false, ErrNotFound)
	}
	if len(matches) != 1 {
		return "", newError(CodeConflict, "multiple chapter files resolve to the same chapter", false, nil)
	}
	path := filepath.Join(root, matches[0])
	info, err = os.Lstat(path)
	if err != nil {
		return "", newError(CodeStorage, "chapter file could not be inspected", true, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", newError(CodeUnsafePath, "chapter file is unsafe", false, nil)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", newError(CodeUnsafePath, "chapter directory could not be resolved safely", false, err)
	}
	resolvedFile, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", newError(CodeUnsafePath, "chapter file could not be resolved safely", false, err)
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedFile)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", newError(CodeUnsafePath, "chapter file escapes the project boundary", false, nil)
	}
	return path, nil
}

func (s *Store) readExternal(chapter int) (string, string, error) {
	path, err := s.chapterFile(chapter)
	if err != nil {
		return "", "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", "", newError(CodeStorage, "chapter file could not be opened", true, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", "", newError(CodeStorage, "chapter file could not be inspected", true, err)
	}
	if info.Size() > maxExternalChapterBytes {
		return "", "", newError(CodeExternalTooLarge, "chapter file exceeds the external sync size limit", false, nil)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxExternalChapterBytes+1))
	if err != nil {
		return "", "", newError(CodeStorage, "chapter file could not be read", true, err)
	}
	if len(data) > maxExternalChapterBytes {
		return "", "", newError(CodeExternalTooLarge, "chapter file exceeds the external sync size limit", false, nil)
	}
	if !utf8.Valid(data) {
		return "", "", newError(CodeExternalEncoding, "chapter file must be valid UTF-8", false, nil)
	}
	content := domain.NormalizeChapterContent(string(data))
	return content, domain.ChapterContentSHA256(content), nil
}

// DetectExternal compares the file only with the immutable active-final hash.
// It records divergence explicitly and never converts file contents into Truth
// or an active version on its own.
func (s *Store) DetectExternal(ctx context.Context, chapter int) (SyncStatus, error) {
	active, err := s.ActiveFinal(ctx, chapter, false)
	if err != nil {
		return SyncStatus{}, err
	}
	status := SyncStatus{ProjectID: s.projectID, Chapter: chapter}
	if active == nil {
		return status, nil
	}
	_, observed, err := s.readExternal(chapter)
	if err != nil {
		return SyncStatus{}, err
	}
	now := s.now().UTC()
	status.ActiveVersionID = active.ID
	status.ExpectedSHA = active.ContentSHA
	status.ObservedSHA = observed
	status.ObservedAt = now
	status.SyncRequired = active.ContentSHA != observed
	_, err = s.db.ExecContext(ctx, `INSERT INTO chapter_external_state(project_id,chapter,active_version_id,expected_sha,observed_sha,observed_at,sync_required)
		VALUES(?,?,?,?,?,?,?) ON CONFLICT(project_id,chapter) DO UPDATE SET
		active_version_id=excluded.active_version_id,expected_sha=excluded.expected_sha,observed_sha=excluded.observed_sha,
		observed_at=excluded.observed_at,sync_required=excluded.sync_required`, s.projectID, chapter, active.ID, active.ContentSHA, observed,
		now.Format(time.RFC3339Nano), boolInt(status.SyncRequired))
	if err != nil {
		return SyncStatus{}, newError(CodeStorage, "external chapter state could not be recorded", true, err)
	}
	if status.SyncRequired {
		payload := []byte(fmt.Sprintf(`{"expected_sha":%q,"observed_sha":%q}`, active.ContentSHA, observed))
		_ = s.AppendEvent(ctx, chapter, active.ID, "external_change_detected", "chapter file differs from active final", payload)
	}
	return status, nil
}

func (s *Store) SyncStatus(ctx context.Context, chapter int, detect bool) (SyncStatus, error) {
	if detect {
		return s.DetectExternal(ctx, chapter)
	}
	var status SyncStatus
	var observedAt string
	var required int
	err := s.db.QueryRowContext(ctx, `SELECT project_id,chapter,active_version_id,expected_sha,observed_sha,observed_at,sync_required
		FROM chapter_external_state WHERE project_id=? AND chapter=?`, s.projectID, chapter).
		Scan(&status.ProjectID, &status.Chapter, &status.ActiveVersionID, &status.ExpectedSHA, &status.ObservedSHA, &observedAt, &required)
	if errors.Is(err, os.ErrNotExist) {
		return SyncStatus{ProjectID: s.projectID, Chapter: chapter}, nil
	}
	if err != nil {
		// sql.ErrNoRows is intentionally translated to an empty status.
		if strings.Contains(err.Error(), "no rows") {
			return SyncStatus{ProjectID: s.projectID, Chapter: chapter}, nil
		}
		return SyncStatus{}, newError(CodeStorage, "external chapter state could not be read", true, err)
	}
	if observedAt != "" {
		if parsed, parseErr := time.Parse(time.RFC3339Nano, observedAt); parseErr == nil {
			status.ObservedAt = parsed.UTC()
		}
	}
	status.SyncRequired = required == 1
	return status, nil
}

func (s *Store) ClearSyncRequired(ctx context.Context, chapter int, active Version) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO chapter_external_state(project_id,chapter,active_version_id,expected_sha,observed_sha,observed_at,sync_required)
		VALUES(?,?,?,?,?,?,0) ON CONFLICT(project_id,chapter) DO UPDATE SET active_version_id=excluded.active_version_id,
		expected_sha=excluded.expected_sha,observed_sha=excluded.observed_sha,observed_at=excluded.observed_at,sync_required=0`,
		s.projectID, chapter, active.ID, active.ContentSHA, active.ContentSHA, s.now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return newError(CodeStorage, "external sync state could not be cleared", true, err)
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
