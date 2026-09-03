package chapterversion

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
	"github.com/voocel/ainovel-cli/internal/domain"
)

const maxListLimit = 100

type Store struct {
	db        *sql.DB
	root      string
	projectID string
	now       func() time.Time
}

type operation struct {
	Key       string
	Operation string
	ProjectID string
	Chapter   int
	VersionID string
	Hash      string
	Status    string
	Result    json.RawMessage
}

func OpenExisting(path, root, projectID string) (*Store, error) {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(root) == "" || strings.TrimSpace(projectID) == "" {
		return nil, newError(CodeValidation, "chapter version store identity is incomplete", false, nil)
	}
	db, err := migrate.Open(path, 5*time.Second)
	if err != nil {
		return nil, newError(CodeStorage, "chapter version database could not be opened", true, err)
	}
	return &Store{db: db, root: root, projectID: projectID, now: time.Now}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Database() *sql.DB { return s.db }
func (s *Store) Root() string      { return s.root }
func (s *Store) ProjectID() string { return s.projectID }

func normalizeJSON(value json.RawMessage) (json.RawMessage, error) {
	if len(value) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if !json.Valid(value) {
		return nil, newError(CodeValidation, "chapter version JSON metadata is invalid", false, nil)
	}
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return nil, newError(CodeValidation, "chapter version JSON metadata is invalid", false, err)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return nil, newError(CodeValidation, "chapter version JSON metadata is invalid", false, err)
	}
	return encoded, nil
}

func hashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func newOpaqueID(prefix string) (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(buffer), nil
}

func validType(value VersionType) bool {
	switch value {
	case TypeDraft, TypeContinuityFix, TypeEditorRevision, TypeHumanRevision, TypeFinal, TypeRejected:
		return true
	default:
		return false
	}
}

func validAuthor(value AuthorType) bool {
	switch value {
	case AuthorWriter, AuthorLibrarian, AuthorEditor, AuthorHuman, AuthorRestore, AuthorSystem:
		return true
	default:
		return false
	}
}

func (s *Store) Create(ctx context.Context, chapter int, input CreateInput) (Version, error) {
	if s == nil || s.db == nil || chapter < 1 {
		return Version{}, newError(CodeValidation, "positive chapter is required", false, nil)
	}
	input.Content = domain.NormalizeChapterContent(input.Content)
	if input.Content == "" {
		return Version{}, newError(CodeValidation, "chapter content is required", false, nil)
	}
	if input.Type == "" {
		input.Type = TypeDraft
	}
	if input.AuthorType == "" {
		input.AuthorType = AuthorHuman
	}
	if !validType(input.Type) || !validAuthor(input.AuthorType) {
		return Version{}, newError(CodeValidation, "chapter version type or author type is invalid", false, nil)
	}
	if input.AuthorType == AuthorHuman && (strings.TrimSpace(input.Provider) != "" || strings.TrimSpace(input.Model) != "") {
		return Version{}, newError(CodeValidation, "human revisions cannot claim model provenance", false, nil)
	}
	review, err := normalizeJSON(input.Review)
	if err != nil {
		return Version{}, err
	}
	continuity, err := normalizeJSON(input.Continuity)
	if err != nil {
		return Version{}, err
	}
	provenance, err := normalizeJSON(input.Provenance)
	if err != nil {
		return Version{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Version{}, newError(CodeStorage, "chapter version transaction could not start", true, err)
	}
	defer tx.Rollback()

	if input.ParentVersionID != "" {
		var parentProject string
		var parentChapter int
		if err := tx.QueryRowContext(ctx, `SELECT project_id, chapter FROM chapter_versions WHERE id=?`, input.ParentVersionID).Scan(&parentProject, &parentChapter); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return Version{}, newError(CodeNotFound, "parent chapter version was not found", false, ErrNotFound)
			}
			return Version{}, newError(CodeStorage, "parent chapter version could not be read", true, err)
		}
		if parentProject != s.projectID || parentChapter != chapter {
			return Version{}, newError(CodeConflict, "parent version belongs to another project or chapter", false, nil)
		}
	}

	var number int
	if err := tx.QueryRowContext(ctx, `INSERT INTO chapter_version_counters(project_id,chapter,next_version)
		VALUES(?,?,1) ON CONFLICT(project_id,chapter) DO UPDATE SET next_version=chapter_version_counters.next_version+1
		RETURNING next_version`, s.projectID, chapter).Scan(&number); err != nil {
		return Version{}, newError(CodeStorage, "next chapter version number could not be allocated", true, err)
	}
	id, err := newOpaqueID("cv_")
	if err != nil {
		return Version{}, newError(CodeStorage, "chapter version id could not be generated", true, err)
	}
	now := s.now().UTC()
	sha := domain.ChapterContentSHA256(input.Content)
	_, err = tx.ExecContext(ctx, `INSERT INTO chapter_versions(
		id,project_id,chapter,version_number,version_type,content,content_sha,parent_version_id,
		author_type,provider,model,prompt_hash,review_json,continuity_json,provenance_json,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id,
		s.projectID,
		chapter,
		number,
		string(input.Type),
		input.Content,
		sha,
		nullString(input.ParentVersionID),
		string(input.AuthorType),
		strings.TrimSpace(input.Provider),
		strings.TrimSpace(input.Model),
		strings.TrimSpace(input.PromptHash),
		string(review),
		string(continuity),
		string(provenance),
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return Version{}, newError(CodeConflict, "chapter version could not be created", false, err)
	}
	if err := appendEventTx(ctx, tx, s.projectID, chapter, id, "created", "immutable chapter version created", json.RawMessage(`{}`), now); err != nil {
		return Version{}, err
	}
	if err := tx.Commit(); err != nil {
		return Version{}, newError(CodeStorage, "chapter version could not commit", true, err)
	}
	return s.Get(ctx, chapter, id, true)
}

func (s *Store) Get(ctx context.Context, chapter int, versionID string, includeContent bool) (Version, error) {
	versionID = strings.TrimSpace(versionID)
	if chapter < 1 || versionID == "" {
		return Version{}, newError(CodeValidation, "chapter and version are required", false, nil)
	}
	row := s.db.QueryRowContext(ctx, `SELECT id,project_id,chapter,version_number,version_type,content,content_sha,
		COALESCE(parent_version_id,''),author_type,provider,model,prompt_hash,review_json,continuity_json,provenance_json,created_at
		FROM chapter_versions WHERE id=? AND project_id=? AND chapter=?`, versionID, s.projectID, chapter)
	version, err := scanVersion(row, includeContent)
	if errors.Is(err, sql.ErrNoRows) {
		return Version{}, newError(CodeNotFound, "chapter version was not found", false, ErrNotFound)
	}
	if err != nil {
		return Version{}, newError(CodeStorage, "chapter version could not be read", true, err)
	}
	if err := s.decorate(ctx, &version); err != nil {
		return Version{}, err
	}
	return version, nil
}

func (s *Store) Latest(ctx context.Context, chapter int, includeContent bool) (*Version, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id,project_id,chapter,version_number,version_type,content,content_sha,
		COALESCE(parent_version_id,''),author_type,provider,model,prompt_hash,review_json,continuity_json,provenance_json,created_at
		FROM chapter_versions WHERE project_id=? AND chapter=? ORDER BY version_number DESC LIMIT 1`, s.projectID, chapter)
	version, err := scanVersion(row, includeContent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, newError(CodeStorage, "latest chapter version could not be read", true, err)
	}
	if err := s.decorate(ctx, &version); err != nil {
		return nil, err
	}
	return &version, nil
}

func (s *Store) ActiveFinal(ctx context.Context, chapter int, includeContent bool) (*Version, error) {
	row := s.db.QueryRowContext(ctx, `SELECT v.id,v.project_id,v.chapter,v.version_number,v.version_type,v.content,v.content_sha,
		COALESCE(v.parent_version_id,''),v.author_type,v.provider,v.model,v.prompt_hash,v.review_json,v.continuity_json,v.provenance_json,v.created_at
		FROM chapter_active_finals a JOIN chapter_versions v ON v.id=a.version_id
		WHERE a.project_id=? AND a.chapter=?`, s.projectID, chapter)
	version, err := scanVersion(row, includeContent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, newError(CodeStorage, "active final could not be read", true, err)
	}
	if err := s.decorate(ctx, &version); err != nil {
		return nil, err
	}
	return &version, nil
}

func (s *Store) List(ctx context.Context, chapter int, options ListOptions) (ListResult, error) {
	if chapter < 1 || options.Offset < 0 {
		return ListResult{}, newError(CodeValidation, "chapter and pagination are invalid", false, nil)
	}
	if options.Limit <= 0 {
		options.Limit = 50
	}
	if options.Limit > maxListLimit {
		options.Limit = maxListLimit
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,project_id,chapter,version_number,version_type,content,content_sha,
		COALESCE(parent_version_id,''),author_type,provider,model,prompt_hash,review_json,continuity_json,provenance_json,created_at
		FROM chapter_versions WHERE project_id=? AND chapter=? ORDER BY version_number DESC`, s.projectID, chapter)
	if err != nil {
		return ListResult{}, newError(CodeStorage, "chapter versions could not be listed", true, err)
	}

	all := []Version{}
	for rows.Next() {
		version, err := scanVersion(rows, options.IncludeContent)
		if err != nil {
			_ = rows.Close()
			return ListResult{}, newError(CodeStorage, "chapter version row could not be decoded", true, err)
		}
		all = append(all, version)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return ListResult{}, newError(CodeStorage, "chapter version iteration failed", true, err)
	}
	if err := rows.Close(); err != nil {
		return ListResult{}, newError(CodeStorage, "chapter version rows could not be closed", true, err)
	}

	filtered := make([]Version, 0, len(all))
	for i := range all {
		if err := s.decorate(ctx, &all[i]); err != nil {
			return ListResult{}, err
		}
		version := all[i]
		if options.Type != "" && version.Type != options.Type {
			continue
		}
		if options.AuthorType != "" && version.AuthorType != options.AuthorType {
			continue
		}
		if options.Status != "" && version.Status != options.Status {
			continue
		}
		filtered = append(filtered, version)
	}

	result := ListResult{Versions: []Version{}, Total: len(filtered), Limit: options.Limit, Offset: options.Offset}
	if options.Offset >= len(filtered) {
		return result, nil
	}
	end := options.Offset + options.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	result.Versions = append(result.Versions, filtered[options.Offset:end]...)
	if end < len(filtered) {
		next := end
		result.NextOffset = &next
	}
	return result, nil
}

type scanner interface {
	Scan(...any) error
}

func scanVersion(row scanner, includeContent bool) (Version, error) {
	var version Version
	var typ, author, created string
	var content, review, continuity, provenance string
	if err := row.Scan(
		&version.ID,
		&version.ProjectID,
		&version.Chapter,
		&version.VersionNumber,
		&typ,
		&content,
		&version.ContentSHA,
		&version.ParentVersionID,
		&author,
		&version.Provider,
		&version.Model,
		&version.PromptHash,
		&review,
		&continuity,
		&provenance,
		&created,
	); err != nil {
		return Version{}, err
	}
	version.Type = VersionType(typ)
	version.AuthorType = AuthorType(author)
	version.Review = json.RawMessage(review)
	version.Continuity = json.RawMessage(continuity)
	version.Provenance = json.RawMessage(provenance)
	if includeContent {
		version.Content = content
	}
	parsed, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Version{}, err
	}
	version.CreatedAt = parsed.UTC()
	version.Status = string(version.Type)
	return version, nil
}

func nullString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func requestDigest(parts ...string) string {
	return hashText(strings.Join(parts, "\x00"))
}

func decodeReplay[T any](op operation) (T, bool, error) {
	var zero T
	if op.Status != "completed" {
		return zero, false, nil
	}
	if err := json.Unmarshal(op.Result, &zero); err != nil {
		return zero, false, newError(CodeStorage, "stored idempotent result is invalid", false, err)
	}
	return zero, true, nil
}
