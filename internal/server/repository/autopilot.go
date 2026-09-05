package repository

import "github.com/voocel/ainovel-cli/internal/autopilot"

// Workspace migration 3 is additive; project migrations and their checksums
// remain untouched. Foundation output stays in the project directory.
func init(){ControlMigrations=append(ControlMigrations,autopilot.Migration())}
