package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/compat"
)

type doctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

type doctorReport struct {
	Product      string                   `json:"product"`
	Status       string                   `json:"status"`
	Home         string                   `json:"home"`
	ProjectRoot  string                   `json:"project_root"`
	ConfigSource *compat.ConfigCandidate  `json:"config_source,omitempty"`
	ConfigLayers []compat.ConfigCandidate `json:"config_layers,omitempty"`
	Checks       []doctorCheck            `json:"checks"`
}

func runDoctorCommand(args []string) int {
	return runDoctorCommandWith(args, os.Stdout, os.Stderr)
}

func runDoctorCommandWith(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	home := flags.String("home", "", "override the user home directory for diagnostics")
	projectRoot := flags.String("project-root", "", "project directory to inspect (default: current directory)")
	explicitConfig := flags.String("config", "", "inspect an explicit configuration file (same precedence as --config)")
	jsonOutput := flags.Bool("json", false, "write the report as JSON")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "doctor does not accept positional arguments")
		return 2
	}
	if *explicitConfig == "" {
		*explicitConfig = compat.ExplicitConfigPath()
	}

	report, err := buildDoctorReport(*home, *projectRoot, *explicitConfig)
	if err != nil {
		fmt.Fprintf(stderr, "doctor: %v\n", err)
		return 1
	}
	if *jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(stderr, "doctor: write report: %v\n", err)
			return 1
		}
	} else {
		printDoctorReport(stdout, report)
	}
	if report.Status == "error" {
		return 1
	}
	return 0
}

func buildDoctorReport(home, projectRoot, explicit string) (doctorReport, error) {
	paths, err := compat.ResolvePaths(home, projectRoot)
	if err != nil {
		return doctorReport{}, err
	}
	resolution, err := paths.ResolveConfig(explicit)
	if err != nil {
		return doctorReport{}, err
	}
	report := doctorReport{
		Product:     "NovelForge",
		Status:      "ok",
		Home:        paths.Home,
		ProjectRoot: paths.ProjectRoot,
	}
	if resolution.Effective != nil {
		effective := *resolution.Effective
		report.ConfigSource = &effective
	}
	for _, layer := range activeConfigLayers(resolution) {
		report.ConfigLayers = append(report.ConfigLayers, layer)
	}

	if info, err := os.Stat(paths.ProjectRoot); err != nil {
		report.addCheck("project.root", "error", err.Error(), paths.ProjectRoot)
	} else if !info.IsDir() {
		report.addCheck("project.root", "error", "project root is not a directory", paths.ProjectRoot)
	} else {
		report.addCheck("project.root", "ok", "project root is accessible", paths.ProjectRoot)
	}

	active := make(map[string]compat.ConfigCandidate)
	for _, layer := range activeConfigLayers(resolution) {
		active[filepath.Clean(layer.Path)] = layer
	}
	for _, current := range doctorCandidates(paths, resolution) {
		name := "config." + string(current.Scope) + "." + string(current.Generation)
		if !current.Exists {
			if current.Scope == compat.ScopeExplicit {
				report.addCheck(name, "error", "explicit configuration does not exist", current.Path)
			}
			continue
		}
		if _, ok := active[filepath.Clean(current.Path)]; !ok {
			report.addCheck(name, "info", "configuration is shadowed by a higher-precedence layer and will not be merged", current.Path)
			continue
		}
		if _, err := bootstrap.LoadConfigFile(current.Path); err != nil {
			status := "error"
			message := "active configuration cannot be parsed: " + err.Error()
			if current.Scope == compat.ScopeGlobal && resolution.Project != nil {
				status = "warning"
				message = "global configuration cannot be parsed; a project layer may still provide a complete configuration: " + err.Error()
			}
			report.addCheck(name, status, message, current.Path)
			continue
		}
		if current.Generation == compat.GenerationLegacy {
			report.addCheck(name, "warning", "using the .ainovel compatibility fallback; run novelforge migrate to prepare a backed-up .novelforge copy", current.Path)
		} else {
			report.addCheck(name, "ok", "configuration is readable and selected", current.Path)
		}
	}

	checkConfigOverlap(&report, paths.ConfigCandidates(), compat.ScopeProject)
	checkConfigOverlap(&report, paths.ConfigCandidates(), compat.ScopeGlobal)

	if resolution.Effective == nil {
		report.addCheck("config.source", "error", "no configuration file was found", "")
		return report, nil
	}

	loaded, loadErr := bootstrap.LoadNovelForgeConfig(paths.Home, paths.ProjectRoot, explicit)
	for _, warning := range loaded.Warnings {
		report.addCheck("config.load."+string(warning.Scope), "warning", warning.Err.Error(), warning.Path)
	}
	if loadErr != nil {
		report.addCheck("config.load", "error", loadErr.Error(), resolution.Effective.Path)
		return report, nil
	}
	cfg := loaded.Config
	cfg.FillDefaults()
	if err := cfg.ValidateBase(); err != nil {
		report.addCheck("config.validation", "error", err.Error(), resolution.Effective.Path)
	} else {
		report.addCheck("config.validation", "ok", "effective configuration passed runtime validation", resolution.Effective.Path)
	}
	return report, nil
}

func activeConfigLayers(resolution compat.ConfigResolution) []compat.ConfigCandidate {
	if resolution.Explicit != nil {
		return []compat.ConfigCandidate{*resolution.Explicit}
	}
	var layers []compat.ConfigCandidate
	if resolution.Global != nil {
		layers = append(layers, *resolution.Global)
	}
	if resolution.Project != nil {
		layers = append(layers, *resolution.Project)
	}
	return layers
}

func doctorCandidates(paths compat.Paths, resolution compat.ConfigResolution) []compat.ConfigCandidate {
	var candidates []compat.ConfigCandidate
	seen := make(map[string]bool)
	appendCandidate := func(current compat.ConfigCandidate) {
		key := filepath.Clean(current.Path)
		if seen[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, current)
	}
	if resolution.Explicit != nil {
		appendCandidate(*resolution.Explicit)
	}
	for _, current := range paths.ConfigCandidates() {
		appendCandidate(current)
	}
	return candidates
}

func checkConfigOverlap(report *doctorReport, candidates []compat.ConfigCandidate, scope compat.Scope) {
	var preferred, legacy *compat.ConfigCandidate
	for i := range candidates {
		current := &candidates[i]
		if current.Scope != scope || !current.Exists {
			continue
		}
		switch current.Generation {
		case compat.GenerationNovelForge:
			preferred = current
		case compat.GenerationLegacy:
			legacy = current
		}
	}
	if preferred != nil && legacy != nil {
		report.addCheck(
			"config."+string(scope)+".precedence",
			"info",
			"both directories exist; .novelforge is active and .ainovel is retained unchanged for rollback and legacy compatibility",
			preferred.Path,
		)
	}
}

func (r *doctorReport) addCheck(name, status, message, path string) {
	r.Checks = append(r.Checks, doctorCheck{Name: name, Status: status, Message: message, Path: path})
	switch status {
	case "error":
		r.Status = "error"
	case "warning":
		if r.Status == "ok" {
			r.Status = "warning"
		}
	}
}

func printDoctorReport(w io.Writer, report doctorReport) {
	fmt.Fprintf(w, "NovelForge doctor: %s\n", report.Status)
	fmt.Fprintf(w, "home: %s\n", report.Home)
	fmt.Fprintf(w, "project: %s\n", report.ProjectRoot)
	for _, check := range report.Checks {
		marker := "OK"
		switch check.Status {
		case "info":
			marker = "INFO"
		case "warning":
			marker = "WARN"
		case "error":
			marker = "ERROR"
		}
		fmt.Fprintf(w, "[%s] %s: %s", marker, check.Name, check.Message)
		if check.Path != "" {
			fmt.Fprintf(w, " (%s)", check.Path)
		}
		fmt.Fprintln(w)
	}
}

func runMigrateCommand(args []string) int {
	return runMigrateCommandWith(context.Background(), args, os.Stdout, os.Stderr)
}

func runMigrateCommandWith(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	home := flags.String("home", "", "override the user home directory")
	projectRoot := flags.String("project-root", "", "project directory to migrate (default: current directory)")
	dryRun := flags.Bool("dry-run", false, "show migration actions without writing files")
	globalOnly := flags.Bool("global-only", false, "migrate only the global ~/.ainovel directory")
	projectOnly := flags.Bool("project-only", false, "migrate only the project ./.ainovel directory")
	jsonOutput := flags.Bool("json", false, "write the report as JSON")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "migrate does not accept positional arguments")
		return 2
	}
	if *globalOnly && *projectOnly {
		fmt.Fprintln(stderr, "migrate: --global-only and --project-only cannot be combined")
		return 2
	}

	options := compat.MigrationOptions{
		Home:           *home,
		ProjectRoot:    *projectRoot,
		DryRun:         *dryRun,
		IncludeGlobal:  !*projectOnly,
		IncludeProject: !*globalOnly,
	}
	report, migrateErr := compat.Migrate(ctx, options)
	if *jsonOutput {
		payload := struct {
			compat.MigrationReport
			Error string `json:"error,omitempty"`
		}{MigrationReport: report}
		if migrateErr != nil {
			payload.Error = migrateErr.Error()
		}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(payload); err != nil {
			fmt.Fprintf(stderr, "migrate: write report: %v\n", err)
			return 1
		}
	} else {
		printMigrationReport(stdout, report)
	}
	if migrateErr != nil {
		fmt.Fprintf(stderr, "migrate: %v\n", migrateErr)
		return 1
	}
	return 0
}

func printMigrationReport(w io.Writer, report compat.MigrationReport) {
	mode := "apply"
	if report.DryRun {
		mode = "dry-run"
	}
	fmt.Fprintf(w, "NovelForge migration (%s)\n", mode)
	for _, action := range report.Actions {
		fmt.Fprintf(w, "[%s] %s: %s -> %s", action.Status, action.Scope, action.Source, action.Destination)
		if action.Backup != "" {
			fmt.Fprintf(w, " (backup: %s)", action.Backup)
		}
		if action.Message != "" {
			fmt.Fprintf(w, " — %s", action.Message)
		}
		fmt.Fprintln(w)
	}
}
