package compat

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMigrateFailureAfterBackupRollsBackStagingAndRetainsSource(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	legacy := filepath.Join(home, LegacyDirName)
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, ConfigFileName), []byte(`{"provider":"safe"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	// Calls: preflight, backup root, backup file, then staging root. Cancel on
	// the staging root so the verified backup exists before the injected failure.
	ctx := &cancelAfterErrCalls{cancelAt: 4}
	report, err := Migrate(ctx, MigrationOptions{
		Home:           home,
		ProjectRoot:    project,
		IncludeGlobal:  true,
		IncludeProject: false,
		Now: func() time.Time {
			return time.Date(2026, time.August, 31, 13, 0, 0, 0, time.UTC)
		},
	})
	if err == nil {
		t.Fatal("expected injected staging failure")
	}
	if len(report.Actions) != 1 || report.Actions[0].Status != "failed" {
		t.Fatalf("report=%#v", report)
	}
	action := report.Actions[0]
	if _, err := os.Stat(filepath.Join(action.Source, ConfigFileName)); err != nil {
		t.Fatalf("source was not retained: %v", err)
	}
	if _, err := os.Stat(filepath.Join(action.Backup, ConfigFileName)); err != nil {
		t.Fatalf("pre-migration backup was not retained: %v", err)
	}
	if _, err := os.Stat(filepath.Join(action.Backup, MigrationManifestName)); err != nil {
		t.Fatalf("backup manifest missing: %v", err)
	}
	if _, err := os.Stat(action.Destination); !os.IsNotExist(err) {
		t.Fatalf("failed migration left a destination: %v", err)
	}
	stages, err := filepath.Glob(filepath.Join(home, ".novelforge-migrate-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) != 0 {
		t.Fatalf("failed migration left staging directories: %#v", stages)
	}
}

type cancelAfterErrCalls struct {
	calls    int
	cancelAt int
}

func (c *cancelAfterErrCalls) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterErrCalls) Done() <-chan struct{}       { return nil }
func (c *cancelAfterErrCalls) Value(any) any               { return nil }
func (c *cancelAfterErrCalls) Err() error {
	c.calls++
	if c.calls >= c.cancelAt {
		return context.Canceled
	}
	return nil
}
