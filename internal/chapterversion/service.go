package chapterversion

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/voocel/ainovel-cli/internal/domain"
)

type Service struct {
	Store *Store
}

func (s *Service) validate() error {
	if s == nil || s.Store == nil {
		return newError(CodeStorage, "chapter version service is not configured", false, nil)
	}
	return nil
}

// SaveHuman always appends a human_revision. It never mutates or implicitly
// replaces the active Final, and it records the active Final as parent when one
// exists so later checks have an explicit comparison boundary.
func (s *Service) SaveHuman(ctx context.Context, chapter int, key, content string) (Version, error) {
	if err := s.validate(); err != nil {
		return Version{}, err
	}
	content = domain.NormalizeChapterContent(content)
	if strings.TrimSpace(key) == "" || content == "" {
		return Version{}, newError(CodeValidation, "Idempotency-Key and chapter content are required", false, nil)
	}
	active, err := s.Store.ActiveFinal(ctx, chapter, false)
	if err != nil {
		return Version{}, err
	}
	parent := ""
	if active != nil {
		parent = active.ID
	}
	digest := requestDigest("human_save", s.Store.projectID, string(rune(chapter)), parent, domain.ChapterContentSHA256(content))
	op, replay, err := s.Store.BeginOperation(ctx, key, "human_save", chapter, parent, digest)
	if err != nil {
		return Version{}, err
	}
	if replay {
		if result, ok, decodeErr := decodeReplay[Version](op); decodeErr != nil {
			return Version{}, decodeErr
		} else if ok {
			return result, nil
		}
		if existing, findErr := s.Store.findMatching(ctx, chapter, parent, TypeHumanRevision, AuthorHuman, domain.ChapterContentSHA256(content)); findErr == nil && existing != nil {
			_ = s.Store.CompleteOperation(ctx, key, *existing)
			return *existing, nil
		}
	}
	provenance, _ := json.Marshal(map[string]any{
		"source":       "human_editor",
		"parent_final": parent,
		"content_sha":  domain.ChapterContentSHA256(content),
	})
	version, err := s.Store.Create(ctx, chapter, CreateInput{Content: content, Type: TypeHumanRevision, ParentVersionID: parent, AuthorType: AuthorHuman, Provenance: provenance})
	if err != nil {
		_ = s.Store.FailOperation(ctx, key, errorCode(err))
		return Version{}, err
	}
	if err := s.Store.CompleteOperation(ctx, key, version); err != nil {
		return Version{}, err
	}
	return version, nil
}

func (s *Service) Restore(ctx context.Context, chapter int, key, sourceVersionID string) (Version, error) {
	if err := s.validate(); err != nil {
		return Version{}, err
	}
	source, err := s.Store.Get(ctx, chapter, sourceVersionID, true)
	if err != nil {
		return Version{}, err
	}
	digest := requestDigest("restore", s.Store.projectID, source.ID, source.ContentSHA)
	op, replay, err := s.Store.BeginOperation(ctx, key, "restore", chapter, source.ID, digest)
	if err != nil {
		return Version{}, err
	}
	if replay {
		if result, ok, decodeErr := decodeReplay[Version](op); decodeErr != nil {
			return Version{}, decodeErr
		} else if ok {
			return result, nil
		}
		if existing, findErr := s.Store.findMatching(ctx, chapter, source.ID, TypeDraft, AuthorRestore, source.ContentSHA); findErr == nil && existing != nil {
			_ = s.Store.CompleteOperation(ctx, key, *existing)
			return *existing, nil
		}
	}
	provenance, _ := json.Marshal(map[string]any{"restore_from": source.ID, "restore_from_sha": source.ContentSHA})
	version, err := s.Store.Create(ctx, chapter, CreateInput{Content: source.Content, Type: TypeDraft, ParentVersionID: source.ID, AuthorType: AuthorRestore, Provenance: provenance})
	if err != nil {
		_ = s.Store.FailOperation(ctx, key, errorCode(err))
		return Version{}, err
	}
	payload, _ := json.Marshal(map[string]string{"source_version": source.ID})
	if err := s.Store.AppendEvent(ctx, chapter, version.ID, "restore", "restored content into a new immutable version", payload); err != nil {
		return Version{}, err
	}
	if err := s.Store.CompleteOperation(ctx, key, version); err != nil {
		return Version{}, err
	}
	return version, nil
}

func (s *Service) Accept(ctx context.Context, chapter int, key, versionID, reason string) (Version, error) {
	return s.reviewAction(ctx, chapter, key, versionID, "accept", reason)
}

func (s *Service) Reject(ctx context.Context, chapter int, key, versionID, reason string) (Version, error) {
	return s.reviewAction(ctx, chapter, key, versionID, "reject", reason)
}

func (s *Service) reviewAction(ctx context.Context, chapter int, key, versionID, action, reason string) (Version, error) {
	if err := s.validate(); err != nil {
		return Version{}, err
	}
	version, err := s.Store.Get(ctx, chapter, versionID, false)
	if err != nil {
		return Version{}, err
	}
	digest := requestDigest(action, s.Store.projectID, version.ID, strings.TrimSpace(reason))
	op, replay, err := s.Store.BeginOperation(ctx, key, action, chapter, version.ID, digest)
	if err != nil {
		return Version{}, err
	}
	if replay {
		if result, ok, decodeErr := decodeReplay[Version](op); decodeErr != nil {
			return Version{}, decodeErr
		} else if ok {
			return result, nil
		}
	}
	if action == "accept" {
		version, err = s.Store.Accept(ctx, chapter, version.ID, reason)
	} else {
		version, err = s.Store.Reject(ctx, chapter, version.ID, reason)
	}
	if err != nil {
		_ = s.Store.FailOperation(ctx, key, errorCode(err))
		return Version{}, err
	}
	if err := s.Store.CompleteOperation(ctx, key, version); err != nil {
		return Version{}, err
	}
	return version, nil
}

func (s *Store) findMatching(ctx context.Context, chapter int, parent string, typ VersionType, author AuthorType, sha string) (*Version, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM chapter_versions WHERE project_id=? AND chapter=? AND COALESCE(parent_version_id,'')=?
		AND version_type=? AND author_type=? AND content_sha=? ORDER BY version_number ASC LIMIT 1`,
		s.projectID, chapter, parent, string(typ), string(author), sha).Scan(&id)
	if err != nil {
		if errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), "no rows") {
			return nil, nil
		}
		return nil, newError(CodeStorage, "chapter version idempotency lookup failed", true, err)
	}
	version, err := s.Get(ctx, chapter, id, true)
	if err != nil {
		return nil, err
	}
	return &version, nil
}

func errorCode(err error) string {
	var chapterErr *Error
	if errors.As(err, &chapterErr) {
		return chapterErr.Code
	}
	return CodeStorage
}
