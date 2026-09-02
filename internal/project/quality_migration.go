package project

import "github.com/voocel/ainovel-cli/internal/qualitygate"

func init() {
	projectMigrations = append(projectMigrations, qualitygate.Migration())
}
