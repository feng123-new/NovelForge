package compat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMigrateDryRunDoesNotWrite(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeLegacyConfig(t, filepath.Join(home, LegacyDirName), "global-secret")
	writeLegacyConfig(t, filepath.Join(project, LegacyDirName), "project-secret")

	report, err := Migrate(context.Background(), MigrationOptions{
		Home:        home,
		ProjectRoot: project,
		DryRun:      true,
		Now:         fixedMigrationTime,
	})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(report.Actions) != 2 {
		t.Fatalf("actions = %d, want 2", len(report.Actions))
	}
	for _, action := range report.Actions {
		if action.Status != "planned" {
			t.Fatalf("action = %#v, want planned", action)
		}
		if _, err := os.Stat(action.Destination); !os.IsNotExist(err) {
			t.Fatalf("dry-run created destination %s", action.Destination)
		}
		if _, err := os.Stat(action.Backup); !os.IsNotExist(err) {
			t.Fatalf("dry-run created backup %s", action.Backup)
		}
	}
}

func TestMigrateCopiesBothScopesWithBackupAndManifest(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeLegacyConfig(t, filepath.Join(home, LegacyDirName), "global-secret")
	writeLegacyConfig(t, filepath.Join(project, LegacyDirName), "project-secret")

	report, err := Migrate(context.Background(), MigrationOptions{
		Home:        home,
		ProjectRoot: project,
		Now:         fixedMigrationTime,
	})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(report.Actions) != 2 {
		t.Fatalf("actions = %d, want 2", len(report.Actions))
	}
	for _, action := range report.Actions {
		if action.Status != "migrated" {
			t.Fatalf("action = %#v, want migrated", action)
		}
		assertFileContains(t, filepath.Join(action.Source, ConfigFileName), "secret")
		assertFileContains(t, filepath.Join(action.Destination, ConfigFileName), "secret")
		assertFileContains(t, filepath.Join(action.Backup, ConfigFileName), "secret")

		manifestData, err := os.ReadFile(filepath.Join(action.Destination, MigrationManifestName))
		if err != nil {
			t.Fatalf("read destination manifest: %v", err)
		}
		if strings.Contains(string(manifestData), "global-secret") || strings.Contains(string(manifestData), "project-secret") {
			t.Fatal("manifest leaked configuration contents")
		}
		var manifest MigrationManifest
		if err := json.Unmarshal(manifestData, &manifest); err != nil {
			t.Fatalf("decode manifest: %v", err)
		}
		if manifest.Version != MigrationManifestVersion || len(manifest.Files) != 2 {
			t.Fatalf("unexpected manifest: %#v", manifest)
		}
		if manifest.Files[0].Path != ConfigFileName || manifest.Files[1].Path != "rules/voice.md" {
			t.Fatalf("manifest order = %#v", manifest.Files)
		}
	}
}

func TestMigrateIsIdempotentWhenDestinationExists(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeLegacyConfig(t, filepath.Join(home, LegacyDirName), "global-secret")

	first, err := Migrate(context.Background(), MigrationOptions{
		Home:           home,
		ProjectRoot:    project,
		IncludeGlobal:  true,
		IncludeProject: false,
		Now:            fixedMigrationTime,
	})
	if err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if first.Actions[0].Status != "migrated" {
		t.Fatalf("first action = %#v", first.Actions[0])
	}
	firstDestinationData, err := os.ReadFile(filepath.Join(first.Actions[0].Destination, ConfigFileName))
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}

	second, err := Migrate(context.Background(), MigrationOptions{
		Home:           home,
		ProjectRoot:    project,
		IncludeGlobal:  true,
		IncludeProject: false,
		Now:            fixedMigrationTime,
	})
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if second.Actions[0].Status != "destination_exists" {
		t.Fatalf("second action = %#v", second.Actions[0])
	}
	secondDestinationData, err := os.ReadFile(filepath.Join(second.Actions[0].Destination, ConfigFileName))
	if err != nil {
		t.Fatalf("read destination again: %v", err)
	}
	if string(firstDestinationData) != string(secondDestinationData) {
		t.Fatal("idempotent migration changed destination content")
	}
}

func TestMigrateRejectsSymbolicLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated privileges on Windows")
	}
	home := t.TempDir()
	project := t.TempDir()
	legacy := filepath.Join(home, LegacyDirName)
	writeLegacyConfig(t, legacy, "global-secret")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(legacy, "linked-secret")); err != nil {
		t.Fatal(err)
	}

	report, err := Migrate(context.Background(), MigrationOptions{
		Home:           home,
		ProjectRoot:    project,
		IncludeGlobal:  true,
		IncludeProject: false,
		Now:            fixedMigrationTime,
	})
	if err == nil {
		t.Fatal("expected symbolic link migration to fail")
	}
	if len(report.Actions) != 1 || report.Actions[0].Status != "failed" {
		t.Fatalf("report = %#v", report)
	}
	paths, resolveErr := ResolvePaths(home, project)
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	if _, statErr := os.Stat(paths.GlobalNovelForgeDir); !os.IsNotExist(statErr) {
		t.Fatalf("failed migration created destination: %v", statErr)
	}
}

func TestMigrateHonorsCancelledContext(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeLegacyConfig(t, filepath.Join(home, LegacyDirName), "global-secret")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	report, err := Migrate(ctx, MigrationOptions{
		Home:           home,
		ProjectRoot:    project,
		IncludeGlobal:  true,
		IncludeProject: false,
		Now:            fixedMigrationTime,
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if len(report.Actions) != 1 || report.Actions[0].Status != "failed" {
		t.Fatalf("report = %#v", report)
	}
}

func fixedMigrationTime() time.Time {
	return time.Date(2026, time.August, 31, 12, 0, 0, 0, time.UTC)
}

func writeLegacyConfig(t *testing.T, dir, secret string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "rules"), 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	config := `{"provider":"test","providers":{"test":{"api_key":"` + secret + `"}}}`
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rules", "voice.md"), []byte("restrained voice\n"), 0o644); err != nil {
		t.Fatalf("write rules: %v", err)
	}
}

func assertFileContains(t *testing.T, path, text string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), text) {
		t.Fatalf("%s does not contain %q", path, text)
	}
}
