// Command dependency_license_inventory creates the reviewed module-license
// inventory committed with NovelForge releases. It intentionally uses only the
// Go toolchain and standard library so the license check cannot hide another
// dependency.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type module struct {
	Path    string
	Version string
	Dir     string
	Main    bool
	Replace *module
}

type inventoryEntry struct {
	Module  string
	Version string
	License string
}

var forbiddenLicenses = map[string]struct{}{
	"AGPL":    {},
	"GPL":     {},
	"LGPL":    {},
	"SSPL":    {},
	"UNKNOWN": {},
	"REVIEW":  {},
}

// reviewedLicenseOverrides is intentionally exact and may only be used when a
// published module snapshot omits a license file but an auditable upstream
// grant covers the identical source. Every entry needs a written review in
// docs/LICENSES.md; broad module- or vendor-level overrides are prohibited.
var reviewedLicenseOverrides = map[string]string{
	"github.com/mattn/go-localereader@v0.0.1": "MIT",
}

func main() {
	checkPath := flag.String("check", "", "compare generated inventory with this file")
	flag.Parse()

	inventory, entries, err := buildInventory()
	if err != nil {
		fatal(err)
	}
	if *checkPath == "" {
		if _, err := os.Stdout.Write(inventory); err != nil {
			fatal(err)
		}
		return
	}
	if err := validatePolicy(entries); err != nil {
		fatal(err)
	}
	existing, err := os.ReadFile(*checkPath)
	if err != nil {
		fatal(fmt.Errorf("read committed inventory: %w", err))
	}
	if !bytes.Equal(existing, inventory) {
		fatal(errors.New("dependency license inventory is stale; regenerate it with go run ./scripts/dependency_license_inventory.go"))
	}
}

func buildInventory() ([]byte, []inventoryEntry, error) {
	modules, err := listModules()
	if err != nil {
		return nil, nil, err
	}
	entries := make([]inventoryEntry, 0, len(modules))
	for _, item := range modules {
		if item.Main {
			continue
		}
		effective := item
		if item.Replace != nil {
			effective = *item.Replace
		}
		license, err := detectModuleLicense(effective.Dir)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", item.Path, err)
		}
		if license == "UNKNOWN" {
			if reviewed, ok := reviewedLicenseOverrides[item.Path+"@"+item.Version]; ok {
				license = reviewed
			}
		}
		version := item.Version
		if item.Replace != nil {
			replacement := item.Replace.Path
			if item.Replace.Version != "" {
				replacement += "@" + item.Replace.Version
			}
			version += " → " + replacement
		}
		entries = append(entries, inventoryEntry{
			Module:  item.Path,
			Version: version,
			License: license,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Module == entries[j].Module {
			return entries[i].Version < entries[j].Version
		}
		return entries[i].Module < entries[j].Module
	})

	var output strings.Builder
	output.WriteString("# Dependency License Inventory\n\n")
	output.WriteString("Generated deterministically from `go list -m -json all` and the license files in the Go module cache. Regenerate with:\n\n")
	output.WriteString("```sh\nGOWORK=off go mod download all\nGOWORK=off go run ./scripts/dependency_license_inventory.go > docs/DEPENDENCY_LICENSES.md\n```\n\n")
	output.WriteString("| Module | Version | Detected license |\n")
	output.WriteString("|---|---:|---|\n")
	for _, entry := range entries {
		fmt.Fprintf(
			&output,
			"| `%s` | `%s` | %s |\n",
			escapeTable(entry.Module),
			escapeTable(entry.Version),
			escapeTable(entry.License),
		)
	}
	return []byte(output.String()), entries, nil
}

func listModules() ([]module, error) {
	command := exec.Command("go", "list", "-m", "-json", "all")
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("go list modules: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("go list modules: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	var modules []module
	for {
		var item module
		if err := decoder.Decode(&item); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("decode module graph: %w", err)
		}
		modules = append(modules, item)
	}
	return modules, nil
}

func detectModuleLicense(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "UNKNOWN", nil
	}
	items, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("read module directory: %w", err)
	}
	var primary, fallback []string
	for _, item := range items {
		if item.IsDir() {
			continue
		}
		name := strings.ToLower(item.Name())
		path := filepath.Join(root, item.Name())
		switch {
		case name == "license" || strings.HasPrefix(name, "license.") || strings.HasPrefix(name, "license-"),
			name == "copying" || strings.HasPrefix(name, "copying.") || strings.HasPrefix(name, "copying-"):
			primary = append(primary, path)
		case name == "copyright" || strings.HasPrefix(name, "copyright.") || strings.HasPrefix(name, "copyright-"):
			// COPYRIGHT files often contain notices rather than grant terms.
			// Treat them only as a fallback when the module ships no primary
			// LICENSE/COPYING file, otherwise an unrelated notice can turn a
			// known permissive license into an UNKNOWN composite result.
			fallback = append(fallback, path)
		}
	}
	candidates := primary
	if len(candidates) == 0 {
		candidates = fallback
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		return "UNKNOWN", nil
	}
	licenses := make(map[string]struct{})
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", filepath.Base(candidate), err)
		}
		licenses[classifyLicense(string(data))] = struct{}{}
	}
	result := make([]string, 0, len(licenses))
	for license := range licenses {
		result = append(result, license)
	}
	sort.Strings(result)
	return strings.Join(result, " OR "), nil
}

func classifyLicense(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "server side public license"):
		return "SSPL"
	// MPL-2.0 names GPL-family licenses as optional secondary licenses. Check
	// its own grant first so those references cannot create a false GPL result.
	case strings.Contains(lower, "mozilla public license") && strings.Contains(lower, "version 2.0"):
		return "MPL-2.0"
	case strings.Contains(lower, "gnu affero general public license"):
		return "AGPL"
	case strings.Contains(lower, "gnu lesser general public license"):
		return "LGPL"
	case strings.Contains(lower, "gnu general public license"):
		return "GPL"
	case strings.Contains(lower, "eclipse public license"):
		return "REVIEW"
	case strings.Contains(lower, "apache license") && strings.Contains(lower, "version 2.0"):
		return "Apache-2.0"
	case strings.Contains(lower, "permission is hereby granted, free of charge"):
		return "MIT"
	case strings.Contains(lower, "redistribution and use in source and binary forms") &&
		strings.Contains(lower, "neither the name"):
		return "BSD-3-Clause"
	case strings.Contains(lower, "redistribution and use in source and binary forms"):
		return "BSD-2-Clause"
	case strings.Contains(lower, "permission to use, copy, modify, and/or distribute this software"):
		return "ISC"
	case strings.Contains(lower, "boost software license"):
		return "BSL-1.0"
	case strings.Contains(lower, "creative commons zero") || strings.Contains(lower, "cc0 1.0 universal"):
		return "CC0-1.0"
	case strings.Contains(lower, "this is free and unencumbered software released into the public domain"):
		return "Unlicense"
	case strings.Contains(lower, "zlib license") ||
		(strings.Contains(lower, "this software is provided 'as-is'") && strings.Contains(lower, "altered source versions must be plainly marked")):
		return "Zlib"
	default:
		return "UNKNOWN"
	}
}

func validatePolicy(entries []inventoryEntry) error {
	var violations []string
	for _, entry := range entries {
		for _, license := range strings.Split(entry.License, " OR ") {
			if _, forbidden := forbiddenLicenses[license]; forbidden {
				violations = append(violations, entry.Module+" ("+license+")")
			}
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return fmt.Errorf("dependency licenses require review or are forbidden: %s", strings.Join(violations, ", "))
	}
	return nil
}

func escapeTable(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
