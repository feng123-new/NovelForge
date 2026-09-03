package project

import "github.com/voocel/ainovel-cli/internal/contextcompiler"

func init() {
	projectMigrations = append(projectMigrations, contextcompiler.Migration())
}
