package project

import "github.com/voocel/ainovel-cli/internal/truthstore"

func init() {
	projectMigrations = append(projectMigrations, truthstore.Migration())
}
