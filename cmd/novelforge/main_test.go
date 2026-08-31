package main

import (
	"bytes"
	"strings"
	"testing"

	buildversion "github.com/voocel/ainovel-cli/internal/version"
)

func TestParseCLIOptionsPreservesHeadlessEntry(t *testing.T) {
	opts, args, err := parseCLIOptions([]string{"--headless", "--prompt", "write chapter one"})
	if err != nil {
		t.Fatalf("parseCLIOptions: %v", err)
	}
	if !opts.Headless || opts.Prompt != "write chapter one" || len(args) != 0 {
		t.Fatalf("unexpected options: %#v args=%#v", opts, args)
	}
}

func TestParseCLIOptionsRejectsConflictingPromptSources(t *testing.T) {
	_, _, err := parseCLIOptions([]string{"--headless", "--prompt", "a", "--prompt-file", "b"})
	if err == nil {
		t.Fatal("expected conflicting prompt error")
	}
}

func TestPrintVersionUsesNovelForgeBrand(t *testing.T) {
	var output bytes.Buffer
	printVersion(&output, buildversion.Info{Version: "v0.1.0", Commit: "abc", Date: "2026-08-31"})
	if got := output.String(); !strings.HasPrefix(got, "novelforge v0.1.0\n") || strings.Contains(got, "ainovel-cli") {
		t.Fatalf("unexpected version output: %q", got)
	}
}

func TestIsLoopbackHost(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "::1", "localhost", "LOCALHOST"} {
		if !isLoopbackHost(host) {
			t.Errorf("expected loopback: %q", host)
		}
	}
	for _, host := range []string{"0.0.0.0", "::", "192.0.2.10", "example.com"} {
		if isLoopbackHost(host) {
			t.Errorf("expected non-loopback: %q", host)
		}
	}
}
