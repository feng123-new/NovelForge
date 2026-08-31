package compat

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	ProductDirName = ".novelforge"
	LegacyDirName  = ".ainovel"
	ConfigFileName = "config.json"
)

type Scope string

const (
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
)

type Generation string

const (
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

func (p Paths) ConfigCandidates() []ConfigCandidate {
	return []ConfigCandidate{
		candidate(ScopeProject, GenerationNovelForge, p.ProjectNovelForgeConfig),
		candidate(ScopeProject, GenerationLegacy, p.ProjectLegacyConfig),
		candidate(ScopeGlobal, GenerationNovelForge, p.GlobalNovelForgeConfig),
		candidate(ScopeGlobal, GenerationLegacy, p.GlobalLegacyConfig),
	}
}

func (p Paths) EffectiveConfig() (ConfigCandidate, bool) {
	for _, candidate := range p.ConfigCandidates() {
		if candidate.Exists {
			return candidate, true
		}
	}
	return ConfigCandidate{}, false
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
