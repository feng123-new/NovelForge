package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyLicense(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"Apache License Version 2.0": "Apache-2.0",
		"Permission is hereby granted, free of charge, to any person obtaining a copy":       "MIT",
		"Redistribution and use in source and binary forms ... Neither the name of X":        "BSD-3-Clause",
		"Redistribution and use in source and binary forms are permitted":                    "BSD-2-Clause",
		"Permission to use, copy, modify, and/or distribute this software for any purpose":   "ISC",
		"GNU AFFERO GENERAL PUBLIC LICENSE":                                                  "AGPL",
		"GNU LESSER GENERAL PUBLIC LICENSE":                                                  "LGPL",
		"GNU GENERAL PUBLIC LICENSE":                                                         "GPL",
		"Mozilla Public License Version 2.0":                                                 "MPL-2.0",
		"Mozilla Public License Version 2.0 with GNU General Public License secondary terms": "MPL-2.0",
		"unrecognized terms": "UNKNOWN",
	}
	for text, expected := range tests {
		text, expected := text, expected
		t.Run(expected, func(t *testing.T) {
			t.Parallel()
			if actual := classifyLicense(text); actual != expected {
				t.Fatalf("classifyLicense() = %q, want %q", actual, expected)
			}
		})
	}
}

func TestValidatePolicy(t *testing.T) {
	t.Parallel()
	if err := validatePolicy([]inventoryEntry{{Module: "safe", License: "MIT"}}); err != nil {
		t.Fatalf("safe policy: %v", err)
	}
	if err := validatePolicy([]inventoryEntry{{Module: "unsafe", License: "MIT OR GPL"}}); err == nil {
		t.Fatal("expected GPL policy failure")
	}
}

func TestDetectModuleLicensePrefersPrimaryGrantOverCopyrightNotice(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "LICENSE"), []byte("Permission is hereby granted, free of charge"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "COPYRIGHT"), []byte("Copyright 2026 Example Corp"), 0o644); err != nil {
		t.Fatal(err)
	}
	license, err := detectModuleLicense(root)
	if err != nil {
		t.Fatal(err)
	}
	if license != "MIT" {
		t.Fatalf("license = %q, want MIT", license)
	}
}

func TestDetectModuleLicenseRecognizesHyphenatedGrantFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "LICENSE-MIT"),
		[]byte("Permission is hereby granted, free of charge"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "LICENSE-BSD"),
		[]byte("Redistribution and use in source and binary forms ... Neither the name of X"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	license, err := detectModuleLicense(root)
	if err != nil {
		t.Fatal(err)
	}
	if license != "BSD-3-Clause OR MIT" {
		t.Fatalf("license = %q, want BSD-3-Clause OR MIT", license)
	}
}

func TestReviewedLicenseOverrideIsExact(t *testing.T) {
	t.Parallel()
	if reviewedLicenseOverrides["github.com/mattn/go-localereader@v0.0.1"] != "MIT" {
		t.Fatal("expected reviewed exact-version MIT override")
	}
	if _, ok := reviewedLicenseOverrides["github.com/mattn/go-localereader@v0.0.2"]; ok {
		t.Fatal("license override must not cover unreviewed versions")
	}
}
