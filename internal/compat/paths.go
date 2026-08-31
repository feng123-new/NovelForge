package compat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	ProductDirName = ".novelforge"
	LegacyDirName  = ".ainovel"
	ConfigFileName = "config.json"

	// RuntimeProfileEnv is set internally by cmd/novelforge. The legacy
	// cmd/ainovel-cli entrypoint does not set it and therefore keeps its original
	// path behavior.
	RuntimeProfileEnv        = "NOVELFORGE_INTERNAL_RUNTIME"
	RuntimeProfileNovelForge = "novelforge"
	ExplicitConfigEnv        = "NOVELFORGE_CONFIG"
)

type Scope string

const (
	ScopeExplicit Scope = "explicit"
	ScopeProject  Scope = "project"
	ScopeGlobal   Scope = "global"
)

type Generation string

const (
	GenerationExplicit   Generation = "explicit"
	GenerationNovelForge Generation = "novelforge"
	GenerationLegacy     Generation = "ainovel"
)

type Paths struct {
	Home                    string
	ProjectRoot             string
	GlobalNovelForgeDir     string
	GlobalNovelForgeConfig  string
	GlobalLegacyDir         string
	GlobalLegacyConfig      string
	ProjectNovelForgeDir    string
	ProjectNovelForgeConfig string
	ProjectLegacyDir        string
	ProjectLegacyConfig     string
}

type ConfigCandidate struct {
	Scope      Scope      `json:"scope"`
	Generation Generation `json:"generation"`
	Path       string     `json:"path"`
	Exists     bool       `json:"exists"`
}

// ConfigResolution records the one selected file per scope. NovelForge never
// merges .novelforge and .ainovel files from the same scope: the new path
// shadows the legacy path as a complete layer. Project still overlays global.
type ConfigResolution struct {
	Explicit  *ConfigCandidate `json:"explicit,omitempty"`
	Project   *ConfigCandidate `json:"project,omitempty"`
	Global    *ConfigCandidate `json:"global,omitempty"`
	Effective *ConfigCandidate `json:"effective,omitempty"`
}

// ActivateNovelForgeRuntime switches shared compatibility-aware packages to
// NovelForge path semantics for this process. It does not move or copy files.
func ActivateNovelForgeRuntime() error {
	return os.Setenv(RuntimeProfileEnv, RuntimeProfileNovelForge)
}

func NovelForgeRuntimeActive() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(RuntimeProfileEnv)), RuntimeProfileNovelForge)
}

// ExplicitConfigPath returns the user-supplied NovelForge config path from the
// environment. It is ignored by the legacy command because that command never
// activates the NovelForge runtime profile.
func ExplicitConfigPath() string {
	if !NovelForgeRuntimeActive() {
		return ""
	}
	return strings.TrimSpace(os.Getenv(ExplicitConfigEnv))
}

// SetExplicitConfigPath applies a CLI --config override. Relative paths are
// resolved by ResolveConfig against the selected project root, not here.
func SetExplicitConfigPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	return os.Setenv(ExplicitConfigEnv, path)
}

func ResolvePaths(home, projectRoot string) (Paths, error) {
	var err error
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return Paths{}, fmt.Errorf("resolve home directory: %w", err)
		}
	}
	if projectRoot == "" {
		projectRoot, err = os.Getwd()
		if err != nil {
			return Paths{}, fmt.Errorf("resolve project directory: %w", err)
		}
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home path: %w", err)
	}
	projectRoot, err = filepath.Abs(projectRoot)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve project path: %w", err)
	}

	globalNovelForgeDir := filepath.Join(home, ProductDirName)
	globalLegacyDir := filepath.Join(home, LegacyDirName)
	projectNovelForgeDir := filepath.Join(projectRoot, ProductDirName)
	projectLegacyDir := filepath.Join(projectRoot, LegacyDirName)
	return Paths{
		Home:                    filepath.Clean(home),
		ProjectRoot:             filepath.Clean(projectRoot),
		GlobalNovelForgeDir:     globalNovelForgeDir,
		GlobalNovelForgeConfig:  filepath.Join(globalNovelForgeDir, ConfigFileName),
		GlobalLegacyDir:         globalLegacyDir,
		GlobalLegacyConfig:      filepath.Join(globalLegacyDir, ConfigFileName),
		ProjectNovelForgeDir:    projectNovelForgeDir,
		ProjectNovelForgeConfig: filepath.Join(projectNovelForgeDir, ConfigFileName),
		ProjectLegacyDir:        projectLegacyDir,
		ProjectLegacyConfig:     filepath.Join(projectLegacyDir, ConfigFileName),
	}, nil
}

// ConfigCandidates returns candidates in effective high-to-low order.
func (p Paths) ConfigCandidates() []ConfigCandidate {
	return []ConfigCandidate{
		candidate(ScopeProject, GenerationNovelForge, p.ProjectNovelForgeConfig),
		candidate(ScopeProject, GenerationLegacy, p.ProjectLegacyConfig),
		candidate(ScopeGlobal, GenerationNovelForge, p.GlobalNovelForgeConfig),
		candidate(ScopeGlobal, GenerationLegacy, p.GlobalLegacyConfig),
	}
}

// SelectedConfig returns exactly one complete layer for a scope. A present
// .novelforge file shadows .ainovel even when the legacy file is also present.
func (p Paths) SelectedConfig(scope Scope) (ConfigCandidate, bool) {
	for _, current := range p.ConfigCandidates() {
		if current.Scope == scope && current.Exists {
			return current, true
		}
	}
	return ConfigCandidate{}, false
}

func (p Paths) EffectiveConfig() (ConfigCandidate, bool) {
	resolution, err := p.ResolveConfig("")
	if err != nil || resolution.Effective == nil {
		return ConfigCandidate{}, false
	}
	return *resolution.Effective, true
}

// ResolveConfig applies the NovelForge precedence contract:
//
//	explicit --config / NOVELFORGE_CONFIG
//	project .novelforge
//	project .ainovel
//	global .novelforge
//	global .ainovel
//
// Explicit configuration is authoritative and is not merged with project or
// global files. Project and global selections remain in the report so doctor
// can explain what was shadowed.
func (p Paths) ResolveConfig(explicit string) (ConfigResolution, error) {
	var resolution ConfigResolution
	if project, ok := p.SelectedConfig(ScopeProject); ok {
		projectCopy := project
		resolution.Project = &projectCopy
	}
	if global, ok := p.SelectedConfig(ScopeGlobal); ok {
		globalCopy := global
		resolution.Global = &globalCopy
	}

	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		if !filepath.IsAbs(explicit) {
			explicit = filepath.Join(p.ProjectRoot, explicit)
		}
		absolute, err := filepath.Abs(explicit)
		if err != nil {
			return ConfigResolution{}, fmt.Errorf("resolve explicit config: %w", err)
		}
		current := candidate(ScopeExplicit, GenerationExplicit, filepath.Clean(absolute))
		resolution.Explicit = &current
		resolution.Effective = &current
		return resolution, nil
	}

	if resolution.Project != nil {
		effective := *resolution.Project
		resolution.Effective = &effective
	} else if resolution.Global != nil {
		effective := *resolution.Global
		resolution.Effective = &effective
	}
	return resolution, nil
}

func candidate(scope Scope, generation Generation, path string) ConfigCandidate {
	info, err := os.Stat(path)
	return ConfigCandidate{
		Scope:      scope,
		Generation: generation,
		Path:       path,
		Exists:     err == nil && info.Mode().IsRegular(),
	}
}
