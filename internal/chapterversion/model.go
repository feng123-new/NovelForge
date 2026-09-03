package chapterversion

import (
	"encoding/json"
	"time"
)

type VersionType string

type AuthorType string

const (
	TypeDraft          VersionType = "draft"
	TypeContinuityFix  VersionType = "continuity_fix"
	TypeEditorRevision VersionType = "editor_revision"
	TypeHumanRevision  VersionType = "human_revision"
	TypeFinal          VersionType = "final"
	TypeRejected       VersionType = "rejected"

	AuthorWriter    AuthorType = "writer"
	AuthorLibrarian AuthorType = "librarian"
	AuthorEditor    AuthorType = "editor"
	AuthorHuman     AuthorType = "human"
	AuthorRestore   AuthorType = "restore"
	AuthorSystem    AuthorType = "system"

	AuthorityGeneratedFinal = "generated_final"
	AuthorityHumanFinal     = "human_final"
)

type Version struct {
	ID              string          `json:"id"`
	ProjectID       string          `json:"project_id"`
	Chapter         int             `json:"chapter"`
	VersionNumber   int             `json:"version_number"`
	Type            VersionType     `json:"type"`
	Status          string          `json:"status"`
	Content         string          `json:"content,omitempty"`
	ContentSHA      string          `json:"content_sha"`
	ParentVersionID string          `json:"parent_version,omitempty"`
	AuthorType      AuthorType      `json:"author_type"`
	Provider        string          `json:"provider,omitempty"`
	Model           string          `json:"model,omitempty"`
	PromptHash      string          `json:"prompt_hash,omitempty"`
	Review          json.RawMessage `json:"review,omitempty"`
	Continuity      json.RawMessage `json:"continuity,omitempty"`
	Provenance      json.RawMessage `json:"provenance,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	Accepted        bool            `json:"accepted"`
	Rejected        bool            `json:"rejected"`
	ActiveFinal     bool            `json:"active_final"`
	Authority       string          `json:"authority,omitempty"`
	RejectionReason string          `json:"rejection_reason,omitempty"`
}

type CreateInput struct {
	Content         string          `json:"content"`
	Type            VersionType     `json:"type,omitempty"`
	ParentVersionID string          `json:"parent_version,omitempty"`
	AuthorType      AuthorType      `json:"author_type,omitempty"`
	Provider        string          `json:"provider,omitempty"`
	Model           string          `json:"model,omitempty"`
	PromptHash      string          `json:"prompt_hash,omitempty"`
	Review          json.RawMessage `json:"review,omitempty"`
	Continuity      json.RawMessage `json:"continuity,omitempty"`
	Provenance      json.RawMessage `json:"provenance,omitempty"`
}

type ListOptions struct {
	Limit          int
	Offset         int
	Status         string
	Type           VersionType
	AuthorType     AuthorType
	IncludeContent bool
}

type ListResult struct {
	Versions   []Version `json:"versions"`
	Total      int       `json:"total"`
	Limit      int       `json:"limit"`
	Offset     int       `json:"offset"`
	NextOffset *int      `json:"next_offset,omitempty"`
}

type Event struct {
	Sequence  int64           `json:"sequence"`
	ID        string          `json:"id"`
	ProjectID string          `json:"project_id"`
	Chapter   int             `json:"chapter"`
	VersionID string          `json:"version_id"`
	Type      string          `json:"type"`
	Reason    string          `json:"reason,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type ChapterView struct {
	ProjectID    string     `json:"project_id"`
	Chapter      int        `json:"chapter"`
	ActiveFinal  *Version   `json:"active_final,omitempty"`
	Latest       *Version   `json:"latest,omitempty"`
	VersionCount int        `json:"version_count"`
	Sync         SyncStatus `json:"sync"`
	DerivedState string     `json:"derived_state"`
}

type SyncStatus struct {
	ProjectID       string    `json:"project_id"`
	Chapter         int       `json:"chapter"`
	ActiveVersionID string    `json:"active_version_id,omitempty"`
	ExpectedSHA     string    `json:"expected_sha,omitempty"`
	ObservedSHA     string    `json:"observed_sha,omitempty"`
	ObservedAt      time.Time `json:"observed_at,omitempty"`
	SyncRequired    bool      `json:"sync_required"`
}

type DiffMode string

const (
	DiffInline     DiffMode = "inline"
	DiffSideBySide DiffMode = "side_by_side"
)

type DiffLine struct {
	Kind    string `json:"kind"`
	OldLine *int   `json:"old_line,omitempty"`
	NewLine *int   `json:"new_line,omitempty"`
	OldText string `json:"old_text,omitempty"`
	NewText string `json:"new_text,omitempty"`
}

type DiffHunk struct {
	OldStart  int        `json:"old_start"`
	OldLines  int        `json:"old_lines"`
	NewStart  int        `json:"new_start"`
	NewLines  int        `json:"new_lines"`
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
	Unchanged int        `json:"unchanged"`
	Lines     []DiffLine `json:"lines"`
}

type DiffResult struct {
	FromVersion string     `json:"from_version"`
	ToVersion   string     `json:"to_version"`
	FromSHA     string     `json:"from_sha"`
	ToSHA       string     `json:"to_sha"`
	Mode        DiffMode   `json:"mode"`
	Hunks       []DiffHunk `json:"hunks"`
	Additions   int        `json:"additions"`
	Deletions   int        `json:"deletions"`
	Unchanged   int        `json:"unchanged"`
	Truncated   bool       `json:"truncated"`
	NextCursor  string     `json:"next_cursor,omitempty"`
}

type AcceptResult struct {
	Version Version `json:"version"`
}

type RestoreResult struct {
	Version Version `json:"version"`
}

type FinalizeResult struct {
	Version       Version `json:"version"`
	ActiveFinal   Version `json:"active_final"`
	OperationID   string  `json:"operation_id"`
	TruthEvents   int     `json:"truth_events"`
	RebuildStatus string  `json:"rebuild_status"`
}

type Rebuild struct {
	OperationID     string          `json:"operation_id"`
	ProjectID       string          `json:"project_id"`
	BoundaryChapter int             `json:"boundary_chapter"`
	SourceVersion   string          `json:"source_version"`
	State           string          `json:"status"`
	CurrentStep     string          `json:"current_step"`
	Affected        json.RawMessage `json:"affected"`
	BeforeDigest    string          `json:"before_digest"`
	AfterDigest     string          `json:"after_digest"`
	StartedAt       time.Time       `json:"started_at"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty"`
	ErrorCode       string          `json:"error_code,omitempty"`
}

type PlanImpact struct {
	ID                 string    `json:"id"`
	PlanID             string    `json:"plan_id"`
	Chapter            int       `json:"chapter"`
	Severity           string    `json:"severity"`
	AffectedFact       string    `json:"affected_fact"`
	PreviousAssumption string    `json:"previous_assumption"`
	NewTruth           string    `json:"new_truth"`
	ActionRequired     string    `json:"action_required"`
	Reason             string    `json:"reason"`
	SourceVersion      string    `json:"source_version"`
	CreatedAt          time.Time `json:"created_at"`
}

type SyncResult struct {
	Version      Version         `json:"version"`
	Proposal     json.RawMessage `json:"proposal,omitempty"`
	Continuity   json.RawMessage `json:"continuity,omitempty"`
	Review       json.RawMessage `json:"review,omitempty"`
	Conflicts    int             `json:"conflicts"`
	SyncRequired bool            `json:"sync_required"`
}
