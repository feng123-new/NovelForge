package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
)

// LoadModelConfig keeps ID -> path translation in Repository. The result is
// server-private: never serialize it. No chdir or implicit credential migration.
func (r *Repository) LoadModelConfig(ctx context.Context, id, explicit string) (bootstrap.Config, error) {
	if err := ctx.Err(); err != nil { return bootstrap.Config{}, err }
	entry, err := r.find(id)
	if err != nil { return bootstrap.Config{}, err }
	for _, dir := range []string{".novelforge", ".ainovel"} {
		for _, name := range []string{dir, filepath.Join(dir, "config.json")} {
			info, err := os.Lstat(filepath.Join(entry.Root, name))
			if err != nil && !errors.Is(err, os.ErrNotExist) { return bootstrap.Config{}, errors.New("project model configuration is unreadable") }
			if err == nil && info.Mode()&os.ModeSymlink != 0 { return bootstrap.Config{}, errors.New("project model configuration cannot be a symlink") }
		}
	}
	result, err := bootstrap.LoadNovelForgeConfig("", entry.Root, explicit)
	if err != nil { return bootstrap.Config{}, errors.New("project model configuration could not be loaded") }
	return result.Config, nil
}
