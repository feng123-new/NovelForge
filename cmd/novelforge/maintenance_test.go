package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/compat"
)

func TestDoctorReportsLegacyConfigWithoutLeakingSecrets(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	secret := "sk-doctor-must-not-leak"
	writeMaintenanceConfig(t, filepath.Join(home, compat.LegacyDirName, compat.ConfigFileName), secret)

	var stdout, stderr bytes.Buffer
	exitCode := runDoctorCommandWith([]string{
		"--home", home,
		"--project-root", project,
		"--json",
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %s", exitCode, stderr.String())
	}
	if strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) {
		t.Fatal("doctor output leaked an API key")
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.Status != "warning" {
		t.Fatalf("status = %q, want warning; report=%#v", report.Status, report)
	}
	if report.ConfigSource == nil || report.ConfigSource.Generation != compat.GenerationLegacy {
		t.Fatalf("config source = %#v, want legacy", report.ConfigSource)
	}
}

func TestDoctorFailsForInvalidPreferredConfig(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	path := filepath.Join(project, compat.ProductDirName, compat.ConfigFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"provider":}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	exitCode := runDoctorCommandWith([]string{
		"--home", home,
		"--project-root", project,
		"--json",
	}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1; stderr=%s", exitCode, stderr.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.Status != "error" {
		t.Fatalf("status = %q, want error", report.Status)
	}
}

func TestMigrateCommandDryRunIsReadOnly(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	secret := "sk-migrate-must-not-leak"
	writeMaintenanceConfig(t, filepath.Join(home, compat.LegacyDirName, compat.ConfigFileName), secret)

	var stdout, stderr bytes.Buffer
	exitCode := runMigrateCommandWith(context.Background(), []string{
		"--home", home,
		"--project-root", project,
		"--global-only",
		"--dry-run",
		"--json",
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stderr=%s", exitCode, stderr.String())
	}
	if strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) {
		t.Fatal("migration output leaked an API key")
	}
	paths, err := compat.ResolvePaths(home, project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.GlobalNovelForgeDir); !os.IsNotExist(err) {
		t.Fatalf("dry-run created destination: %v", err)
	}
}

func TestMigrateCommandCreatesDestinationAndRetainsLegacy(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeMaintenanceConfig(t, filepath.Join(home, compat.LegacyDirName, compat.ConfigFileName), "sk-safe")

	var stdout, stderr bytes.Buffer
	exitCode := runMigrateCommandWith(context.Background(), []string{
		"--home", home,
		"--project-root", project,
		"--global-only",
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stderr=%s", exitCode, stderr.String())
	}
	paths, err := compat.ResolvePaths(home, project)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{paths.GlobalLegacyConfig, paths.GlobalNovelForgeConfig} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
}

func TestMigrateCommandRejectsConflictingScopeFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := runMigrateCommandWith(context.Background(), []string{
		"--global-only",
		"--project-only",
	}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	if !strings.Contains(stderr.String(), "cannot be combined") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func writeMaintenanceConfig(t *testing.T, path, secret string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{
  "provider": "test",
  "model": "test-model",
  "providers": {
    "test": { "type": "openai", "api_key": "` + secret + `" }
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
