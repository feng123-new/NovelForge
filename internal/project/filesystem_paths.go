package project

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func ensureChildPath(parent, candidate string) error {
	parentAbsolute, err := filepath.Abs(parent)
	if err != nil {
		return newError("PROJECT_PATH_UNSAFE", "project path is unsafe", ErrUnsafePath)
	}
	candidateAbsolute, err := filepath.Abs(candidate)
	if err != nil {
		return newError("PROJECT_PATH_UNSAFE", "project path is unsafe", ErrUnsafePath)
	}
	relative, err := filepath.Rel(parentAbsolute, candidateAbsolute)
	if err != nil ||
		relative == "." ||
		relative == "" ||
		relative == ".." ||
		filepath.IsAbs(relative) ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return newError("PROJECT_PATH_UNSAFE", "project path is outside its allowed boundary", ErrUnsafePath)
	}
	return nil
}

func (r *Repository) resolveImportPath(relativePath string) (string, error) {
	relativePath = strings.TrimSpace(relativePath)
	portablePath := strings.ReplaceAll(relativePath, "\\", "/")
	if relativePath == "" ||
		filepath.IsAbs(relativePath) ||
		filepath.VolumeName(relativePath) != "" ||
		path.IsAbs(portablePath) ||
		looksLikeWindowsAbsolute(relativePath) ||
		strings.Contains(relativePath, "\x00") {
		return "", newError(
			"PROJECT_IMPORT_PATH_INVALID",
			"import_path must be a workspace-relative directory",
			ErrUnsafePath,
		)
	}
	portablePath = path.Clean(portablePath)
	if portablePath == "." || portablePath == ".." || strings.HasPrefix(portablePath, "../") {
		return "", newError(
			"PROJECT_IMPORT_PATH_INVALID",
			"import_path must identify a child directory",
			ErrUnsafePath,
		)
	}
	clean := filepath.Clean(filepath.FromSlash(portablePath))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", newError(
			"PROJECT_IMPORT_PATH_INVALID",
			"import_path must identify a child directory",
			ErrUnsafePath,
		)
	}
	root := filepath.Join(r.workspace, clean)
	if err := ensureChildPath(r.workspace, root); err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", newError(
				"PROJECT_IMPORT_NOT_FOUND",
				"import project directory was not found",
				ErrNotFound,
			)
		}
		return "", newError(
			"PROJECT_STORAGE_ERROR",
			"import project directory could not be inspected",
			err,
		)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", newError(
			"PROJECT_IMPORT_PATH_INVALID",
			"import project directory is unsafe",
			ErrUnsafePath,
		)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", newError(
			"PROJECT_IMPORT_PATH_INVALID",
			"import project directory is unsafe",
			ErrUnsafePath,
		)
	}
	if err := ensureChildPath(r.resolvedWorkspace, resolved); err != nil {
		return "", err
	}
	return filepath.Clean(root), nil
}

func looksLikeWindowsAbsolute(value string) bool {
	if len(value) >= 3 {
		letter := value[0]
		if ((letter >= 'a' && letter <= 'z') || (letter >= 'A' && letter <= 'Z')) &&
			value[1] == ':' && (value[2] == '\\' || value[2] == '/') {
			return true
		}
	}
	return strings.HasPrefix(value, `\\`) || strings.HasPrefix(value, "//")
}

func (r *Repository) validateDestructiveRoot(root string) error {
	root = filepath.Clean(root)
	if root == r.workspace {
		return newError(
			"PROJECT_DELETE_WORKSPACE_REFUSED",
			"workspace root cannot be deleted",
			ErrWorkspaceRoot,
		)
	}
	if err := ensureChildPath(r.workspace, root); err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return newError("PROJECT_NOT_FOUND", "project not found", ErrNotFound)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return newError(
			"PROJECT_PATH_UNSAFE",
			"project path is unsafe",
			ErrUnsafePath,
		)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return newError(
			"PROJECT_PATH_UNSAFE",
			"project path is unsafe",
			ErrUnsafePath,
		)
	}
	if err := ensureChildPath(r.resolvedWorkspace, resolved); err != nil {
		return err
	}
	if isProtectedRoot(resolved) || isRepositoryRoot(resolved) {
		return newError(
			"PROJECT_DELETE_PROTECTED_PATH",
			"protected filesystem locations cannot be deleted",
			ErrUnsafePath,
		)
	}
	return nil
}

func isProtectedRoot(path string) bool {
	path = filepath.Clean(path)
	volume := filepath.VolumeName(path)
	root := string(filepath.Separator)
	if volume != "" {
		root = volume + string(filepath.Separator)
	}
	if samePath(path, root) {
		return true
	}
	if home, err := os.UserHomeDir(); err == nil && samePath(path, home) {
		return true
	}
	if working, err := os.Getwd(); err == nil && samePath(path, working) {
		return true
	}
	return false
}

func isRepositoryRoot(root string) bool {
	// A workspace may be configured above a source checkout. Never allow a
	// project lifecycle request to remove a directory that is itself a Git
	// repository, whether .git is a directory (normal checkout) or a file
	// (worktree/submodule).
	_, err := os.Lstat(filepath.Join(root, ".git"))
	return err == nil || !errors.Is(err, os.ErrNotExist)
}

func samePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if isCaseInsensitivePlatform() {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func isCaseInsensitivePlatform() bool {
	// Windows is always case-insensitive for supported NovelForge builds. macOS
	// is commonly case-insensitive, but containment still uses filepath.Rel.
	return filepath.Separator == '\\'
}
