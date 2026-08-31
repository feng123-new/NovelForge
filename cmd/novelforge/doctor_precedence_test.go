package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/compat"
)

func TestDoctorPrefersNovelForgeAndTreatsLegacyAsShadowed(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	newSecret := "test-new-doctor-token"
	oldSecret := "test-old-doctor-token"
	writeMaintenanceConfig(t, filepath.Join(home, compat.ProductDirName, compat.ConfigFileName), newSecret)
	writeMaintenanceConfig(t, filepath.Join(home, compat.LegacyDirName, compat.ConfigFileName), oldSecret)

	var stdout, stderr bytes.Buffer
	exitCode := runDoctorCommandWith([]string{
		"--home", home,
		"--project-root", project,
		"--json",
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit=%d stderr=%s", exitCode, stderr.String())
	}
	if strings.Contains(stdout.String(), newSecret) || strings.Contains(stdout.String(), oldSecret) ||
		strings.Contains(stderr.String(), newSecret) || strings.Contains(stderr.String(), oldSecret) {
		t.Fatal("doctor leaked credentials")
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "ok" {
		t.Fatalf("status=%q report=%#v", report.Status, report)
	}
	if report.ConfigSource == nil || report.ConfigSource.Generation != compat.GenerationNovelForge {
		t.Fatalf("source=%#v, want NovelForge", report.ConfigSource)
	}
	if !hasDoctorCheck(report, "config.global.ainovel", "info") {
		t.Fatalf("shadowed legacy layer was not reported as info: %#v", report.Checks)
	}
}

func TestDoctorDoesNotParseCorruptShadowedLegacyConfig(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeMaintenanceConfig(t, filepath.Join(project, compat.ProductDirName, compat.ConfigFileName), "test-active-token")
	legacy := filepath.Join(project, compat.LegacyDirName, compat.ConfigFileName)
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(`{"api_key":"shadowed-test-token",`), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	exitCode := runDoctorCommandWith([]string{
		"--home", home,
		"--project-root", project,
		"--json",
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", exitCode, stderr.String(), stdout.String())
	}
	if strings.Contains(stdout.String(), "shadowed-test-token") || strings.Contains(stderr.String(), "shadowed-test-token") {
		t.Fatal("doctor leaked shadowed config contents")
	}
}

func TestDoctorExplicitConfigIsAuthoritative(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeMaintenanceConfig(t, filepath.Join(project, compat.ProductDirName, compat.ConfigFileName), "test-project-token")
	explicit := filepath.Join(project, "configs", "selected.json")
	writeMaintenanceConfig(t, explicit, "test-explicit-token")

	var stdout, stderr bytes.Buffer
	exitCode := runDoctorCommandWith([]string{
		"--home", home,
		"--project-root", project,
		"--config", filepath.Join("configs", "selected.json"),
		"--json",
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit=%d stderr=%s", exitCode, stderr.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.ConfigSource == nil || report.ConfigSource.Scope != compat.ScopeExplicit || report.ConfigSource.Path != explicit {
		t.Fatalf("source=%#v, want explicit %s", report.ConfigSource, explicit)
	}
	if len(report.ConfigLayers) != 1 || report.ConfigLayers[0].Scope != compat.ScopeExplicit {
		t.Fatalf("layers=%#v", report.ConfigLayers)
	}
}

func TestDoctorMissingExplicitConfigFailsWithoutFallback(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeMaintenanceConfig(t, filepath.Join(project, compat.ProductDirName, compat.ConfigFileName), "test-project-token")

	var stdout, stderr bytes.Buffer
	exitCode := runDoctorCommandWith([]string{
		"--home", home,
		"--project-root", project,
		"--config", "missing.json",
		"--json",
	}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("exit=%d, want 1; stderr=%s stdout=%s", exitCode, stderr.String(), stdout.String())
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "error" || !hasDoctorCheck(report, "config.explicit.explicit", "error") {
		t.Fatalf("report=%#v", report)
	}
}

func hasDoctorCheck(report doctorReport, name, status string) bool {
	for _, check := range report.Checks {
		if check.Name == name && check.Status == status {
			return true
		}
	}
	return false
}
