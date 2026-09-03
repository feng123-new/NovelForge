package chapterversion

import (
	"context"
	"strings"
)

func (c *Coordinator) Check(ctx context.Context, chapter int, key, versionID string) (Evaluation, error) {
	if err := c.validate(); err != nil {
		return Evaluation{}, err
	}
	version, err := c.Store.Get(ctx, chapter, versionID, true)
	if err != nil {
		return Evaluation{}, err
	}
	digest := requestDigest("check", c.Store.projectID, version.ID, version.ContentSHA)
	op, replay, err := c.Store.BeginOperation(ctx, strings.TrimSpace(key), "check", chapter, version.ID, digest)
	if err != nil {
		return Evaluation{}, err
	}
	if replay {
		if result, ok, decodeErr := decodeReplay[Evaluation](op); decodeErr != nil {
			return Evaluation{}, decodeErr
		} else if ok {
			return result, nil
		}
	}
	evaluation, err := c.evaluate(ctx, version)
	if err != nil {
		_ = c.Store.FailOperation(ctx, key, errorCode(err))
		return Evaluation{}, err
	}
	if err := c.Store.AppendEvent(ctx, chapter, version.ID, "evaluation_completed", "chapter version quality and conflict evaluation completed", mustJSON(evaluation)); err != nil {
		return Evaluation{}, err
	}
	if err := c.Store.CompleteOperation(ctx, key, evaluation); err != nil {
		return Evaluation{}, err
	}
	return evaluation, nil
}
