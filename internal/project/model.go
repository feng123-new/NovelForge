package project

import (
	"errors"
	"fmt"
	"time"
)

const (
	CurrentFormatVersion = 2
	StatusActive         = "active"
	StatusArchived       = "archived"
)

var (
	ErrNotFound      = errors.New("project not found")
	ErrConflict      = errors.New("project conflict")
	ErrValidation    = errors.New("project validation failed")
	ErrUnsafePath    = errors.New("unsafe project path")
	ErrConfirmation  = errors.New("project confirmation failed")
	ErrWorkspaceRoot = errors.New("workspace root cannot be used for this operation")
)

// Error attaches a stable machine code without exposing host paths.
type Error struct {
	Code    string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "project operation failed"
}

func (e *Error) Unwrap() error { return e.Err }

func newError(code, message string, cause error) error {
	return &Error{Code: code, Message: message, Err: cause}
}

// Summary is the stable collection representation. Path is always
// workspace-relative and exists only for legacy compatibility.
type Summary struct {
	ID                string    `json:"id"`
	Title             string    `json:"title"`
	Path              string    `json:"path,omitempty"`
	Status            string    `json:"status"`
	Archived          bool      `json:"archived"`
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

// Project is the detail representation.
type Project struct {
	Summary
	Synopsis        string     `json:"synopsis,omitempty"`
	Genre           string     `json:"genre,omitempty"`
	Language        string     `json:"language,omitempty"`
	TargetWords     int        `json:"target_words,omitempty"`
	WordsPerChapter int        `json:"words_per_chapter,omitempty"`
	CreatedAt       time.Time  `json:"created_at,omitempty"`
	ArchivedAt      *time.Time `json:"archived_at,omitempty"`
	SourceFormat    string     `json:"source_format,omitempty"`
}

// Metadata is persisted at .novelforge/project.json.
type Metadata struct {
	FormatVersion   int        `json:"format_version"`
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Synopsis        string     `json:"synopsis,omitempty"`
	Genre           string     `json:"genre,omitempty"`
	Language        string     `json:"language,omitempty"`
	TargetWords     int        `json:"target_words,omitempty"`
	TargetChapters  int        `json:"target_chapters,omitempty"`
	WordsPerChapter int        `json:"words_per_chapter,omitempty"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	ArchivedAt      *time.Time `json:"archived_at,omitempty"`
	SourceFormat    string     `json:"source_format,omitempty"`
}

// CreateInput creates a new project or imports an existing skeleton when
// ImportPath is set. ImportPath must be workspace-relative.
type CreateInput struct {
	Title           string `json:"title"`
	Slug            string `json:"slug,omitempty"`
	Synopsis        string `json:"synopsis,omitempty"`
	Genre           string `json:"genre,omitempty"`
	Language        string `json:"language,omitempty"`
	TargetWords     int    `json:"target_words,omitempty"`
	TargetChapters  int    `json:"target_chapters,omitempty"`
	WordsPerChapter int    `json:"words_per_chapter,omitempty"`
	ImportPath      string `json:"import_path,omitempty"`
}

// UpdateInput changes mutable project metadata.
type UpdateInput struct {
	Title           *string `json:"title,omitempty"`
	Synopsis        *string `json:"synopsis,omitempty"`
	Genre           *string `json:"genre,omitempty"`
	Language        *string `json:"language,omitempty"`
	TargetWords     *int    `json:"target_words,omitempty"`
	TargetChapters  *int    `json:"target_chapters,omitempty"`
	WordsPerChapter *int    `json:"words_per_chapter,omitempty"`
}

// DuplicateInput controls the copy name and directory label.
type DuplicateInput struct {
	Title string `json:"title,omitempty"`
	Slug  string `json:"slug,omitempty"`
}

// DeleteInput requires an exact project ID or title confirmation.
type DeleteInput struct {
	Confirm   string `json:"confirm"`
	Permanent bool   `json:"permanent,omitempty"`
}

// DeleteResult deliberately does not expose filesystem paths.
type DeleteResult struct {
	ID        string `json:"id"`
	Deleted   bool   `json:"deleted"`
	Permanent bool   `json:"permanent"`
}

// ListOptions provides stable offset pagination.
type ListOptions struct {
	Limit    int
	Offset   int
	Archived *bool
	Query    string
}

// ListResult contains a stable page and total count.
type ListResult struct {
	Projects   []Summary `json:"projects"`
	Total      int       `json:"total"`
	Limit      int       `json:"limit"`
	Offset     int       `json:"offset"`
	NextOffset *int      `json:"next_offset,omitempty"`
}

func validateNonNegative(name string, value int) error {
	if value < 0 {
		return newError(
			"PROJECT_VALIDATION_FAILED",
			fmt.Sprintf("%s must not be negative", name),
			ErrValidation,
		)
	}
	return nil
}
