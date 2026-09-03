package narrativeledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/voocel/ainovel-cli/internal/truthstore"
)

// HumanAuthorityPromotion is the durable second half of an Accepted Human
// Final ledger commit. Phase 5's CommitAcceptedFinal remains backward-compatible
// and therefore defaults its projection to Generated Final authority. Phase 8
// immediately promotes only the resources touched by the human source version,
// recording an append-only authority audit event. Replays are idempotent.
type HumanAuthorityPromotion struct {
	ProjectID      string `json:"project_id"`
	TransactionID  string `json:"transaction_id"`
	CandidateID    string `json:"candidate_id"`
	Chapter        int    `json:"chapter"`
	SourceVersion  string `json:"source_version"`
	IdempotencyKey string `json:"-"`
}

type HumanAuthorityPromotionResult struct {
	Foreshadows int  `json:"foreshadows"`
	Secrets     int  `json:"secrets"`
	Holders     int  `json:"holders"`
	Replayed    bool `json:"replayed"`
}

func (s *Store) PromoteAcceptedFinalAuthority(ctx context.Context, input HumanAuthorityPromotion) (HumanAuthorityPromotionResult, error) {
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.TransactionID = strings.TrimSpace(input.TransactionID)
	input.CandidateID = strings.TrimSpace(input.CandidateID)
	input.SourceVersion = strings.TrimSpace(input.SourceVersion)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.ProjectID == "" || input.TransactionID == "" || input.CandidateID == "" || input.SourceVersion == "" || input.IdempotencyKey == "" || input.Chapter < 1 {
		return HumanAuthorityPromotionResult{}, fmt.Errorf("%w: human final authority promotion metadata is incomplete", ErrValidation)
	}
	requestHash, err := hashJSON(input)
	if err != nil {
		return HumanAuthorityPromotionResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return HumanAuthorityPromotionResult{}, err
	}
	defer tx.Rollback()
	if _, replay, err := checkOperation(ctx, tx, input.IdempotencyKey, requestHash); err != nil {
		return HumanAuthorityPromotionResult{}, err
	} else if replay {
		result, err := promotionCounts(ctx, tx, input.ProjectID, input.SourceVersion)
		if err != nil {
			return HumanAuthorityPromotionResult{}, err
		}
		result.Replayed = true
		return result, tx.Commit()
	}

	foreshadows, err := resourceIDs(ctx, tx, `SELECT id FROM foreshadows WHERE project_id=? AND source_version=? ORDER BY id`, input.ProjectID, input.SourceVersion)
	if err != nil {
		return HumanAuthorityPromotionResult{}, err
	}
	secrets, err := resourceIDs(ctx, tx, `SELECT id FROM secrets WHERE project_id=? AND source_version=? ORDER BY id`, input.ProjectID, input.SourceVersion)
	if err != nil {
		return HumanAuthorityPromotionResult{}, err
	}
	var holderCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM secret_holders h JOIN secrets s ON s.id=h.secret_id WHERE s.project_id=? AND h.source_version=?`, input.ProjectID, input.SourceVersion).Scan(&holderCount); err != nil {
		return HumanAuthorityPromotionResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE foreshadows SET authority=?,updated_at=? WHERE project_id=? AND source_version=?`, truthstore.AuthorityHumanFinal, utcText(s.now()), input.ProjectID, input.SourceVersion); err != nil {
		return HumanAuthorityPromotionResult{}, classifyWrite(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE secrets SET authority=?,updated_at=? WHERE project_id=? AND source_version=?`, truthstore.AuthorityHumanFinal, utcText(s.now()), input.ProjectID, input.SourceVersion); err != nil {
		return HumanAuthorityPromotionResult{}, classifyWrite(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE secret_holders SET authority=? WHERE source_version=? AND secret_id IN (SELECT id FROM secrets WHERE project_id=?)`, truthstore.AuthorityHumanFinal, input.SourceVersion, input.ProjectID); err != nil {
		return HumanAuthorityPromotionResult{}, classifyWrite(err)
	}

	source := truthstore.Source{Type: "chapter_human_final", ID: input.CandidateID, Chapter: input.Chapter, Version: input.SourceVersion, ConfirmedBy: "human"}
	payload := map[string]any{"authority": truthstore.AuthorityHumanFinal, "transaction_id": input.TransactionID}
	for _, id := range foreshadows {
		if err := appendForeshadowEvent(ctx, tx, id, input.ProjectID, "authority_promoted", input.Chapter, payload, input.SourceVersion, truthstore.AuthorityHumanFinal, source, s.now); err != nil {
			return HumanAuthorityPromotionResult{}, err
		}
	}
	for _, id := range secrets {
		if err := appendSecretEvent(ctx, tx, id, input.ProjectID, "authority_promoted", input.Chapter, payload, input.SourceVersion, truthstore.AuthorityHumanFinal, source, s.now); err != nil {
			return HumanAuthorityPromotionResult{}, err
		}
	}
	result := HumanAuthorityPromotionResult{Foreshadows: len(foreshadows), Secrets: len(secrets), Holders: holderCount}
	resource := input.TransactionID
	if err := saveOperation(ctx, tx, input.IdempotencyKey, requestHash, "accepted_final.promote_human_authority", resource, utcText(s.now())); err != nil {
		return HumanAuthorityPromotionResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return HumanAuthorityPromotionResult{}, classifyWrite(err)
	}
	return result, nil
}

func resourceIDs(ctx context.Context, tx *sql.Tx, query, projectID, sourceVersion string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, projectID, sourceVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func promotionCounts(ctx context.Context, tx *sql.Tx, projectID, sourceVersion string) (HumanAuthorityPromotionResult, error) {
	var result HumanAuthorityPromotionResult
	queries := []struct {
		query string
		dest  *int
	}{
		{`SELECT COUNT(*) FROM foreshadows WHERE project_id=? AND source_version=? AND authority='human_final'`, &result.Foreshadows},
		{`SELECT COUNT(*) FROM secrets WHERE project_id=? AND source_version=? AND authority='human_final'`, &result.Secrets},
		{`SELECT COUNT(*) FROM secret_holders h JOIN secrets s ON s.id=h.secret_id WHERE s.project_id=? AND h.source_version=? AND h.authority='human_final'`, &result.Holders},
	}
	for _, item := range queries {
		if err := tx.QueryRowContext(ctx, item.query, projectID, sourceVersion).Scan(item.dest); err != nil {
			return HumanAuthorityPromotionResult{}, err
		}
	}
	return result, nil
}

var _ = json.Valid
