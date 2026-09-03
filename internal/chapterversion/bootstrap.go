package chapterversion

import (
	"context"
	"encoding/json"
	"errors"
)

// BootstrapLegacyFinal registers a pre-Phase-8 finalized chapter file as the
// initial immutable Generated Final. It is a representation migration only:
// Phase 5 already committed the corresponding Truth/Ledger state, so bootstrap
// deliberately does not append Truth or Narrative Ledger events.
func (s *Store) BootstrapLegacyFinal(ctx context.Context, chapter int) (*Version, error) {
	if active, err := s.ActiveFinal(ctx, chapter, true); err != nil {
		return nil, err
	} else if active != nil {
		return active, nil
	}
	if latest, err := s.Latest(ctx, chapter, true); err != nil {
		return nil, err
	} else if latest != nil {
		return nil, nil
	}
	content, sha, err := s.readExternal(chapter)
	if err != nil {
		if errors.Is(err, ErrNotFound) || IsCode(err, CodeNotFound) {
			return nil, nil
		}
		return nil, err
	}
	provenance, _ := json.Marshal(map[string]any{
		"source":          "pre_phase8_final_bootstrap",
		"content_sha":     sha,
		"truth_replayed":  false,
		"ledger_replayed": false,
	})
	version, err := s.Create(ctx, chapter, CreateInput{
		Content:     content,
		Type:        TypeFinal,
		AuthorType:  AuthorSystem,
		Provenance:  provenance,
	})
	if err != nil {
		// A concurrent bootstrap can win version 1. Re-read the Active Final
		// before surfacing a conflict.
		if active, getErr := s.ActiveFinal(ctx, chapter, true); getErr == nil && active != nil {
			return active, nil
		}
		return nil, err
	}
	if err := s.SwitchActiveFinal(ctx, chapter, version.ID, AuthorityGeneratedFinal); err != nil {
		return nil, err
	}
	active, err := s.ActiveFinal(ctx, chapter, true)
	if err != nil {
		return nil, err
	}
	return active, nil
}
