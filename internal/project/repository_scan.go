package project

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type entry struct {
	Root     string
	Project  Project
	Metadata Metadata
}

func (r *Repository) scan() ([]entry, error) {
	directories := make([]string, 0)
	if looksLikeProject(r.workspace) {
		directories = append(directories, r.workspace)
	}
	items, err := os.ReadDir(r.workspace)
	if err != nil {
		return nil, newError(
			"PROJECT_DISCOVERY_FAILED",
			"workspace projects could not be listed",
			err,
		)
	}
	for _, item := range items {
		if !item.IsDir() || item.Name() == ".novelforge" {
			continue
		}
		root := filepath.Join(r.workspace, item.Name())
		if looksLikeProject(root) {
			directories = append(directories, root)
		}
	}
	entries := make([]entry, 0, len(directories))
	for _, root := range directories {
		project, err := r.read(root)
		if err != nil {
			return nil, err
		}
		entries = append(entries, project)
	}
	sort.Slice(entries, func(i, j int) bool {
		left := strings.ToLower(entries[i].Project.Title)
		right := strings.ToLower(entries[j].Project.Title)
		if left == right {
			return entries[i].Project.ID < entries[j].Project.ID
		}
		return left < right
	})
	return entries, nil
}

func (r *Repository) find(id string) (entry, error) {
	id = strings.TrimSpace(id)
	if id == "" || strings.ContainsAny(id, `/\`) {
		return entry{}, newError("PROJECT_NOT_FOUND", "project not found", ErrNotFound)
	}
	entries, err := r.scan()
	if err != nil {
		return entry{}, err
	}
	for _, candidate := range entries {
		if candidate.Project.ID == id {
			return candidate, nil
		}
	}
	return entry{}, newError("PROJECT_NOT_FOUND", "project not found", ErrNotFound)
}

func (r *Repository) read(root string) (entry, error) {
	relative, err := filepath.Rel(r.workspace, root)
	if err != nil {
		return entry{}, newError(
			"PROJECT_DISCOVERY_FAILED",
			"project location could not be resolved",
			err,
		)
	}
	relative = filepath.ToSlash(relative)
	if relative == "." {
		relative = "."
	}

	var metadata Metadata
	metadataPath := filepath.Join(root, projectMetadataRelative)
	data, err := os.ReadFile(metadataPath)
	if err == nil {
		if err := json.Unmarshal(data, &metadata); err != nil {
			return entry{}, newError(
				"PROJECT_METADATA_INVALID",
				"project metadata is invalid",
				err,
			)
		}
		if metadata.ID == "" || metadata.Title == "" {
			return entry{}, newError(
				"PROJECT_METADATA_INVALID",
				"project metadata is incomplete",
				ErrValidation,
			)
		}
		status := metadata.Status
		if status == "" {
			status = StatusActive
		}
		project := Project{
			Summary: Summary{
				ID:            metadata.ID,
				Title:         metadata.Title,
				Path:          relative,
				Status:        status,
				Archived:      status == StatusArchived,
				TotalChapters: metadata.TargetChapters,
				FormatVersion: metadata.FormatVersion,
				UpdatedAt:     metadata.UpdatedAt.UTC(),
			},
			Synopsis:        metadata.Synopsis,
			Genre:           metadata.Genre,
			Language:        metadata.Language,
			TargetWords:     metadata.TargetWords,
			WordsPerChapter: metadata.WordsPerChapter,
			CreatedAt:       metadata.CreatedAt.UTC(),
			ArchivedAt:      metadata.ArchivedAt,
			SourceFormat:    metadata.SourceFormat,
		}
		applyLegacyProgress(root, &project)
		if project.UpdatedAt.IsZero() {
			project.UpdatedAt = latestModTime(root)
		}
		return entry{Root: root, Project: project, Metadata: metadata}, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return entry{}, newError(
			"PROJECT_DISCOVERY_FAILED",
			"project metadata could not be read",
			err,
		)
	}
	legacy, err := readLegacyProject(r.workspace, root)
	if err != nil {
		return entry{}, err
	}
	return entry{Root: root, Project: legacy}, nil
}
