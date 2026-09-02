package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/qualitygate"
)

func (r *Repository) OpenQualityStore(ctx context.Context, id string) (*qualitygate.Store, error) {
	entry, err := r.find(id)
	if err != nil {
		return nil, err
	}
	if err := r.initializeProjectDatabase(ctx, entry.Root); err != nil {
		return nil, err
	}
	store, err := qualitygate.OpenExisting(filepath.Join(entry.Root, projectDatabaseRelative), 5*time.Second)
	if err != nil {
		return nil, newError("PROJECT_QUALITY_STORE_ERROR", "project quality store could not be opened", err)
	}
	return store, nil
}

// WriteFinalChapter implements the qualitygate FinalChapterWriter boundary. It
// resolves the opaque project ID inside Repository, rejects chapter-directory
// symlinks, writes a same-directory temporary file, fsyncs it, and atomically
// switches the final file. A retry with the same content hash is a no-op.
func (r *Repository) WriteFinalChapter(ctx context.Context, projectID string, chapter int, content, expectedSHA string) error {
	_ = ctx
	if chapter <= 0 || content == "" || expectedSHA == "" {
		return newError("CHAPTER_VALIDATION_FAILED", "chapter, content and content hash are required", ErrValidation)
	}
	sum := sha256.Sum256([]byte(content))
	if actual := hex.EncodeToString(sum[:]); actual != expectedSHA {
		return newError("CHAPTER_HASH_MISMATCH", "final chapter hash does not match content", ErrValidation)
	}
	entry, err := r.find(projectID)
	if err != nil {
		return err
	}
	chapterRoot := filepath.Join(entry.Root, "chapters")
	if info, statErr := os.Lstat(chapterRoot); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return newError("CHAPTER_PATH_UNSAFE", "chapter directory is not safe", ErrValidation)
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return newError("CHAPTER_WRITE_FAILED", "chapter directory could not be inspected", statErr)
	}
	if err := os.MkdirAll(chapterRoot, 0o755); err != nil {
		return newError("CHAPTER_WRITE_FAILED", "chapter directory could not be created", err)
	}
	if err := recoverFinalChapterBackup(chapterRoot, chapter); err != nil {
		return newError("CHAPTER_WRITE_FAILED", "interrupted chapter switch could not be recovered", err)
	}
	name, err := finalChapterName(chapterRoot, chapter)
	if err != nil {
		return newError("CHAPTER_WRITE_FAILED", "chapter file is ambiguous", err)
	}
	path := filepath.Join(chapterRoot, name)
	backupPath := path + ".quality-backup"
	if _, backupErr := os.Lstat(backupPath); backupErr == nil {
		if _, targetErr := os.Lstat(path); errors.Is(targetErr, os.ErrNotExist) {
			if err := os.Rename(backupPath, path); err != nil {
				return newError("CHAPTER_WRITE_FAILED", "interrupted chapter switch could not be recovered", err)
			}
		} else if targetErr == nil {
			if err := os.Remove(backupPath); err != nil {
				return newError("CHAPTER_WRITE_FAILED", "stale chapter backup could not be removed", err)
			}
		} else {
			return newError("CHAPTER_WRITE_FAILED", "chapter target could not be inspected during recovery", targetErr)
		}
	} else if !errors.Is(backupErr, os.ErrNotExist) {
		return newError("CHAPTER_WRITE_FAILED", "chapter recovery state could not be inspected", backupErr)
	}
	if info, statErr := os.Lstat(path); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return newError("CHAPTER_PATH_UNSAFE", "chapter target is not a regular file", ErrValidation)
		}
		current, readErr := os.ReadFile(path)
		if readErr == nil {
			currentSum := sha256.Sum256(current)
			if hex.EncodeToString(currentSum[:]) == expectedSHA {
				return nil
			}
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return newError("CHAPTER_WRITE_FAILED", "chapter target could not be inspected", statErr)
	}
	temporary, err := os.CreateTemp(chapterRoot, ".final-*.tmp")
	if err != nil {
		return newError("CHAPTER_WRITE_FAILED", "temporary final chapter could not be created", err)
	}
	tempName := temporary.Name()
	defer os.Remove(tempName)
	if _, err := temporary.WriteString(content); err != nil {
		_ = temporary.Close()
		return newError("CHAPTER_WRITE_FAILED", "final chapter could not be written", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return newError("CHAPTER_WRITE_FAILED", "final chapter could not be synchronized", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return newError("CHAPTER_WRITE_FAILED", "final chapter permissions could not be set", err)
	}
	if err := temporary.Close(); err != nil {
		return newError("CHAPTER_WRITE_FAILED", "final chapter could not be closed", err)
	}
	if _, statErr := os.Lstat(path); errors.Is(statErr, os.ErrNotExist) {
		if err := os.Rename(tempName, path); err != nil {
			return newError("CHAPTER_WRITE_FAILED", "final chapter could not be switched", err)
		}
		return nil
	} else if statErr != nil {
		return newError("CHAPTER_WRITE_FAILED", "chapter target could not be inspected before switch", statErr)
	}
	// Windows does not replace an existing file with os.Rename. Keep a
	// deterministic same-directory backup so a process crash between the two
	// renames is recoverable on the next invocation.
	if err := os.Rename(path, backupPath); err != nil {
		return newError("CHAPTER_WRITE_FAILED", "existing chapter could not be staged for replacement", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		restoreErr := os.Rename(backupPath, path)
		if restoreErr != nil {
			return newError("CHAPTER_WRITE_FAILED", "final chapter switch failed and requires recovery", errors.Join(err, restoreErr))
		}
		return newError("CHAPTER_WRITE_FAILED", "final chapter could not be switched", err)
	}
	if err := os.Remove(backupPath); err != nil {
		return newError("CHAPTER_WRITE_FAILED", "chapter backup could not be removed after switch", err)
	}
	return nil
}

func finalChapterName(root string, chapter int) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	matches := []string{}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || strings.HasSuffix(entry.Name(), ".quality-backup") {
			continue
		}
		if number, ok := chapterNumber(entry.Name()); ok && number == chapter {
			matches = append(matches, entry.Name())
		}
	}
	sort.Strings(matches)
	switch len(matches) {
	case 0:
		return fmt.Sprintf("chapter-%04d.md", chapter), nil
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("multiple chapter files resolve to chapter %d", chapter)
	}
}

func recoverFinalChapterBackup(root string, chapter int) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	backups := make([]string, 0, 1)
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".quality-backup") {
			continue
		}
		targetName := strings.TrimSuffix(entry.Name(), ".quality-backup")
		if number, ok := chapterNumber(targetName); ok && number == chapter {
			backups = append(backups, entry.Name())
		}
	}
	if len(backups) > 1 {
		return fmt.Errorf("multiple interrupted backups resolve to chapter %d", chapter)
	}
	if len(backups) == 0 {
		return nil
	}
	backup := filepath.Join(root, backups[0])
	target := filepath.Join(root, strings.TrimSuffix(backups[0], ".quality-backup"))
	if _, err := os.Lstat(target); err == nil {
		return os.Remove(backup)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(backup, target)
}
