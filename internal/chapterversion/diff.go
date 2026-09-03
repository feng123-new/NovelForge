package chapterversion

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/voocel/ainovel-cli/internal/domain"
)

const (
	maxDiffBytes     = 2 << 20
	maxDiffLines     = 20000
	maxDiffPage      = 500
	diffLookahead    = 64
	maxDiffWorkUnits = 2_000_000
)

type diffAtom struct {
	kind    string
	oldLine int
	newLine int
	oldText string
	newText string
}

// Diff computes a deterministic bounded line diff. The matcher never searches
// farther than diffLookahead lines and has a hard work-unit ceiling, so a large
// or adversarial chapter cannot trigger an unbounded quadratic scan.
func (s *Store) Diff(ctx context.Context, chapter int, fromID, toID string, mode DiffMode, cursor string, limit int) (DiffResult, error) {
	if mode == "" {
		mode = DiffInline
	}
	if mode != DiffInline && mode != DiffSideBySide {
		return DiffResult{}, newError(CodeValidation, "diff mode must be inline or side_by_side", false, nil)
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > maxDiffPage {
		limit = maxDiffPage
	}
	offset := 0
	if strings.TrimSpace(cursor) != "" {
		parsed, err := strconv.Atoi(cursor)
		if err != nil || parsed < 0 {
			return DiffResult{}, newError(CodeDiffCursorInvalid, "diff cursor is invalid", false, nil)
		}
		offset = parsed
	}
	from, err := s.Get(ctx, chapter, fromID, true)
	if err != nil {
		return DiffResult{}, err
	}
	to, err := s.Get(ctx, chapter, toID, true)
	if err != nil {
		return DiffResult{}, err
	}
	left := domain.NormalizeChapterContent(from.Content)
	right := domain.NormalizeChapterContent(to.Content)
	if len(left) > maxDiffBytes || len(right) > maxDiffBytes {
		return DiffResult{}, newError(CodeDiffTooLarge, "chapter diff exceeds the bounded byte limit", false, nil)
	}
	oldLines := strings.Split(left, "\n")
	newLines := strings.Split(right, "\n")
	if len(oldLines) > maxDiffLines || len(newLines) > maxDiffLines {
		return DiffResult{}, newError(CodeDiffTooLarge, "chapter diff exceeds the bounded line limit", false, nil)
	}
	atoms, err := boundedDiff(ctx, oldLines, newLines)
	if err != nil {
		return DiffResult{}, err
	}
	if mode == DiffSideBySide {
		atoms = pairReplacements(atoms)
	}
	result := DiffResult{FromVersion: from.ID, ToVersion: to.ID, FromSHA: from.ContentSHA, ToSHA: to.ContentSHA, Mode: mode, Hunks: []DiffHunk{}}
	for _, atom := range atoms {
		switch atom.kind {
		case "add":
			result.Additions++
		case "delete":
			result.Deletions++
		case "replace":
			result.Additions++
			result.Deletions++
		default:
			result.Unchanged++
		}
	}
	if offset > len(atoms) {
		return DiffResult{}, newError(CodeDiffCursorInvalid, "diff cursor is outside the result", false, nil)
	}
	end := offset + limit
	if end > len(atoms) {
		end = len(atoms)
	}
	page := atoms[offset:end]
	if len(page) > 0 {
		hunk := DiffHunk{Lines: make([]DiffLine, 0, len(page))}
		for _, atom := range page {
			line := DiffLine{Kind: atom.kind, OldText: atom.oldText, NewText: atom.newText}
			if atom.oldLine > 0 {
				value := atom.oldLine
				line.OldLine = &value
				if hunk.OldStart == 0 {
					hunk.OldStart = value
				}
				hunk.OldLines++
			}
			if atom.newLine > 0 {
				value := atom.newLine
				line.NewLine = &value
				if hunk.NewStart == 0 {
					hunk.NewStart = value
				}
				hunk.NewLines++
			}
			switch atom.kind {
			case "add":
				hunk.Additions++
			case "delete":
				hunk.Deletions++
			case "replace":
				hunk.Additions++
				hunk.Deletions++
			default:
				hunk.Unchanged++
			}
			hunk.Lines = append(hunk.Lines, line)
		}
		result.Hunks = append(result.Hunks, hunk)
	}
	if end < len(atoms) {
		result.Truncated = true
		result.NextCursor = strconv.Itoa(end)
	}
	return result, nil
}

func boundedDiff(ctx context.Context, oldLines, newLines []string) ([]diffAtom, error) {
	atoms := make([]diffAtom, 0, len(oldLines)+len(newLines))
	i, j, work := 0, 0, 0
	for i < len(oldLines) || j < len(newLines) {
		if work%256 == 0 {
			select {
			case <-ctx.Done():
				return nil, newError(CodeDiffTooLarge, "chapter diff exceeded its execution boundary", true, ctx.Err())
			default:
			}
		}
		if work > maxDiffWorkUnits {
			return nil, newError(CodeDiffTooLarge, "chapter diff exceeded its deterministic work limit", false, nil)
		}
		if i < len(oldLines) && j < len(newLines) && oldLines[i] == newLines[j] {
			atoms = append(atoms, diffAtom{kind: "equal", oldLine: i + 1, newLine: j + 1, oldText: oldLines[i], newText: newLines[j]})
			i++
			j++
			work++
			continue
		}
		oldMatch, newMatch := -1, -1
		maxOld := minInt(len(oldLines), i+diffLookahead+1)
		maxNew := minInt(len(newLines), j+diffLookahead+1)
		bestCost := int(^uint(0) >> 1)
		for oi := i; oi < maxOld; oi++ {
			for nj := j; nj < maxNew; nj++ {
				work++
				if work > maxDiffWorkUnits {
					return nil, newError(CodeDiffTooLarge, "chapter diff exceeded its deterministic work limit", false, nil)
				}
				if oldLines[oi] != newLines[nj] {
					continue
				}
				cost := (oi - i) + (nj - j)
				if cost < bestCost || cost == bestCost && (oi < oldMatch || oldMatch < 0) {
					oldMatch, newMatch, bestCost = oi, nj, cost
				}
			}
		}
		if oldMatch < 0 {
			if i < len(oldLines) {
				atoms = append(atoms, diffAtom{kind: "delete", oldLine: i + 1, oldText: oldLines[i]})
				i++
			}
			if j < len(newLines) {
				atoms = append(atoms, diffAtom{kind: "add", newLine: j + 1, newText: newLines[j]})
				j++
			}
			continue
		}
		for i < oldMatch {
			atoms = append(atoms, diffAtom{kind: "delete", oldLine: i + 1, oldText: oldLines[i]})
			i++
		}
		for j < newMatch {
			atoms = append(atoms, diffAtom{kind: "add", newLine: j + 1, newText: newLines[j]})
			j++
		}
	}
	return atoms, nil
}

func pairReplacements(atoms []diffAtom) []diffAtom {
	out := make([]diffAtom, 0, len(atoms))
	for i := 0; i < len(atoms); i++ {
		current := atoms[i]
		if current.kind == "delete" && i+1 < len(atoms) && atoms[i+1].kind == "add" {
			next := atoms[i+1]
			out = append(out, diffAtom{kind: "replace", oldLine: current.oldLine, newLine: next.newLine, oldText: current.oldText, newText: next.newText})
			i++
			continue
		}
		out = append(out, current)
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func DiffLimits() map[string]int {
	return map[string]int{
		"max_bytes_per_version": maxDiffBytes,
		"max_lines_per_version": maxDiffLines,
		"max_page_lines":        maxDiffPage,
		"lookahead_lines":       diffLookahead,
		"max_work_units":        maxDiffWorkUnits,
	}
}

func validateDiffIdentity(fromID, toID string) error {
	if strings.TrimSpace(fromID) == "" || strings.TrimSpace(toID) == "" {
		return fmt.Errorf("from_version and to_version are required")
	}
	return nil
}
