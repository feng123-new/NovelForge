package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ProjectSummary struct {
	ID                string    `json:"id"`
	Title             string    `json:"title"`
	Path              string    `json:"path"`
	Phase             string    `json:"phase,omitempty"`
	CurrentChapter    int       `json:"current_chapter"`
	CompletedChapters int       `json:"completed_chapters"`
	TotalChapters     int       `json:"total_chapters"`
	TotalWords        int       `json:"total_words"`
	CurrentVolume     int       `json:"current_volume,omitempty"`
	CurrentArc        int       `json:"current_arc,omitempty"`
	FormatVersion     int       `json:"format_version,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
	Warnings          []string  `json:"warnings,omitempty"`
}

type ProjectDetail struct {
	ProjectSummary
	Synopsis string `json:"synopsis,omitempty"`
}

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

type formatMetadata struct {
	Version int `json:"version"`
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	projects, err := discoverProjects(s.cfg.Workspace)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (s *Server) handleProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/projects/")
	if id == "" || strings.Contains(id, "/") {
		writeAPIError(w, http.StatusNotFound, "project not found")
		return
	}
	projects, err := discoverProjectDetails(s.cfg.Workspace)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, project := range projects {
		if project.ID == id {
			writeJSON(w, http.StatusOK, project)
			return
		}
	}
	writeAPIError(w, http.StatusNotFound, "project not found")
}

func discoverProjects(workspace string) ([]ProjectSummary, error) {
	details, err := discoverProjectDetails(workspace)
	if err != nil {
		return nil, err
	}
	projects := make([]ProjectSummary, 0, len(details))
	for _, detail := range details {
		projects = append(projects, detail.ProjectSummary)
	}
	return projects, nil
}

func discoverProjectDetails(workspace string) ([]ProjectDetail, error) {
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return nil, fmt.Errorf("read workspace: %w", err)
	}

	candidateDirs := make([]string, 0, len(entries)+1)
	if looksLikeProject(workspace) {
		candidateDirs = append(candidateDirs, workspace)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(workspace, entry.Name())
		if looksLikeProject(dir) {
			candidateDirs = append(candidateDirs, dir)
		}
	}

	projects := make([]ProjectDetail, 0, len(candidateDirs))
	for _, dir := range candidateDirs {
		project, err := readProject(workspace, dir)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	sort.Slice(projects, func(i, j int) bool {
		left := strings.ToLower(projects[i].Title)
		right := strings.ToLower(projects[j].Title)
		if left == right {
			return projects[i].Path < projects[j].Path
		}
		return left < right
	})
	return projects, nil
}

func looksLikeProject(dir string) bool {
	markers := []string{
		filepath.Join(dir, "meta", "book.json"),
		filepath.Join(dir, "meta", "progress.json"),
		filepath.Join(dir, "outline.json"),
		filepath.Join(dir, "chapters"),
	}
	for _, marker := range markers {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	return false
}

func readProject(workspace, dir string) (ProjectDetail, error) {
	relative, err := filepath.Rel(workspace, dir)
	if err != nil {
		return ProjectDetail{}, fmt.Errorf("resolve project path: %w", err)
	}
	relative = filepath.ToSlash(relative)
	if relative == "." {
		relative = "."
	}

	project := ProjectDetail{
		ProjectSummary: ProjectSummary{
			ID:        stableProjectID(relative),
			Title:     filepath.Base(dir),
			Path:      relative,
			UpdatedAt: latestModTime(dir),
		},
	}

	var book legacyBook
	if err := readOptionalJSON(filepath.Join(dir, "meta", "book.json"), &book); err != nil {
		project.Warnings = append(project.Warnings, "book metadata: "+err.Error())
	} else {
		if strings.TrimSpace(book.Title) != "" {
			project.Title = strings.TrimSpace(book.Title)
		}
		project.Synopsis = strings.TrimSpace(book.Synopsis)
	}

	var progress legacyProgress
	if err := readOptionalJSON(filepath.Join(dir, "meta", "progress.json"), &progress); err != nil {
		project.Warnings = append(project.Warnings, "progress metadata: "+err.Error())
	} else {
		project.Phase = progress.Phase
		project.CurrentChapter = progress.CurrentChapter
		if progress.InProgressChapter > project.CurrentChapter {
			project.CurrentChapter = progress.InProgressChapter
		}
		project.CompletedChapters = len(progress.CompletedChapters)
		project.TotalChapters = progress.TotalChapters
		project.TotalWords = progress.TotalWordCount
		project.CurrentVolume = progress.CurrentVolume
		project.CurrentArc = progress.CurrentArc
	}

	var format formatMetadata
	if err := readOptionalJSON(filepath.Join(dir, "meta", "format.json"), &format); err != nil {
		project.Warnings = append(project.Warnings, "format metadata: "+err.Error())
	} else {
		project.FormatVersion = format.Version
	}
	if project.FormatVersion == 0 {
		project.FormatVersion = 1
	}
	return project, nil
}

func readOptionalJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return err
	}
	return nil
}

func stableProjectID(relative string) string {
	digest := sha256.Sum256([]byte(filepath.ToSlash(relative)))
	return hex.EncodeToString(digest[:8])
}

func latestModTime(dir string) time.Time {
	paths := []string{
		dir,
		filepath.Join(dir, "meta", "book.json"),
		filepath.Join(dir, "meta", "progress.json"),
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
