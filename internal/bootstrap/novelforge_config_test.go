package bootstrap

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/voocel/ainovel-cli/internal/compat"
)

func TestNovelForgeConfigPrecedenceSelectsNewPathPerScope(t *testing.T) {
	home, project := activateNovelForgeConfigTest(t)
	paths, err := compat.ResolvePaths(home, project)
	if err != nil {
		t.Fatal(err)
	}
	writeConfigFixture(t, paths.GlobalLegacyConfig, configFixture("legacy-global", "legacy-global-model", "legacy-global-token"))
	writeConfigFixture(t, paths.GlobalNovelForgeConfig, configFixture("novelforge", "global-new-model", "new-global-token"))
	writeConfigFixture(t, paths.ProjectLegacyConfig, `{
  "model": "project-legacy-model",
  "providers": {"legacy-only": {"type":"openai", "api_key":"legacy-project-token"}}
}`)
	writeConfigFixture(t, paths.ProjectNovelForgeConfig, `{"model":"project-new-model"}`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Provider != "novelforge" || cfg.ModelName != "project-new-model" {
		t.Fatalf("unexpected selected config: provider=%q model=%q", cfg.Provider, cfg.ModelName)
	}
	if _, ok := cfg.Providers["legacy-global"]; ok {
		t.Fatal("global .ainovel must be shadowed by global .novelforge")
	}
	if _, ok := cfg.Providers["legacy-only"]; ok {
		t.Fatal("project .ainovel must be shadowed by project .novelforge")
	}
	if got := EffectiveConfigPath(); got != paths.ProjectNovelForgeConfig {
		t.Fatalf("EffectiveConfigPath=%q, want %q", got, paths.ProjectNovelForgeConfig)
	}
}

func TestNovelForgeConfigFallsBackToLegacyLayers(t *testing.T) {
	home, project := activateNovelForgeConfigTest(t)
	paths, err := compat.ResolvePaths(home, project)
	if err != nil {
		t.Fatal(err)
	}
	writeConfigFixture(t, paths.GlobalLegacyConfig, configFixture("legacy", "global-model", "legacy-token"))
	writeConfigFixture(t, paths.ProjectLegacyConfig, `{"model":"project-model"}`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Provider != "legacy" || cfg.ModelName != "project-model" {
		t.Fatalf("legacy fallback failed: %#v", cfg)
	}
	if got := EffectiveConfigPath(); got != paths.ProjectLegacyConfig {
		t.Fatalf("EffectiveConfigPath=%q, want legacy project %q", got, paths.ProjectLegacyConfig)
	}
	if _, err := os.Stat(paths.GlobalNovelForgeDir); !os.IsNotExist(err) {
		t.Fatalf("fallback must not create global NovelForge directory: %v", err)
	}
}

func TestNovelForgePreferredPathShadowsCorruptLegacyPeer(t *testing.T) {
	home, project := activateNovelForgeConfigTest(t)
	paths, err := compat.ResolvePaths(home, project)
	if err != nil {
		t.Fatal(err)
	}
	writeConfigFixture(t, paths.GlobalNovelForgeConfig, configFixture("new", "new-model", "new-token"))
	writeConfigFixture(t, paths.GlobalLegacyConfig, `{not-json`)
	writeConfigFixture(t, paths.ProjectNovelForgeConfig, `{"model":"project-new"}`)
	writeConfigFixture(t, paths.ProjectLegacyConfig, `{not-json`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("shadowed legacy files must not be parsed: %v", err)
	}
	if cfg.Provider != "new" || cfg.ModelName != "project-new" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestNovelForgeCorruptPreferredProjectDoesNotFallBack(t *testing.T) {
	home, project := activateNovelForgeConfigTest(t)
	paths, err := compat.ResolvePaths(home, project)
	if err != nil {
		t.Fatal(err)
	}
	writeConfigFixture(t, paths.GlobalNovelForgeConfig, configFixture("new", "new-model", "new-token"))
	writeConfigFixture(t, paths.ProjectLegacyConfig, `{"model":"legacy-project"}`)
	writeConfigFixture(t, paths.ProjectNovelForgeConfig, `{"model":}`)

	if _, err := LoadConfig(); err == nil {
		t.Fatal("corrupt project .novelforge must fail instead of falling back to .ainovel")
	}
}

func TestNovelForgeExplicitConfigIsAuthoritative(t *testing.T) {
	home, project := activateNovelForgeConfigTest(t)
	paths, err := compat.ResolvePaths(home, project)
	if err != nil {
		t.Fatal(err)
	}
	writeConfigFixture(t, paths.GlobalNovelForgeConfig, configFixture("global", "global-model", "global-token"))
	writeConfigFixture(t, paths.ProjectNovelForgeConfig, `{"model":"project-model"}`)
	explicit := filepath.Join(project, "config", "explicit.json")
	writeConfigFixture(t, explicit, configFixture("explicit", "explicit-model", "explicit-token"))
	t.Setenv(compat.ExplicitConfigEnv, filepath.Join("config", "explicit.json"))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Provider != "explicit" || cfg.ModelName != "explicit-model" {
		t.Fatalf("explicit config not authoritative: %#v", cfg)
	}
	if _, ok := cfg.Providers["global"]; ok {
		t.Fatal("explicit configuration must not merge global credentials")
	}
	if got := EffectiveConfigPath(); got != explicit {
		t.Fatalf("EffectiveConfigPath=%q, want explicit %q", got, explicit)
	}
}

func TestNovelForgeFreshSetupTargetsNewDirectory(t *testing.T) {
	home, project := activateNovelForgeConfigTest(t)
	paths, err := compat.ResolvePaths(home, project)
	if err != nil {
		t.Fatal(err)
	}
	if got := DefaultConfigPath(); got != paths.GlobalNovelForgeConfig {
		t.Fatalf("DefaultConfigPath=%q, want %q", got, paths.GlobalNovelForgeConfig)
	}
	if got := projectConfigPath(); got != paths.ProjectNovelForgeConfig {
		t.Fatalf("projectConfigPath=%q, want %q", got, paths.ProjectNovelForgeConfig)
	}
	if !NeedsSetup() {
		t.Fatal("fresh NovelForge profile should require setup")
	}
}

func TestNovelForgeStartupErrorUsesSelectedDirectory(t *testing.T) {
	home, project := activateNovelForgeConfigTest(t)
	paths, err := compat.ResolvePaths(home, project)
	if err != nil {
		t.Fatal(err)
	}
	writeConfigFixture(t, paths.GlobalNovelForgeConfig, configFixture("new", "new-model", "new-token"))

	path := WriteStartupError("novelforge startup failure")
	if path != filepath.Join(paths.GlobalNovelForgeDir, "last-error.log") {
		t.Fatalf("startup log=%q, want NovelForge directory", path)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("startup log permissions are too broad: %o", info.Mode().Perm())
	}
}

func TestLegacyRuntimeIgnoresNovelForgePaths(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(compat.RuntimeProfileEnv, "")
	t.Setenv(compat.ExplicitConfigEnv, filepath.Join(project, "explicit.json"))
	t.Chdir(project)
	paths, err := compat.ResolvePaths(home, project)
	if err != nil {
		t.Fatal(err)
	}
	writeConfigFixture(t, paths.GlobalLegacyConfig, configFixture("legacy", "legacy-model", "legacy-token"))
	writeConfigFixture(t, paths.GlobalNovelForgeConfig, configFixture("new", "new-model", "new-token"))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "legacy" || DefaultConfigPath() != paths.GlobalLegacyConfig {
		t.Fatalf("legacy runtime changed behavior: cfg=%#v path=%q", cfg, DefaultConfigPath())
	}
}

func activateNovelForgeConfigTest(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(compat.RuntimeProfileEnv, compat.RuntimeProfileNovelForge)
	t.Setenv(compat.ExplicitConfigEnv, "")
	t.Chdir(project)
	return home, project
}

func configFixture(provider, model, key string) string {
	return `{
  "provider": "` + provider + `",
  "model": "` + model + `",
  "providers": {
    "` + provider + `": {"type":"openai", "api_key":"` + key + `"}
  }
}`
}

func writeConfigFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
