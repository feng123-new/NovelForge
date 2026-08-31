package project

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func copyProjectTree(source, destination string) error {
	sourceInfo, err := os.Lstat(source)
	if err != nil {
		return newError("PROJECT_DUPLICATE_FAILED", "source project cannot be read", err)
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.IsDir() {
		return newError("PROJECT_PATH_UNSAFE", "source project path is unsafe", ErrUnsafePath)
	}
	if err := os.Mkdir(destination, sourceInfo.Mode().Perm()); err != nil {
		return newError(
			"PROJECT_DUPLICATE_FAILED",
			"duplicate project directory could not be created",
			err,
		)
	}
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		relativeSlash := filepath.ToSlash(relative)
		if shouldSkipDuplicatePath(relativeSlash, entry.IsDir()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return newError(
				"PROJECT_PATH_UNSAFE",
				"projects containing symbolic links cannot be duplicated",
				ErrUnsafePath,
			)
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyRegularFile(path, target, info.Mode().Perm())
	})
	if err != nil {
		return newError(
			"PROJECT_DUPLICATE_FAILED",
			"project files could not be duplicated",
			err,
		)
	}
	return nil
}

func shouldSkipDuplicatePath(relative string, directory bool) bool {
	clean := strings.ToLower(strings.TrimPrefix(filepath.ToSlash(relative), "./"))
	if clean == ".novelforge/backups" ||
		clean == ".novelforge/trash" ||
		clean == ".novelforge/deleted.json" {
		return true
	}
	base := strings.ToLower(filepath.Base(clean))
	if !directory {
		if strings.HasSuffix(base, "-wal") || strings.HasSuffix(base, "-shm") {
			return true
		}
		if base == ".env" || strings.HasPrefix(base, ".env.") ||
			strings.Contains(base, "credential") ||
			strings.Contains(base, "password") ||
			strings.Contains(base, "private_key") ||
			strings.Contains(base, "secret") ||
			strings.Contains(base, "token") {
			return true
		}
		if isRuntimeConfigPath(clean) {
			return true
		}
	}
	return false
}

func isRuntimeConfigPath(relative string) bool {
	relative = strings.ToLower(filepath.ToSlash(relative))
	for _, prefix := range []string{".novelforge/config.", ".ainovel/config."} {
		if strings.HasPrefix(relative, prefix) {
			return true
		}
	}
	return false
}

func copyRegularFile(source, destination string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		_ = output.Close()
		if !complete {
			_ = os.Remove(destination)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}
