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
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
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
	for i, candidate := range candidates {
		if candidate.Scope != want[i].scope || candidate.Generation != want[i].generation || !candidate.Exists {
			t.Fatalf("candidate[%d] = %#v, want scope=%s generation=%s exists", i, candidate, want[i].scope, want[i].generation)
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
