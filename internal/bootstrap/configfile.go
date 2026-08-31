package bootstrap

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/voocel/ainovel-cli/internal/compat"
)

// ConfigLoadWarning records a non-fatal lower-layer read failure. Global
// configuration remains tolerant so a valid project configuration can still
// start the application, matching the legacy behavior.
type ConfigLoadWarning struct {
	Scope compat.Scope
	Path  string
	Err   error
}

// NovelForgeConfigResult exposes the selected layers for doctor and tests
// without returning configuration contents through diagnostics.
type NovelForgeConfigResult struct {
	Config     Config
	Resolution compat.ConfigResolution
	Warnings   []ConfigLoadWarning
}

// DefaultConfigPath returns the effective writable top-level configuration
// path. cmd/ainovel-cli keeps ~/.ainovel/config.json. cmd/novelforge selects an
// explicit config first, then an existing ~/.novelforge file, then the legacy
// fallback; a fresh NovelForge setup writes ~/.novelforge/config.json.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if !compat.NovelForgeRuntimeActive() {
		return filepath.Join(home, compat.LegacyDirName, compat.ConfigFileName)
	}
	paths, err := compat.ResolvePaths(home, "")
	if err != nil {
		return ""
	}
	if explicit := compat.ExplicitConfigPath(); explicit != "" {
		resolution, resolveErr := paths.ResolveConfig(explicit)
		if resolveErr == nil && resolution.Explicit != nil {
			return resolution.Explicit.Path
		}
	}
	if selected, ok := paths.SelectedConfig(compat.ScopeGlobal); ok {
		return selected.Path
	}
	return paths.GlobalNovelForgeConfig
}

// DefaultConfigDir returns the directory containing DefaultConfigPath.
func DefaultConfigDir() string {
	path := DefaultConfigPath()
	if path == "" {
		return ""
	}
	return filepath.Dir(path)
}

// configDir returns the active global or explicit configuration directory,
// creating it when needed by first-run setup.
func configDir() (string, error) {
	dir := DefaultConfigDir()
	if dir == "" {
		return "", fmt.Errorf("home dir is unavailable")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	return dir, nil
}

// projectConfigPath returns the selected project configuration layer. A fresh
// NovelForge project uses ./.novelforge/config.json; a legacy-only project is
// read in place and is never copied or moved implicitly.
func projectConfigPath() string {
	if !compat.NovelForgeRuntimeActive() {
		return filepath.Join(compat.LegacyDirName, compat.ConfigFileName)
	}
	paths, err := compat.ResolvePaths("", "")
	if err != nil {
		return filepath.Join(compat.ProductDirName, compat.ConfigFileName)
	}
	if selected, ok := paths.SelectedConfig(compat.ScopeProject); ok {
		return selected.Path
	}
	return paths.ProjectNovelForgeConfig
}

// EffectiveConfigPath returns the file edited by TUI /config and /model.
// NovelForge writes the active explicit/project/global layer instead of
// silently creating a second credentials file.
func EffectiveConfigPath() string {
	if !compat.NovelForgeRuntimeActive() {
		rel := projectConfigPath()
		if _, err := os.Stat(rel); err == nil {
			if abs, err := filepath.Abs(rel); err == nil {
				return abs
			}
			return rel
		}
		return DefaultConfigPath()
	}

	paths, err := compat.ResolvePaths("", "")
	if err != nil {
		return DefaultConfigPath()
	}
	resolution, err := paths.ResolveConfig(compat.ExplicitConfigPath())
	if err == nil {
		if resolution.Explicit != nil {
			return resolution.Explicit.Path
		}
		if resolution.Project != nil {
			return resolution.Project.Path
		}
		if resolution.Global != nil {
			return resolution.Global.Path
		}
	}
	return paths.GlobalNovelForgeConfig
}

// LoadConfig loads the configuration for the current command profile.
// cmd/ainovel-cli keeps the original two-layer .ainovel merge. NovelForge uses
// one selected file per scope and never merges new and legacy credential files
// from the same scope.
func LoadConfig() (Config, error) {
	if !compat.NovelForgeRuntimeActive() {
		return loadLegacyConfig()
	}
	result, err := LoadNovelForgeConfig("", "", compat.ExplicitConfigPath())
	for _, warning := range result.Warnings {
		slog.Warn("配置解析失败，已忽略较低优先级层", "module", "config", "scope", warning.Scope, "path", warning.Path, "err", warning.Err)
	}
	return result.Config, err
}

// LoadNovelForgeConfig loads NovelForge configuration for an explicit home and
// project root. It is used by doctor and by unit tests so diagnostics do not
// need to mutate HOME or inspect secret values.
func LoadNovelForgeConfig(home, projectRoot, explicit string) (NovelForgeConfigResult, error) {
	paths, err := compat.ResolvePaths(home, projectRoot)
	if err != nil {
		return NovelForgeConfigResult{}, err
	}
	resolution, err := paths.ResolveConfig(explicit)
	if err != nil {
		return NovelForgeConfigResult{}, err
	}
	result := NovelForgeConfigResult{Resolution: resolution}

	if resolution.Explicit != nil {
		if !resolution.Explicit.Exists {
			return result, fmt.Errorf("显式配置不存在: %s", resolution.Explicit.Path)
		}
		cfg, err := loadJSONFile(resolution.Explicit.Path)
		if err != nil {
			return result, fmt.Errorf("显式配置 %s 解析失败: %w", resolution.Explicit.Path, err)
		}
		result.Config = cfg
		return result, nil
	}

	var cfg Config
	if resolution.Global != nil {
		global, err := loadJSONFile(resolution.Global.Path)
		if err != nil {
			result.Warnings = append(result.Warnings, ConfigLoadWarning{
				Scope: compat.ScopeGlobal,
				Path:  resolution.Global.Path,
				Err:   err,
			})
		} else {
			cfg = global
		}
	}

	if resolution.Project != nil {
		project, err := loadJSONFile(resolution.Project.Path)
		if err != nil {
			return result, fmt.Errorf("项目级配置 %s 解析失败（请检查 JSON 语法）: %w", resolution.Project.Path, err)
		}
		cfg = mergeConfig(cfg, project)
	}
	result.Config = cfg
	return result, nil
}

// loadLegacyConfig preserves the original ainovel-cli behavior exactly:
// global ~/.ainovel is a tolerant base and project ./.ainovel is a fail-loud
// overlay.
func loadLegacyConfig() (Config, error) {
	var cfg Config

	if p := DefaultConfigPath(); p != "" {
		global, found, err := loadOptionalJSON(p)
		switch {
		case err != nil:
			slog.Warn("全局配置解析失败，已忽略（可被项目级覆盖）", "module", "config", "path", p, "err", err)
		case found:
			cfg = global
		}
	}

	project, found, err := loadOptionalJSON(projectConfigPath())
	if err != nil {
		return cfg, fmt.Errorf("项目级配置 ./.ainovel/config.json 解析失败（请检查 JSON 语法）: %w", err)
	}
	if found {
		cfg = mergeConfig(cfg, project)
	}
	return cfg, nil
}

// loadOptionalJSON reads an optional configuration file.
func loadOptionalJSON(path string) (Config, bool, error) {
	cfg, err := loadJSONFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, false, nil
		}
		return Config{}, false, err
	}
	return cfg, true, nil
}

// LoadConfigFile reads one JSON/JSONC configuration file without merging.
func LoadConfigFile(path string) (Config, error) {
	return loadJSONFile(path)
}

func loadJSONFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cleaned := stripJSONComments(data)
	var cfg Config
	if err := json.Unmarshal(cleaned, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// mergeConfig overlays non-zero scalar values and merges maps by key.
func mergeConfig(base, overlay Config) Config {
	if overlay.Provider != "" {
		base.Provider = overlay.Provider
	}
	if overlay.ModelName != "" {
		base.ModelName = overlay.ModelName
	}
	if overlay.ReasoningEffort != "" {
		base.ReasoningEffort = overlay.ReasoningEffort
	}
	if overlay.Style != "" {
		base.Style = overlay.Style
	}
	if overlay.ContextWindow > 0 {
		base.ContextWindow = overlay.ContextWindow
	}

	if len(overlay.Providers) > 0 {
		if base.Providers == nil {
			base.Providers = make(map[string]ProviderConfig)
		}
		for k, v := range overlay.Providers {
			existing := base.Providers[k]
			if v.Type != "" {
				existing.Type = v.Type
			}
			if v.API != "" {
				existing.API = v.API
			}
			if v.APIKey != "" {
				existing.APIKey = v.APIKey
			}
			if v.BaseURL != "" {
				existing.BaseURL = v.BaseURL
			}
			if len(v.Models) > 0 {
				existing.Models = append([]ModelConfig(nil), v.Models...)
			}
			if len(v.ExtraBody) > 0 {
				existing.ExtraBody = cloneMap(v.ExtraBody)
			}
			if len(v.Extra) > 0 {
				existing.Extra = cloneMap(v.Extra)
			}
			if v.StreamIdleTimeout != "" {
				existing.StreamIdleTimeout = v.StreamIdleTimeout
			}
			base.Providers[k] = existing
		}
	}

	if len(overlay.Roles) > 0 {
		if base.Roles == nil {
			base.Roles = make(map[string]RoleConfig)
		}
		for k, v := range overlay.Roles {
			existing := base.Roles[k]
			if v.Provider != "" {
				existing.Provider = v.Provider
			}
			if v.Model != "" {
				existing.Model = v.Model
			}
			if len(v.Fallbacks) > 0 {
				existing.Fallbacks = append([]ModelRef(nil), v.Fallbacks...)
			}
			if v.ReasoningEffort != "" {
				existing.ReasoningEffort = v.ReasoningEffort
			}
			base.Roles[k] = existing
		}
	}

	if overlay.Budget != (BudgetConfig{}) {
		base.Budget = overlay.Budget
	}
	if overlay.Notify.Enabled != nil || overlay.Notify.Command != "" || len(overlay.Notify.Events) > 0 {
		base.Notify = overlay.Notify
	}
	return base
}

func cloneMap(m map[string]any) map[string]any {
	if len(m) == 0 {
		return nil
	}
	c := make(map[string]any, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

// CloneConfig deep-copies mutable configuration maps and slices.
func CloneConfig(cfg Config) Config {
	clone := cfg
	clone.Providers = make(map[string]ProviderConfig, len(cfg.Providers))
	for name, pc := range cfg.Providers {
		pc.Models = append([]ModelConfig(nil), pc.Models...)
		pc.Extra = cloneMap(pc.Extra)
		pc.ExtraBody = cloneMap(pc.ExtraBody)
		clone.Providers[name] = pc
	}
	clone.Roles = make(map[string]RoleConfig, len(cfg.Roles))
	for role, rc := range cfg.Roles {
		rc.Fallbacks = append([]ModelRef(nil), rc.Fallbacks...)
		clone.Roles[role] = rc
	}
	clone.Notify.Events = append([]string(nil), cfg.Notify.Events...)
	return clone
}

// SaveProviderConfig patch-updates one provider in the target configuration.
func SaveProviderConfig(path string, provider string, pc ProviderConfig) error {
	target, found, err := loadOptionalJSON(path)
	if err != nil {
		return err
	}
	if !found {
		target = Config{}
	}
	if target.Providers == nil {
		target.Providers = make(map[string]ProviderConfig)
	}
	target.Providers[provider] = pc
	return SaveConfig(path, target)
}

// stripJSONComments removes // comments while preserving string contents.
func stripJSONComments(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inString := false
	escaped := false

	for i := 0; i < len(data); i++ {
		b := data[i]
		if escaped {
			out = append(out, b)
			escaped = false
			continue
		}
		if inString {
			out = append(out, b)
			if b == '\\' {
				escaped = true
			} else if b == '"' {
				inString = false
			}
			continue
		}
		if b == '"' {
			inString = true
			out = append(out, b)
			continue
		}
		if b == '/' && i+1 < len(data) && data[i+1] == '/' {
			for i < len(data) && data[i] != '\n' {
				i++
			}
			if i < len(data) {
				out = append(out, '\n')
			}
			continue
		}
		out = append(out, b)
	}
	return out
}

// WriteStartupError appends a best-effort startup error to the active runtime
// directory. NovelForge uses .novelforge when present (or selected explicitly)
// and falls back to .ainovel without moving credentials.
func WriteStartupError(msg string) string {
	dir := DefaultConfigDir()
	if dir == "" {
		return ""
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	path := filepath.Join(dir, "last-error.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return ""
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "[%s] %s\n", time.Now().Format(time.RFC3339), msg); err != nil {
		return ""
	}
	return path
}

// SaveConfig atomically writes formatted JSON with owner-only permissions.
func SaveConfig(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}
