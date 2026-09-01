package project

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCreateProjectInitializesLayoutAndDatabase(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	project, err := repository.Create(context.Background(), CreateInput{
		Title:           "Sky Road",
		Genre:           "fantasy",
		Language:        "zh-CN",
		TargetWords:     1_000_000,
		TargetChapters:  300,
		WordsPerChapter: 3500,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if project.ID == "" || strings.Contains(project.ID, "sky-road") {
		t.Fatalf("opaque project ID = %q", project.ID)
	}
	root := filepath.Join(repository.workspace, project.Path)
	for _, relative := range []string{
		".novelforge/project.json",
		".novelforge/project.db",
		".novelforge/config.json",
		".novelforge/rules",
		".novelforge/skills",
		".novelforge/style",
		".novelforge/output",
		".novelforge/backups",
		".novelforge/trash",
		"chapters",
		"references",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err != nil {
			t.Fatalf("%s: %v", relative, err)
		}
	}
	if project.TotalChapters != 300 || project.TargetWords != 1_000_000 {
		t.Fatalf("project = %#v", project)
	}
}

func TestLegacyProjectStillReadable(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	root := filepath.Join(repository.workspace, "legacy")
	mustMkdirAll(t, filepath.Join(root, "meta"))
	mustMkdirAll(t, filepath.Join(root, "chapters"))
	mustWrite(t, filepath.Join(root, "meta", "book.json"), `{"title":"旧书","synopsis":"兼容测试"}`)
	mustWrite(t, filepath.Join(root, "meta", "progress.json"), `{
		"current_chapter": 12,
		"in_progress_chapter": 13,
		"total_chapters": 200,
		"total_word_count": 42000,
		"completed_chapters": [1,2,3]
	}`)

	result, err := repository.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Projects) != 1 {
		t.Fatalf("projects = %#v", result.Projects)
	}
	project, err := repository.Get(context.Background(), result.Projects[0].ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if project.Title != "旧书" ||
		project.CurrentChapter != 13 ||
		project.CompletedChapters != 3 ||
		project.SourceFormat != "ainovel" {
		t.Fatalf("legacy project = %#v", project)
	}
	if filepath.IsAbs(project.Path) {
		t.Fatalf("API path must be relative: %q", project.Path)
	}
}

func TestDuplicateScrubsSecretsAndPreservesContent(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	source, err := repository.Create(context.Background(), CreateInput{Title: "Original"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sourceRoot := filepath.Join(repository.workspace, source.Path)
	mustWrite(t, filepath.Join(sourceRoot, "chapters", "001.md"), "# Chapter 1")
	mustWrite(t, filepath.Join(sourceRoot, ".novelforge", "config.json"), `{
		"version": 1,
		"provider": {
			"name": "openai-compatible",
			"api_key": "must-not-copy",
			"nested": {"access_token": "must-not-copy"}
		},
		"model": "test-model"
	}`)
	mustWrite(t, filepath.Join(sourceRoot, ".ainovel", "config.json"), `{"api_key":"legacy-secret"}`)
	mustWrite(t, filepath.Join(sourceRoot, ".env.production"), "TOKEN=environment-secret")

	duplicate, err := repository.Duplicate(
		context.Background(),
		source.ID,
		DuplicateInput{Title: "Copy"},
	)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if duplicate.ID == source.ID {
		t.Fatal("duplicate reused project ID")
	}
	duplicateRoot := filepath.Join(repository.workspace, duplicate.Path)
	content, err := os.ReadFile(filepath.Join(duplicateRoot, "chapters", "001.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# Chapter 1" {
		t.Fatalf("chapter = %q", content)
	}
	config, err := os.ReadFile(filepath.Join(duplicateRoot, ".novelforge", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(config), "must-not-copy") ||
		strings.Contains(strings.ToLower(string(config)), "api_key") ||
		strings.Contains(strings.ToLower(string(config)), "access_token") {
		t.Fatalf("secret leaked into duplicate: %s", config)
	}
	for _, forbidden := range []string{".ainovel/config.json", ".env.production"} {
		if _, err := os.Stat(filepath.Join(duplicateRoot, filepath.FromSlash(forbidden))); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("sensitive file %s was copied: %v", forbidden, err)
		}
	}
}

func TestArchiveUnarchiveAndUpdate(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	created, err := repository.Create(context.Background(), CreateInput{Title: "Book"})
	if err != nil {
		t.Fatal(err)
	}
	archived, err := repository.SetArchived(context.Background(), created.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !archived.Archived || archived.Status != StatusArchived || archived.ArchivedAt == nil {
		t.Fatalf("archived = %#v", archived)
	}
	title := "Renamed"
	chapters := 500
	updated, err := repository.Update(context.Background(), created.ID, UpdateInput{
		Title:          &title,
		TargetChapters: &chapters,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != title || updated.TotalChapters != chapters {
		t.Fatalf("updated = %#v", updated)
	}
	active, err := repository.SetArchived(context.Background(), created.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if active.Archived || active.Status != StatusActive || active.ArchivedAt != nil {
		t.Fatalf("active = %#v", active)
	}
}

func TestDeleteRequiresConfirmationAndMovesToWorkspaceTrash(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	project, err := repository.Create(context.Background(), CreateInput{Title: "Delete Me"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Delete(
		context.Background(),
		project.ID,
		DeleteInput{Confirm: "wrong"},
	); !errors.Is(err, ErrConfirmation) {
		t.Fatalf("wrong confirmation error = %v", err)
	}
	result, err := repository.Delete(
		context.Background(),
		project.ID,
		DeleteInput{Confirm: project.ID},
	)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !result.Deleted || result.Permanent {
		t.Fatalf("result = %#v", result)
	}
	if _, err := repository.Get(context.Background(), project.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted project Get error = %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(repository.workspace, ".novelforge", "trash"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("trash entries = %d", len(entries))
	}
}

func TestDeleteRefusesWorkspaceProjectAndSymlinkEscape(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	mustMkdirAll(t, filepath.Join(workspace, "chapters"))
	repository, err := NewRepository(workspace)
	if err != nil {
		t.Fatal(err)
	}
	result, err := repository.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Projects) != 1 {
		t.Fatalf("projects = %#v", result.Projects)
	}
	if _, err := repository.Delete(
		context.Background(),
		result.Projects[0].ID,
		DeleteInput{Confirm: result.Projects[0].ID},
	); !errors.Is(err, ErrWorkspaceRoot) {
		t.Fatalf("workspace delete error = %v", err)
	}

	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation requires privileges")
	}
	outside := t.TempDir()
	link := filepath.Join(workspace, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.resolveImportPath("escape"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("symlink import error = %v", err)
	}
}

func TestImportRejectsTraversalAndWindowsAbsolutePaths(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	for _, input := range []string{
		"../outside",
		"..\\outside",
		"C:\\Users\\author\\book",
		"\\\\server\\share\\book",
		"//server/share/book",
	} {
		if _, err := repository.resolveImportPath(input); !errors.Is(err, ErrUnsafePath) {
			t.Errorf("resolveImportPath(%q) error = %v", input, err)
		}
	}
}

func TestImportedSkeletonIsInitializedWithoutRemovingLegacyFiles(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	root := filepath.Join(repository.workspace, "import-me")
	mustMkdirAll(t, filepath.Join(root, "chapters"))
	mustWrite(t, filepath.Join(root, "chapters", "001.md"), "legacy content")
	mustMkdirAll(t, filepath.Join(root, "meta"))
	mustWrite(t, filepath.Join(root, "meta", "book.json"), `{"title":"Imported"}`)

	project, err := repository.Create(context.Background(), CreateInput{ImportPath: "import-me"})
	if err != nil {
		t.Fatalf("Create import: %v", err)
	}
	if project.Title != "Imported" || project.SourceFormat != "imported-skeleton" {
		t.Fatalf("project = %#v", project)
	}
	if _, err := os.Stat(filepath.Join(root, "chapters", "001.md")); err != nil {
		t.Fatalf("legacy chapter was removed: %v", err)
	}
}

func TestProjectMetadataContainsNoAbsolutePath(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	project, err := repository.Create(context.Background(), CreateInput{Title: "No Paths"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(project)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), repository.workspace) {
		t.Fatalf("absolute workspace leaked: %s", data)
	}
}

func newTestRepository(t *testing.T) *Repository {
	t.Helper()
	repository, err := NewRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository.now = func() time.Time {
		return time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	}
	// The deterministic stream is deliberately not aligned to the 16-byte ID
	// read size, so consecutive generated IDs remain reproducible but distinct.
	repository.random = strings.NewReader(strings.Repeat("0123456789abcdefg", 64))
	return repository
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteRefusesGitRepositoryRoot(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	root := filepath.Join(repository.workspace, "source-checkout")
	mustMkdirAll(t, filepath.Join(root, "chapters"))
	mustMkdirAll(t, filepath.Join(root, ".git"))

	projects, err := repository.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(projects.Projects) != 1 {
		t.Fatalf("projects = %#v", projects.Projects)
	}
	_, err = repository.Delete(
		context.Background(),
		projects.Projects[0].ID,
		DeleteInput{Confirm: projects.Projects[0].ID, Permanent: true},
	)
	if !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("repository delete error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatalf("repository was altered: %v", err)
	}
}
