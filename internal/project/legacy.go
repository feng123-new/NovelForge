package project

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type legacyBook struct {
	Title    string `json:"title"`
	Synopsis string `json:"synopsis"`
}

type legacyProgress struct {
	Phase             string `json:"phase"`
	CurrentChapter    int    `json:"current_chapter"`
	InProgressChapter int    `json:"in_progress_chapter"`
	TotalChapters     int    `json:"total_chapters"`
	TotalWordCount    int    `json:"total_word_count"`
	CurrentVolume     int    `json:"current_volume"`
	CurrentArc        int    `json:"current_arc"`
	CompletedChapters []int  `json:"completed_chapters"`
}

type legacyFormat struct {
	Version int `json:"version"`
}

func looksLikeProject(root string) bool {
	markers := []string{
		filepath.Join(root, projectMetadataRelative),
		filepath.Join(root, "meta", "book.json"),
		filepath.Join(root, "meta", "progress.json"),
		filepath.Join(root, "outline.json"),
		filepath.Join(root, "chapters"),
	}
	for _, marker := range markers {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	return false
}

func readLegacyProject(workspace, root string) (Project, error) {
	relative, err := filepath.Rel(workspace, root)
	if err != nil {
		return Project{}, newError(
			"PROJECT_DISCOVERY_FAILED",
			"project location could not be resolved",
			err,
		)
	}
	relative = filepath.ToSlash(relative)
	if relative == "." {
		relative = "."
	}
	project := Project{
		Summary: Summary{
			ID:            stableProjectID(relative),
			Title:         filepath.Base(root),
			Path:          relative,
			Status:        StatusActive,
			FormatVersion: 1,
			UpdatedAt:     latestModTime(root),
		},
		SourceFormat: "ainovel",
	}
	var book legacyBook
	if found, err := readOptionalJSON(filepath.Join(root, "meta", "book.json"), &book); err != nil {
		project.Warnings = append(project.Warnings, "book metadata is invalid")
	} else if found {
		if book.Title != "" {
			project.Title = book.Title
		}
		project.Synopsis = book.Synopsis
	}
	applyLegacyProgress(root, &project)

	var format legacyFormat
	if found, err := readOptionalJSON(filepath.Join(root, "meta", "format.json"), &format); err != nil {
		project.Warnings = append(project.Warnings, "format metadata is invalid")
	} else if found && format.Version > 0 {
		project.FormatVersion = format.Version
	}
	return project, nil
}

func applyLegacyProgress(root string, project *Project) {
	var progress legacyProgress
	found, err := readOptionalJSON(filepath.Join(root, "meta", "progress.json"), &progress)
	if err != nil {
		project.Warnings = append(project.Warnings, "progress metadata is invalid")
		return
	}
	if !found {
		return
	}
	project.Phase = progress.Phase
	project.CurrentChapter = progress.CurrentChapter
	if progress.InProgressChapter > project.CurrentChapter {
		project.CurrentChapter = progress.InProgressChapter
	}
	project.CompletedChapters = len(progress.CompletedChapters)
	if progress.TotalChapters > 0 {
		project.TotalChapters = progress.TotalChapters
	}
	project.TotalWords = progress.TotalWordCount
	project.CurrentVolume = progress.CurrentVolume
	project.CurrentArc = progress.CurrentArc
}

func readOptionalJSON(path string, target any) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return true, err
	}
	return true, nil
}

func latestModTime(root string) time.Time {
	paths := []string{
		root,
		filepath.Join(root, projectMetadataRelative),
		filepath.Join(root, "meta", "book.json"),
		filepath.Join(root, "meta", "progress.json"),
	}
	var latest time.Time
	for _, path := range paths {
		info, err := os.Stat(path)
		if err == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	return latest.UTC()
}
