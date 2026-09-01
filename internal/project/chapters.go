package project

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const maxChapterInspectionBytes = 1 << 20

var chapterNumberPattern = regexp.MustCompile(`\d+`)

// ChapterSummary is the bounded collection representation used by the Web workspace.
type ChapterSummary struct {
	Chapter        int       `json:"chapter"`
	Title          string    `json:"title"`
	Status         string    `json:"status"`
	CharacterCount int       `json:"character_count"`
	UpdatedAt      time.Time `json:"updated_at"`
	Truncated      bool      `json:"truncated,omitempty"`
}

// ChapterListResult is a deterministic, bounded chapter page.
type ChapterListResult struct {
	Chapters   []ChapterSummary `json:"chapters"`
	Total      int              `json:"total"`
	Limit      int              `json:"limit"`
	Offset     int              `json:"offset"`
	NextOffset *int             `json:"next_offset,omitempty"`
}

// ListChapters scans only one project's chapters directory and never exposes its path.
func (r *Repository) ListChapters(
	_ context.Context,
	projectID string,
	limit int,
	offset int,
) (ChapterListResult, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		return ChapterListResult{}, newError(
			"PROJECT_VALIDATION_FAILED",
			"offset must not be negative",
			ErrValidation,
		)
	}
	candidate, err := r.find(projectID)
	if err != nil {
		return ChapterListResult{}, err
	}
	chapterRoot := filepath.Join(candidate.Root, "chapters")
	entries, err := os.ReadDir(chapterRoot)
	if errors.Is(err, os.ErrNotExist) {
		return ChapterListResult{Chapters: []ChapterSummary{}, Limit: limit, Offset: offset}, nil
	}
	if err != nil {
		return ChapterListResult{}, newError(
			"CHAPTER_LIST_FAILED",
			"chapters could not be listed",
			err,
		)
	}

	chapters := make([]ChapterSummary, 0, len(entries))
	for _, item := range entries {
		if item.IsDir() || item.Type()&os.ModeSymlink != 0 {
			continue
		}
		extension := strings.ToLower(filepath.Ext(item.Name()))
		if extension != ".md" && extension != ".markdown" && extension != ".txt" {
			continue
		}
		number, ok := chapterNumber(item.Name())
		if !ok {
			continue
		}
		summary, err := inspectChapter(filepath.Join(chapterRoot, item.Name()), number, item.Name())
		if err != nil {
			return ChapterListResult{}, newError(
				"CHAPTER_READ_FAILED",
				"chapter metadata could not be read",
				err,
			)
		}
		chapters = append(chapters, summary)
	}
	sort.Slice(chapters, func(i, j int) bool {
		if chapters[i].Chapter == chapters[j].Chapter {
			return chapters[i].Title < chapters[j].Title
		}
		return chapters[i].Chapter < chapters[j].Chapter
	})
	result := ChapterListResult{
		Chapters: []ChapterSummary{},
		Total:    len(chapters),
		Limit:    limit,
		Offset:   offset,
	}
	if offset >= len(chapters) {
		return result, nil
	}
	end := offset + limit
	if end > len(chapters) {
		end = len(chapters)
	}
	result.Chapters = append(result.Chapters, chapters[offset:end]...)
	if end < len(chapters) {
		next := end
		result.NextOffset = &next
	}
	return result, nil
}

func chapterNumber(name string) (int, bool) {
	match := chapterNumberPattern.FindString(name)
	if match == "" {
		return 0, false
	}
	var number int
	if _, err := fmt.Sscanf(match, "%d", &number); err != nil || number <= 0 {
		return 0, false
	}
	return number, true
}

func inspectChapter(path string, chapter int, filename string) (ChapterSummary, error) {
	file, err := os.Open(path)
	if err != nil {
		return ChapterSummary{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return ChapterSummary{}, err
	}
	truncated := info.Size() > maxChapterInspectionBytes
	content, err := io.ReadAll(io.LimitReader(file, maxChapterInspectionBytes))
	if err != nil {
		return ChapterSummary{}, err
	}
	title := chapterTitle(content, filename, chapter)
	return ChapterSummary{
		Chapter:        chapter,
		Title:          title,
		Status:         "available",
		CharacterCount: countVisibleCharacters(content),
		UpdatedAt:      info.ModTime().UTC(),
		Truncated:      truncated,
	}, nil
}

func chapterTitle(content []byte, filename string, chapter int) string {
	firstLine := strings.TrimSpace(strings.SplitN(string(content), "\n", 2)[0])
	firstLine = strings.TrimSpace(strings.TrimLeft(firstLine, "#"))
	if firstLine != "" && utf8.RuneCountInString(firstLine) <= 200 {
		return firstLine
	}
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	base = strings.TrimSpace(strings.NewReplacer("_", " ", "-", " ").Replace(base))
	if base == "" {
		return fmt.Sprintf("Chapter %d", chapter)
	}
	return base
}

func countVisibleCharacters(content []byte) int {
	count := 0
	for _, value := range string(content) {
		if !unicode.IsSpace(value) {
			count++
		}
	}
	return count
}
