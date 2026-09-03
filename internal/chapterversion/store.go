package chapterversion

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
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
	mu        sync.Mutex
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
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(buf), nil
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

	s.mu.Lock()
	defer s.mu.Unlock()
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
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_number),0)+1 FROM chapter_versions WHERE project_id=? AND chapter=?`, s.projectID, chapter).Scan(&number); err != nil {
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
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, s.projectID, chapter, number, string(input.Type), input.Content, sha,
		nullString(input.ParentVersionID), string(input.AuthorType), strings.TrimSpace(input.Provider), strings.TrimSpace(input.Model),
		strings.TrimSpace(input.PromptHash), string(review), string(continuity), string(provenance), now.Format(time.RFC3339Nano))
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
	defer rows.Close()
	all := []Version{}
	for rows.Next() {
		version, err := scanVersion(rows, options.IncludeContent)
		if err != nil {
			return ListResult{}, newError(CodeStorage, "chapter version row could not be decoded", true, err)
		}
		if err := s.decorate(ctx, &version); err != nil {
			return ListResult{}, err
		}
		if options.Type != "" && version.Type != options.Type {
			continue
		}
		if options.AuthorType != "" && version.AuthorType != options.AuthorType {
			continue
		}
		if options.Status != "" && version.Status != options.Status {
			continue
		}
		all = append(all, version)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, newError(CodeStorage, "chapter version iteration failed", true, err)
	}
	result := ListResult{Versions: []Version{}, Total: len(all), Limit: options.Limit, Offset: options.Offset}
	if options.Offset >= len(all) {
		return result, nil
	}
	end := options.Offset + options.Limit
	if end > len(all) {
		end = len(all)
	}
	result.Versions = append(result.Versions, all[options.Offset:end]...)
	if end < len(all) {
		next := end
		result.NextOffset = &next
	}
	return result, nil
}

type scanner interface{ Scan(...any) error }

func scanVersion(row scanner, includeContent bool) (Version, error) {
	var version Version
	var typ, author, created string
	var content, review, continuity, provenance string
	if err := row.Scan(&version.ID, &version.ProjectID, &version.Chapter, &version.VersionNumber, &typ, &content,
		&version.ContentSHA, &version.ParentVersionID, &author, &version.Provider, &version.Model, &version.PromptHash,
		&review, &continuity, &provenance, &created); err != nil {
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

func (s *Store) decorate(ctx context.Context, version *Version) error {
	var accepted, rejected int
	var reason string
	if err := s.db.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM chapter_version_events WHERE version_id=? AND event_type='accept'),
		EXISTS(SELECT 1 FROM chapter_version_events WHERE version_id=? AND event_type='reject'),
		COALESCE((SELECT reason FROM chapter_version_events WHERE version_id=? AND event_type='reject' ORDER BY sequence DESC LIMIT 1),'')`,
		version.ID, version.ID, version.ID).Scan(&accepted, &rejected, &reason); err != nil {
		return newError(CodeStorage, "chapter version state could not be projected", true, err)
	}
	version.Accepted = accepted == 1
	version.Rejected = rejected == 1
	version.RejectionReason = reason
	if version.Rejected {
		version.Status = "rejected"
	} else if version.Accepted && version.Type != TypeFinal {
		version.Status = "accepted"
	}
	var activeID, authority string
	err := s.db.QueryRowContext(ctx, `SELECT version_id,authority FROM chapter_active_finals WHERE project_id=? AND chapter=?`, s.projectID, version.Chapter).Scan(&activeID, &authority)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return newError(CodeStorage, "active final state could not be projected", true, err)
	}
	if activeID == version.ID {
		version.ActiveFinal = true
		version.Authority = authority
		version.Status = "final"
	}
	return nil
}

func appendEventTx(ctx context.Context, tx *sql.Tx, projectID string, chapter int, versionID, eventType, reason string, payload json.RawMessage, now time.Time) error {
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	id, err := newOpaqueID("cve_")
	if err != nil {
		return newError(CodeStorage, "chapter version event id could not be generated", true, err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO chapter_version_events(id,project_id,chapter,version_id,event_type,reason,payload_json,created_at)
		VALUES(?,?,?,?,?,?,?,?)`, id, projectID, chapter, versionID, eventType, strings.TrimSpace(reason), string(payload), now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return newError(CodeStorage, "chapter version event could not be written", true, err)
	}
	return nil
}

func (s *Store) AppendEvent(ctx context.Context, chapter int, versionID, eventType, reason string, payload json.RawMessage) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return newError(CodeStorage, "chapter version event transaction could not start", true, err)
	}
	defer tx.Rollback()
	if err := appendEventTx(ctx, tx, s.projectID, chapter, versionID, eventType, reason, payload, s.now().UTC()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return newError(CodeStorage, "chapter version event could not commit", true, err)
	}
	return nil
}

func (s *Store) BeginOperation(ctx context.Context, key, operationName string, chapter int, versionID, requestHash string) (operation, bool, error) {
	key, operationName, requestHash = strings.TrimSpace(key), strings.TrimSpace(operationName), strings.TrimSpace(requestHash)
	if key == "" || operationName == "" || chapter < 1 || len(requestHash) != 64 {
		return operation{}, false, newError(CodeValidation, "idempotent operation identity is invalid", false, nil)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return operation{}, false, newError(CodeStorage, "chapter operation transaction could not start", true, err)
	}
	defer tx.Rollback()
	var existing operation
	var result string
	err = tx.QueryRowContext(ctx, `SELECT idempotency_key,operation,project_id,chapter,version_id,request_hash,status,result_json
		FROM chapter_revision_operations WHERE idempotency_key=?`, key).
		Scan(&existing.Key, &existing.Operation, &existing.ProjectID, &existing.Chapter, &existing.VersionID, &existing.Hash, &existing.Status, &result)
	if err == nil {
		existing.Result = json.RawMessage(result)
		if existing.Operation != operationName || existing.ProjectID != s.projectID || existing.Chapter != chapter || existing.VersionID != versionID || existing.Hash != requestHash {
			return operation{}, false, newError(CodeIdempotencyConflict, "Idempotency-Key was already used for a different request", false, nil)
		}
		if err := tx.Commit(); err != nil {
			return operation{}, false, newError(CodeStorage, "chapter operation replay could not commit", true, err)
		}
		return existing, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return operation{}, false, newError(CodeStorage, "chapter operation could not be read", true, err)
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT INTO chapter_revision_operations(idempotency_key,operation,project_id,chapter,version_id,request_hash,status,result_json,created_at)
		VALUES(?,?,?,?,?,?, 'pending','{}',?)`, key, operationName, s.projectID, chapter, versionID, requestHash, now)
	if err != nil {
		return operation{}, false, newError(CodeConflict, "chapter operation could not be reserved", true, err)
	}
	if err := tx.Commit(); err != nil {
		return operation{}, false, newError(CodeStorage, "chapter operation could not commit", true, err)
	}
	return operation{Key: key, Operation: operationName, ProjectID: s.projectID, Chapter: chapter, VersionID: versionID, Hash: requestHash, Status: "pending", Result: json.RawMessage(`{}`)}, false, nil
}

func (s *Store) CompleteOperation(ctx context.Context, key string, result any) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return newError(CodeStorage, "chapter operation result could not be encoded", false, err)
	}
	_, err = s.db.ExecContext(ctx, `UPDATE chapter_revision_operations SET status='completed',result_json=?,completed_at=? WHERE idempotency_key=? AND status!='completed'`,
		string(encoded), s.now().UTC().Format(time.RFC3339Nano), strings.TrimSpace(key))
	if err != nil {
		return newError(CodeStorage, "chapter operation could not be completed", true, err)
	}
	return nil
}

func (s *Store) FailOperation(ctx context.Context, key, code string) error {
	payload, _ := json.Marshal(map[string]string{"error_code": code})
	_, err := s.db.ExecContext(ctx, `UPDATE chapter_revision_operations SET status='failed',result_json=?,completed_at=? WHERE idempotency_key=? AND status!='completed'`,
		string(payload), s.now().UTC().Format(time.RFC3339Nano), strings.TrimSpace(key))
	if err != nil {
		return newError(CodeStorage, "chapter operation failure could not be recorded", true, err)
	}
	return nil
}

func (s *Store) Accept(ctx context.Context, chapter int, versionID, reason string) (Version, error) {
	version, err := s.Get(ctx, chapter, versionID, true)
	if err != nil {
		return Version{}, err
	}
	if version.Rejected {
		return Version{}, newError(CodeRejected, "rejected versions cannot be accepted", false, nil)
	}
	if continuityBlocks(version.Continuity) {
		return Version{}, newError(CodeContinuityBlocked, "continuity FAIL blocks acceptance", false, nil)
	}
	if version.Accepted {
		return version, nil
	}
	if err := s.AppendEvent(ctx, chapter, versionID, "accept", reason, json.RawMessage(`{}`)); err != nil {
		return Version{}, err
	}
	return s.Get(ctx, chapter, versionID, true)
}

func (s *Store) Reject(ctx context.Context, chapter int, versionID, reason string) (Version, error) {
	if strings.TrimSpace(reason) == "" {
		return Version{}, newError(CodeValidation, "rejection reason is required", false, nil)
	}
	version, err := s.Get(ctx, chapter, versionID, true)
	if err != nil {
		return Version{}, err
	}
	if version.ActiveFinal {
		return Version{}, newError(CodeFinalizeNotAllowed, "the active final cannot be rejected in place", false, nil)
	}
	if version.Rejected {
		return version, nil
	}
	if err := s.AppendEvent(ctx, chapter, versionID, "reject", reason, json.RawMessage(`{}`)); err != nil {
		return Version{}, err
	}
	return s.Get(ctx, chapter, versionID, true)
}

func (s *Store) SwitchActiveFinal(ctx context.Context, chapter int, finalVersionID, authority string) error {
	if authority != AuthorityGeneratedFinal && authority != AuthorityHumanFinal {
		return newError(CodeValidation, "active final authority is invalid", false, nil)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return newError(CodeStorage, "active final transaction could not start", true, err)
	}
	defer tx.Rollback()
	var typ string
	if err := tx.QueryRowContext(ctx, `SELECT version_type FROM chapter_versions WHERE id=? AND project_id=? AND chapter=?`, finalVersionID, s.projectID, chapter).Scan(&typ); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return newError(CodeNotFound, "final chapter version was not found", false, ErrNotFound)
		}
		return newError(CodeStorage, "final chapter version could not be validated", true, err)
	}
	if VersionType(typ) != TypeFinal {
		return newError(CodeFinalizeNotAllowed, "only final versions can become active", false, nil)
	}
	now := s.now().UTC()
	_, err = tx.ExecContext(ctx, `INSERT INTO chapter_active_finals(project_id,chapter,version_id,authority,activated_at)
		VALUES(?,?,?,?,?) ON CONFLICT(project_id,chapter) DO UPDATE SET version_id=excluded.version_id,authority=excluded.authority,activated_at=excluded.activated_at`,
		s.projectID, chapter, finalVersionID, authority, now.Format(time.RFC3339Nano))
	if err != nil {
		return newError(CodeActiveFinalConflict, "active final could not be switched", true, err)
	}
	payload, _ := json.Marshal(map[string]string{"authority": authority})
	if err := appendEventTx(ctx, tx, s.projectID, chapter, finalVersionID, "active_final_switched", "active final switched", payload, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return newError(CodeStorage, "active final switch could not commit", true, err)
	}
	return nil
}

func (s *Store) CountUnresolvedTruthConflicts(ctx context.Context, chapter int) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM truth_conflicts WHERE status='unresolved' AND from_chapter<=? AND (to_chapter IS NULL OR to_chapter>=?)`, chapter, chapter).Scan(&count); err != nil {
		return 0, newError(CodeStorage, "truth conflicts could not be checked", true, err)
	}
	return count, nil
}

func continuityBlocks(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "{}" {
		return false
	}
	var state struct {
		Status   string `json:"status"`
		Blocking bool   `json:"blocking"`
	}
	if json.Unmarshal(raw, &state) != nil {
		return true
	}
	return strings.EqualFold(state.Status, "FAIL") || state.Blocking
}

func nullString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func requestDigest(parts ...string) string { return hashText(strings.Join(parts, "\x00")) }

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

func (s *Store) DebugCounts(ctx context.Context, chapter int) (map[string]int, error) {
	queries := map[string]string{
		"versions":    `SELECT COUNT(*) FROM chapter_versions WHERE project_id=? AND chapter=?`,
		"active_final": `SELECT COUNT(*) FROM chapter_active_finals WHERE project_id=? AND chapter=?`,
		"events":      `SELECT COUNT(*) FROM chapter_version_events WHERE project_id=? AND chapter=?`,
	}
	out := map[string]int{}
	for key, query := range queries {
		var count int
		if err := s.db.QueryRowContext(ctx, query, s.projectID, chapter).Scan(&count); err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		out[key] = count
	}
	return out, nil
}
