package project

import "github.com/voocel/ainovel-cli/internal/narrativeledger"

func init() {
	projectMigrations = append(projectMigrations, narrativeledger.Migration())
}
