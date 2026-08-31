package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

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
	Product      string                  `json:"product"`
	Status       string                  `json:"status"`
	Home         string                  `json:"home"`
	ProjectRoot  string                  `json:"project_root"`
	ConfigSource *compat.ConfigCandidate `json:"config_source,omitempty"`
	Checks       []doctorCheck           `json:"checks"`
}

func runDoctorCommand(args []string) int {
	return runDoctorCommandWith(args, os.Stdout, os.Stderr)
}

func runDoctorCommandWith(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	home := flags.String("home", "", "override the user home directory for diagnostics")
	projectRoot := flags.String("project-root", "", "project directory to inspect (default: current directory)")
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

	report, err := buildDoctorReport(*home, *projectRoot)
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

func buildDoctorReport(home, projectRoot string) (doctorReport, error) {
	paths, err := compat.ResolvePaths(home, projectRoot)
	if err != nil {
		return doctorReport{}, err
	}
	report := doctorReport{
		Product:     "NovelForge",
		Status:      "ok",
		Home:        paths.Home,
		ProjectRoot: paths.ProjectRoot,
	}

	if info, err := os.Stat(paths.ProjectRoot); err != nil {
		report.addCheck("project.root", "error", err.Error(), paths.ProjectRoot)
	} else if !info.IsDir() {
		report.addCheck("project.root", "error", "project root is not a directory", paths.ProjectRoot)
	} else {
		report.addCheck("project.root", "ok", "project root is accessible", paths.ProjectRoot)
	}

	candidates := paths.ConfigCandidates()
	for _, candidate := range candidates {
		if !candidate.Exists {
			continue
		}
		if _, err := bootstrap.LoadConfigFile(candidate.Path); err != nil {
			report.addCheck(
				"config."+string(candidate.Scope)+"."+string(candidate.Generation),
				"error",
				"configuration cannot be parsed: "+err.Error(),
				candidate.Path,
			)
			continue
		}
		report.addCheck(
			"config."+string(candidate.Scope)+"."+string(candidate.Generation),
			"ok",
			"configuration is readable",
			candidate.Path,
		)
	}

	runtimeSource, runtimeFound := legacyRuntimeConfig(candidates)
	if runtimeFound {
		report.ConfigSource = &runtimeSource
		report.addCheck("config.source", "warning", "the current runtime still reads .ainovel; a backed-up .novelforge copy can be prepared with novelforge migrate", runtimeSource.Path)
	} else if preferred, ok := preferredConfig(candidates); ok {
		report.ConfigSource = &preferred
		report.addCheck("config.source", "error", ".novelforge exists without a legacy runtime source; keep .ainovel until the runtime precedence switch lands", preferred.Path)
	} else {
		report.addCheck("config.source", "error", "no configuration file was found", "")
	}

	checkConfigOverlap(&report, candidates, compat.ScopeProject)
	checkConfigOverlap(&report, candidates, compat.ScopeGlobal)
	return report, nil
}

func legacyRuntimeConfig(candidates []compat.ConfigCandidate) (compat.ConfigCandidate, bool) {
	for _, scope := range []compat.Scope{compat.ScopeProject, compat.ScopeGlobal} {
		for _, candidate := range candidates {
			if candidate.Scope == scope && candidate.Generation == compat.GenerationLegacy && candidate.Exists {
				return candidate, true
			}
		}
	}
	return compat.ConfigCandidate{}, false
}

func preferredConfig(candidates []compat.ConfigCandidate) (compat.ConfigCandidate, bool) {
	for _, scope := range []compat.Scope{compat.ScopeProject, compat.ScopeGlobal} {
		for _, candidate := range candidates {
			if candidate.Scope == scope && candidate.Generation == compat.GenerationNovelForge && candidate.Exists {
				return candidate, true
			}
		}
	}
	return compat.ConfigCandidate{}, false
}

func checkConfigOverlap(report *doctorReport, candidates []compat.ConfigCandidate, scope compat.Scope) {
	var preferred, legacy *compat.ConfigCandidate
	for i := range candidates {
		candidate := &candidates[i]
		if candidate.Scope != scope || !candidate.Exists {
			continue
		}
		switch candidate.Generation {
		case compat.GenerationNovelForge:
			preferred = candidate
		case compat.GenerationLegacy:
			legacy = candidate
		}
	}
	if preferred != nil && legacy != nil {
		report.addCheck(
			"config."+string(scope)+".precedence",
			"warning",
			"both .novelforge and .ainovel exist; this iteration keeps .ainovel as the runtime source while preserving the staged copy",
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
