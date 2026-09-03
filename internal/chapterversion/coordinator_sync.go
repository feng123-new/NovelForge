package chapterversion

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
)

func (c *Coordinator) SyncExternal(ctx context.Context, chapter int, key, detectedSHA string) (SyncResult, error) {
	if err := c.validate(); err != nil {
		return SyncResult{}, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return SyncResult{}, newError(CodeValidation, "Idempotency-Key is required", false, nil)
	}
	status, err := c.Store.DetectExternal(ctx, chapter)
	if err != nil {
		return SyncResult{}, err
	}
	if !status.SyncRequired {
		return SyncResult{}, newError(CodeSyncRequired, "chapter file matches the active final; no sync is required", false, nil)
	}
	if detectedSHA != "" && detectedSHA != status.ObservedSHA {
		return SyncResult{}, newError(CodeSyncContentChanged, "chapter file changed after the displayed sync status", false, nil)
	}
	content, observed, err := c.Store.readExternal(chapter)
	if err != nil {
		return SyncResult{}, err
	}
	if observed != status.ObservedSHA {
		return SyncResult{}, newError(CodeSyncContentChanged, "chapter file changed while sync was starting", false, nil)
	}

	digest := requestDigest("external_sync", c.Store.projectID, strconv.Itoa(chapter), status.ActiveVersionID, status.ExpectedSHA, observed)
	op, replay, err := c.Store.BeginOperation(ctx, key, "external_sync", chapter, status.ActiveVersionID, digest)
	if err != nil {
		return SyncResult{}, err
	}
	if replay {
		if result, ok, decodeErr := decodeReplay[SyncResult](op); decodeErr != nil {
			return SyncResult{}, decodeErr
		} else if ok {
			return result, nil
		}
	}

	active, err := c.Store.ActiveFinal(ctx, chapter, false)
	if err != nil || active == nil || active.ID != status.ActiveVersionID || active.ContentSHA != status.ExpectedSHA {
		return SyncResult{}, newError(CodeSyncContentChanged, "active final changed while sync was starting", false, err)
	}
	payload := map[string]string{}
	payload["expected_sha"] = status.ExpectedSHA
	payload["observed_sha"] = observed
	if err := c.Store.AppendEvent(ctx, chapter, active.ID, "sync_started", "explicit external chapter synchronization started", mustJSON(payload)); err != nil {
		return SyncResult{}, err
	}

	matching, err := c.Store.findMatching(ctx, chapter, active.ID, TypeHumanRevision, AuthorHuman, observed)
	if err != nil {
		return SyncResult{}, err
	}
	var human Version
	if matching != nil {
		human = *matching
	} else {
		provenance := map[string]any{}
		provenance["source"] = "external_file_sync"
		provenance["original_sha"] = status.ExpectedSHA
		provenance["observed_sha"] = observed
		provenance["parent_final"] = active.ID
		input := CreateInput{}
		input.Content = content
		input.Type = TypeHumanRevision
		input.ParentVersionID = active.ID
		input.AuthorType = AuthorHuman
		input.Provenance = mustJSON(provenance)
		human, err = c.Store.Create(ctx, chapter, input)
		if err != nil {
			_ = c.Store.FailOperation(ctx, key, errorCode(err))
			return SyncResult{}, err
		}
	}

	evaluation, err := c.evaluate(ctx, human)
	if err != nil {
		_ = c.Store.FailOperation(ctx, key, errorCode(err))
		return SyncResult{Version: human, SyncRequired: true}, err
	}
	if err := c.Store.AppendEvent(ctx, chapter, human.ID, "sync_completed", "external chapter content evaluated and retained as human_revision", mustJSON(evaluation)); err != nil {
		return SyncResult{}, err
	}
	proposalJSON, _ := json.Marshal(evaluation.Proposal)
	continuityJSON, _ := json.Marshal(evaluation.Continuity)
	var reviewJSON json.RawMessage
	if evaluation.Review != nil {
		reviewJSON, _ = json.Marshal(evaluation.Review)
	}
	result := SyncResult{}
	result.Version = human
	result.Proposal = proposalJSON
	result.Continuity = continuityJSON
	result.Review = reviewJSON
	result.Conflicts = len(evaluation.Conflicts)
	result.SyncRequired = true
	if err := c.Store.CompleteOperation(ctx, key, result); err != nil {
		return SyncResult{}, err
	}
	return result, nil
}
