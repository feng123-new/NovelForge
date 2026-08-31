package main

import "testing"

func TestParseCLIOptionsAcceptsExplicitConfig(t *testing.T) {
	opts, args, err := parseCLIOptions([]string{"--headless", "--config", "configs/book.json", "--prompt", "write chapter one"})
	if err != nil {
		t.Fatalf("parseCLIOptions: %v", err)
	}
	if opts.ConfigFile != "configs/book.json" || !opts.Headless || opts.Prompt != "write chapter one" || len(args) != 0 {
		t.Fatalf("unexpected options: %#v args=%#v", opts, args)
	}
}

func TestParseCLIOptionsRejectsMissingConfigValue(t *testing.T) {
	if _, _, err := parseCLIOptions([]string{"--config"}); err == nil {
		t.Fatal("expected missing --config value error")
	}
}

func TestParseCLIOptionsRejectsConfigWithVersionOrUpdate(t *testing.T) {
	for _, argv := range [][]string{{"version", "--config", "config.json"}, {"update", "v0.1.0", "--config", "config.json"}} {
		if _, _, err := parseCLIOptions(argv); err == nil {
			t.Fatalf("expected conflict for %#v", argv)
		}
	}
}
