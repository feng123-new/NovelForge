package project

import "context"

// ConfigurationRoot resolves a project for internal runtime composition only.
// Like Workspace, this absolute path must never be serialized by transport code.
// Provider construction stays outside the persistence package.
func (r *Repository) ConfigurationRoot(ctx context.Context, id string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	entry, err := r.find(id)
	if err != nil {
		return "", err
	}
	return entry.Root, nil
}
