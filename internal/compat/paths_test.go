package compat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigCandidatesUseNovelForgeBeforeLegacy(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	paths, err := ResolvePaths(home, project)
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	for _, path := range []string{
		paths.GlobalNovelForgeConfig,
		paths.GlobalLegacyConfig,
		paths.ProjectNovelForgeConfig,
		paths.ProjectLegacyConfig,
	} {
		writePathConfig(t, path)
	}

	candidates := paths.ConfigCandidates()
	want := []struct {
		scope      Scope
		generation Generation
	}{
		{ScopeProject, GenerationNovelForge},
		{ScopeProject, GenerationLegacy},
		{ScopeGlobal, GenerationNovelForge},
		{ScopeGlobal, GenerationLegacy},
	}
	if len(candidates) != len(want) {
		t.Fatalf("candidates = %d, want %d", len(candidates), len(want))
	}
	for i, current := range candidates {
		if current.Scope != want[i].scope || current.Generation != want[i].generation || !current.Exists {
			t.Fatalf("candidate[%d] = %#v, want scope=%s generation=%s exists", i, current, want[i].scope, want[i].generation)
		}
	}

	effective, ok := paths.EffectiveConfig()
	if !ok {
		t.Fatal("expected an effective config")
	}
	if effective.Path != paths.ProjectNovelForgeConfig {
		t.Fatalf("effective path = %q, want %q", effective.Path, paths.ProjectNovelForgeConfig)
	}
}

func TestResolveConfigSelectsOneLayerPerScope(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	paths, err := ResolvePaths(home, project)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		paths.GlobalNovelForgeConfig,
		paths.GlobalLegacyConfig,
		paths.ProjectNovelForgeConfig,
		paths.ProjectLegacyConfig,
	} {
		writePathConfig(t, path)
	}

	resolution, err := paths.ResolveConfig("")
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	if resolution.Global == nil || resolution.Global.Path != paths.GlobalNovelForgeConfig {
		t.Fatalf("global = %#v, want NovelForge global", resolution.Global)
	}
	if resolution.Project == nil || resolution.Project.Path != paths.ProjectNovelForgeConfig {
		t.Fatalf("project = %#v, want NovelForge project", resolution.Project)
	}
	if resolution.Effective == nil || resolution.Effective.Path != paths.ProjectNovelForgeConfig {
		t.Fatalf("effective = %#v, want NovelForge project", resolution.Effective)
	}
}

func TestResolveConfigFallsBackToLegacyWithoutCopying(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	paths, err := ResolvePaths(home, project)
	if err != nil {
		t.Fatal(err)
	}
	writePathConfig(t, paths.GlobalLegacyConfig)
	writePathConfig(t, paths.ProjectLegacyConfig)

	resolution, err := paths.ResolveConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Global == nil || resolution.Global.Generation != GenerationLegacy {
		t.Fatalf("global = %#v, want legacy", resolution.Global)
	}
	if resolution.Project == nil || resolution.Project.Generation != GenerationLegacy {
		t.Fatalf("project = %#v, want legacy", resolution.Project)
	}
	if _, err := os.Stat(paths.GlobalNovelForgeDir); !os.IsNotExist(err) {
		t.Fatalf("resolution must not create %s: %v", paths.GlobalNovelForgeDir, err)
	}
	if _, err := os.Stat(paths.ProjectNovelForgeDir); !os.IsNotExist(err) {
		t.Fatalf("resolution must not create %s: %v", paths.ProjectNovelForgeDir, err)
	}
}

func TestResolveConfigExplicitWinsAndUsesProjectRelativePath(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	paths, err := ResolvePaths(home, project)
	if err != nil {
		t.Fatal(err)
	}
	writePathConfig(t, paths.GlobalNovelForgeConfig)
	writePathConfig(t, paths.ProjectNovelForgeConfig)
	explicit := filepath.Join("configs", "selected.json")
	writePathConfig(t, filepath.Join(project, explicit))

	resolution, err := paths.ResolveConfig(explicit)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(project, explicit)
	if resolution.Explicit == nil || resolution.Explicit.Path != want || !resolution.Explicit.Exists {
		t.Fatalf("explicit = %#v, want %s", resolution.Explicit, want)
	}
	if resolution.Effective == nil || resolution.Effective.Path != want {
		t.Fatalf("effective = %#v, want explicit", resolution.Effective)
	}
	if resolution.Project == nil || resolution.Global == nil {
		t.Fatalf("diagnostic layers should remain visible: %#v", resolution)
	}
}

func TestRuntimeProfileIsOptIn(t *testing.T) {
	t.Setenv(RuntimeProfileEnv, "")
	t.Setenv(ExplicitConfigEnv, "relative.json")
	if NovelForgeRuntimeActive() {
		t.Fatal("runtime must be legacy until cmd/novelforge activates it")
	}
	if got := ExplicitConfigPath(); got != "" {
		t.Fatalf("legacy runtime must ignore NOVELFORGE_CONFIG, got %q", got)
	}
	if err := ActivateNovelForgeRuntime(); err != nil {
		t.Fatal(err)
	}
	if !NovelForgeRuntimeActive() {
		t.Fatal("NovelForge runtime was not activated")
	}
	if got := ExplicitConfigPath(); got != "relative.json" {
		t.Fatalf("explicit path = %q", got)
	}
}

func TestResolvePathsUsesAbsoluteCleanRoots(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	paths, err := ResolvePaths(filepath.Join(home, "."), filepath.Join(project, "."))
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	if !filepath.IsAbs(paths.Home) || !filepath.IsAbs(paths.ProjectRoot) {
		t.Fatalf("paths must be absolute: %#v", paths)
	}
	if filepath.Base(paths.GlobalNovelForgeDir) != ProductDirName {
		t.Fatalf("global directory = %q", paths.GlobalNovelForgeDir)
	}
	if filepath.Base(paths.ProjectLegacyDir) != LegacyDirName {
		t.Fatalf("legacy project directory = %q", paths.ProjectLegacyDir)
	}
}

func writePathConfig(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
