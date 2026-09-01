package project

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListChaptersIsStableAndBounded(t *testing.T) {
	repository, err := NewRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := repository.Create(context.Background(), CreateInput{Title: "Sky Road"})
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(repository.Workspace(), created.Path, "chapters")
	if err := os.WriteFile(filepath.Join(root, "chapter-010.md"), []byte("# Ten\n后来的章节。"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "chapter-002.md"), []byte("# Two\n先发生的章节。"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "chapter-003.md")
	if err := os.WriteFile(outside, []byte("# Outside\nMust not be read."), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "chapter-003.md")); err != nil {
		t.Logf("symlink test skipped on this platform: %v", err)
	}
	page, err := repository.ListChapters(context.Background(), created.ID, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Chapters) != 1 || page.Chapters[0].Chapter != 2 {
		t.Fatalf("unexpected page: %#v", page)
	}
	if page.NextOffset == nil || *page.NextOffset != 1 || page.Chapters[0].CharacterCount == 0 {
		t.Fatalf("unexpected pagination or count: %#v", page)
	}
}

func TestFoundationRequestIsPersistedWithoutSecrets(t *testing.T) {
	repository, err := NewRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := repository.Create(context.Background(), CreateInput{Title: "Foundation"})
	if err != nil {
		t.Fatal(err)
	}
	request, err := repository.SaveFoundationRequest(context.Background(), created.ID, FoundationRequestInput{
		Idea:         "A courier carries a forbidden map across a broken empire.",
		Style:        "Close third person, restrained and tense.",
		ModelProfile: map[string]string{"architect": "openai/gpt-test"},
		Automation: AutomationSettings{
			Mode:         "copilot",
			ReviewPolicy: "every_chapter",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.Status != "requested" || request.Automation.WorkerAvailable || request.Automation.MaxRewrites != 2 {
		t.Fatalf("unexpected request: %#v", request)
	}
	stored, err := repository.GetFoundationRequest(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != request.ID || stored.Idea != request.Idea {
		t.Fatalf("stored request mismatch: %#v", stored)
	}

	_, err = repository.SaveFoundationRequest(context.Background(), created.ID, FoundationRequestInput{
		Idea:         "unsafe",
		ModelProfile: map[string]string{"api_key": "sk-do-not-store"},
		Automation: AutomationSettings{
			Mode:         "copilot",
			ReviewPolicy: "every_chapter",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("expected secret rejection, got %v", err)
	}

	_, err = repository.SaveFoundationRequest(context.Background(), created.ID, FoundationRequestInput{
		Idea: "Use api_key=sk-do-not-store for the provider",
		Automation: AutomationSettings{
			Mode:         "copilot",
			ReviewPolicy: "every_chapter",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("expected prose secret rejection, got %v", err)
	}
}
