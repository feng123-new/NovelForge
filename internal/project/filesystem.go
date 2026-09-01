package project

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func initializeLayout(root string) error {
	directories := []struct {
		path string
		mode fs.FileMode
	}{
		{path: filepath.Join(root, ".novelforge"), mode: 0o700},
		{path: filepath.Join(root, ".novelforge", "rules"), mode: 0o700},
		{path: filepath.Join(root, ".novelforge", "skills"), mode: 0o700},
		{path: filepath.Join(root, ".novelforge", "style"), mode: 0o700},
		{path: filepath.Join(root, ".novelforge", "output"), mode: 0o700},
		{path: filepath.Join(root, ".novelforge", "backups"), mode: 0o700},
		{path: filepath.Join(root, ".novelforge", "trash"), mode: 0o700},
		{path: filepath.Join(root, "chapters"), mode: 0o755},
		{path: filepath.Join(root, "references"), mode: 0o755},
	}
	for _, directory := range directories {
		info, err := os.Lstat(directory.path)
		switch {
		case err == nil:
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("required project directory is unsafe")
			}
		case errors.Is(err, os.ErrNotExist):
			if err := os.MkdirAll(directory.path, directory.mode); err != nil {
				return err
			}
		default:
			return err
		}
	}
	return nil
}

func writeJSONAtomic(path string, value any, mode fs.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	committed := false
	defer func() {
		_ = temp.Close()
		if !committed {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(mode); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := replaceFile(tempName, path); err != nil {
		return err
	}
	committed = true
	return nil
}

func replaceFile(source, destination string) error {
	if err := os.Rename(source, destination); err == nil {
		return nil
	}
	if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(source, destination)
}

func randomHex(reader io.Reader, bytes int) (string, error) {
	if bytes <= 0 {
		return "", errors.New("random byte count must be positive")
	}
	buffer := make([]byte, bytes)
	if _, err := io.ReadFull(reader, buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func normalizeSlug(requested, title, id string) string {
	source := strings.TrimSpace(requested)
	if source == "" {
		source = strings.TrimSpace(title)
	}
	var builder strings.Builder
	lastDash := false
	for _, runeValue := range strings.ToLower(source) {
		switch {
		case runeValue >= 'a' && runeValue <= 'z':
			builder.WriteRune(runeValue)
			lastDash = false
		case runeValue >= '0' && runeValue <= '9':
			builder.WriteRune(runeValue)
			lastDash = false
		case runeValue == '-' || runeValue == '_' || unicode.IsSpace(runeValue):
			if builder.Len() > 0 && !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(builder.String(), "-.")
	if slug == "" {
		slug = "project-" + id[:8]
	}
	if len(slug) > 80 {
		slug = strings.Trim(slug[:80], "-.")
	}
	if slug == "" || slug == "." || slug == ".." || slug == ".novelforge" {
		slug = "project-" + id[:8]
	}
	return slug
}
