package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerConfigFlagParsesBeforeUnexpectedArguments(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = file
	defer func() { os.Stderr = old; _ = file.Close() }()
	code := runServerCommand([]string{"--config", "unused.json", "unexpected"})
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if code != 2 || !strings.Contains(string(data), "unexpected arguments") || strings.Contains(string(data), "flag provided but not defined") {
		t.Fatalf("exit=%d stderr=%s", code, data)
	}
}
