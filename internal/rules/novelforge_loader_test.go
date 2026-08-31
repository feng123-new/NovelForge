package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/compat"
)

func TestNovelForgeRulesPreferNewDirectoryPerScope(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(compat.RuntimeProfileEnv, compat.RuntimeProfileNovelForge)
	t.Chdir(project)

	globalNew := filepath.Join(home, compat.ProductDirName, "rules")
	globalOld := filepath.Join(home, compat.LegacyDirName, "rules")
	projectNew := filepath.Join(project, compat.ProductDirName, "rules")
	projectOld := filepath.Join(project, compat.LegacyDirName, "rules")
	for _, dir := range []string{globalNew, globalOld, projectNew, projectOld} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if got := DefaultHomeRulesDir(); got != globalNew {
		t.Fatalf("home rules=%q, want %q", got, globalNew)
	}
	if got := DefaultProjectRulesDir(project); got != projectNew {
		t.Fatalf("project rules=%q, want %q", got, projectNew)
	}
	options := DefaultOptions()
	if options.HomeRulesDir != globalNew || options.ProjectRulesDir != projectNew {
		t.Fatalf("options=%#v", options)
	}
}

func TestNovelForgeRulesFallBackWithoutCreatingNewDirectory(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(compat.RuntimeProfileEnv, compat.RuntimeProfileNovelForge)
	t.Chdir(project)

	globalOld := filepath.Join(home, compat.LegacyDirName, "rules")
	projectOld := filepath.Join(project, compat.LegacyDirName, "rules")
	for _, dir := range []string{globalOld, projectOld} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if got := DefaultHomeRulesDir(); got != globalOld {
		t.Fatalf("home fallback=%q, want %q", got, globalOld)
	}
	if got := DefaultProjectRulesDir(project); got != projectOld {
		t.Fatalf("project fallback=%q, want %q", got, projectOld)
	}
	if _, err := os.Stat(filepath.Join(home, compat.ProductDirName)); !os.IsNotExist(err) {
		t.Fatalf("resolution created new global directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, compat.ProductDirName)); !os.IsNotExist(err) {
		t.Fatalf("resolution created new project directory: %v", err)
	}
}

func TestNovelForgeFreshRulesUseNewDirectoryAndBrand(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(compat.RuntimeProfileEnv, compat.RuntimeProfileNovelForge)
	t.Chdir(project)

	want := filepath.Join(home, compat.ProductDirName, "rules")
	if got := DefaultHomeRulesDir(); got != want {
		t.Fatalf("fresh home rules=%q, want %q", got, want)
	}
	EnsureHomeRulesDir()
	data, err := os.ReadFile(filepath.Join(want, "README.txt"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "NovelForge") || !strings.Contains(text, ".novelforge/rules") {
		t.Fatalf("README is not NovelForge-branded: %q", text)
	}
}

func TestLegacyRulesRemainOnAinovelDirectory(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(compat.RuntimeProfileEnv, "")
	if got := DefaultHomeRulesDir(); got != filepath.Join(home, compat.LegacyDirName, "rules") {
		t.Fatalf("legacy home rules changed: %q", got)
	}
	if got := DefaultProjectRulesDir(project); got != filepath.Join(project, compat.LegacyDirName, "rules") {
		t.Fatalf("legacy project rules changed: %q", got)
	}
}
