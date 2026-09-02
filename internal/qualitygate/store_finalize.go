package qualitygate

import (
	"context"
	"database/sql"
	"errors"
)

func (s *Store) SelectFinal(ctx context.Context, transactionID, candidateID, reason string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	candidate, err := scanCandidate(tx.QueryRowContext(ctx, `SELECT id, transaction_id, chapter, attempt, text_content, text_sha, source_version,
		continuity_status, editor_score, selected, selection_reason, created_at FROM chapter_candidates WHERE id=? AND transaction_id=?`, candidateID, transactionID))
	if err != nil {
		return err
	}
	if candidate.ContinuityStatus == ContinuityFail || candidate.ContinuityStatus == "" {
		return ErrNoSafeCandidate
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chapter_candidates SET selected=0, selection_reason='' WHERE transaction_id=?`, transactionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chapter_candidates SET selected=1, selection_reason=? WHERE id=?`, reason, candidateID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chapter_transactions SET final_candidate_id=?, last_reason=?, updated_at=? WHERE id=?`,
		candidateID, reason, utcText(s.now()), transactionID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) BestSafeCandidate(ctx context.Context, transactionID string, policy Policy) (Candidate, string, error) {
	candidates, err := s.Candidates(ctx, transactionID)
	if err != nil {
		return Candidate{}, "", err
	}
	var best *Candidate
	for i := range candidates {
		candidate := candidates[i]
		if candidate.ContinuityStatus == ContinuityFail || candidate.ContinuityStatus == "" || candidate.EditorScore == nil {
			continue
		}
		if candidate.ContinuityStatus == ContinuityWarn && !policy.AllowWarn {
			continue
		}
		if best == nil || *candidate.EditorScore > *best.EditorScore || (*candidate.EditorScore == *best.EditorScore && candidate.Attempt < best.Attempt) {
			copy := candidate
			best = &copy
		}
	}
	if best == nil {
		return Candidate{}, "", ErrNoSafeCandidate
	}
	reason := "highest editor score among continuity-safe candidates"
	if best.ContinuityStatus == ContinuityPass {
		reason = "highest editor score among PASS candidates"
	}
	return *best, reason, nil
}

func (s *Store) StateChanges(ctx context.Context, transactionID string) ([]StateChange, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT transaction_id, chapter, from_state, to_state, reason, actor, attempt, created_at
		FROM chapter_state_changes WHERE transaction_id=? ORDER BY sequence ASC`, transactionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StateChange{}
	for rows.Next() {
		var change StateChange
		var from, to, created string
		if err := rows.Scan(&change.TransactionID, &change.Chapter, &from, &to, &change.Reason, &change.Actor, &change.Attempt, &created); err != nil {
			return nil, err
		}
		change.FromState, change.ToState, change.CreatedAt = TransactionState(from), TransactionState(to), parseUTC(created)
		out = append(out, change)
	}
	return out, rows.Err()
}

func (s *Store) Snapshot(ctx context.Context, projectID string, chapter int) (Snapshot, error) {
	tx, err := s.transactionByProjectChapter(ctx, projectID, chapter)
	if err != nil {
		return Snapshot{}, err
	}
	candidates, err := s.Candidates(ctx, tx.ID)
	if err != nil {
		return Snapshot{}, err
	}
	states, err := s.StateChanges(ctx, tx.ID)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{Transaction: tx, Candidates: candidates, States: states}
	candidateID := tx.FinalCandidateID
	if candidateID == "" && len(candidates) > 0 {
		candidateID = candidates[len(candidates)-1].ID
	}
	if candidateID != "" {
		if proposal, err := s.Proposal(ctx, tx.ID, candidateID); err == nil {
			snapshot.Proposal = &proposal
		}
		if result, err := s.Continuity(ctx, tx.ID, candidateID); err == nil {
			snapshot.Continuity = &result
		}
		if review, err := s.Editor(ctx, tx.ID, candidateID); err == nil {
			snapshot.Editor = &review
		}
	}
	return snapshot, nil
}

func (s *Store) GetModelCall(ctx context.Context, key string) (ModelCall, string, error) {
	var call ModelCall
	var started, ended, responseJSON string
	err := s.db.QueryRowContext(ctx, `SELECT id, idempotency_key, project_id, chapter, transaction_id, agent, operation, provider, model,
		request_hash, response_hash, status, attempt, input_tokens, output_tokens, started_at, ended_at, error_code, response_json
		FROM model_calls WHERE idempotency_key=?`, key).Scan(&call.ID, &call.IdempotencyKey, &call.ProjectID, &call.Chapter,
		&call.TransactionID, &call.Agent, &call.Operation, &call.Provider, &call.Model, &call.RequestHash, &call.ResponseHash,
		&call.Status, &call.Attempt, &call.InputTokens, &call.OutputTokens, &started, &ended, &call.ErrorCode, &responseJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return ModelCall{}, "", ErrNotFound
	}
	if err != nil {
		return ModelCall{}, "", err
	}
	call.StartedAt, call.EndedAt = parseUTC(started), parseUTC(ended)
	return call, responseJSON, nil
}

func (s *Store) SaveModelCall(ctx context.Context, call ModelCall, responseJSON string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO model_calls(id, idempotency_key, project_id, chapter, transaction_id, agent, operation,
		provider, model, request_hash, response_hash, status, attempt, input_tokens, output_tokens, started_at, ended_at, error_code, response_json)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, call.ID, call.IdempotencyKey, call.ProjectID, call.Chapter,
		call.TransactionID, call.Agent, call.Operation, call.Provider, call.Model, call.RequestHash, call.ResponseHash, call.Status, call.Attempt,
		call.InputTokens, call.OutputTokens, utcText(call.StartedAt), utcText(call.EndedAt), call.ErrorCode, responseJSON)
	return err
}

func (s *Store) SaveTruthCommit(ctx context.Context, transactionID, proposalID string, index int, eventID string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO chapter_truth_commits(transaction_id, proposal_id, change_index, truth_event_id, created_at)
		VALUES(?, ?, ?, ?, ?) ON CONFLICT(transaction_id, proposal_id, change_index) DO NOTHING`, transactionID, proposalID, index, eventID, utcText(s.now()))
	return err
}

func (s *Store) SaveCheckpoint(ctx context.Context, transactionID, candidateID, finalSHA string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO chapter_checkpoints(transaction_id, candidate_id, final_sha, created_at)
		VALUES(?, ?, ?, ?) ON CONFLICT(transaction_id) DO UPDATE SET candidate_id=excluded.candidate_id, final_sha=excluded.final_sha`,
		transactionID, candidateID, finalSHA, utcText(s.now()))
	return err
}
