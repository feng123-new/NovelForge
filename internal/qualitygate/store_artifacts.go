package qualitygate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

func (s *Store) SaveCandidate(ctx context.Context, transactionID, text, sourceVersion string, attempt int) (Candidate, error) {
	if strings.TrimSpace(text) == "" || strings.TrimSpace(sourceVersion) == "" {
		return Candidate{}, errors.New("draft text and source version are required")
	}
	id, err := s.newID("cand_")
	if err != nil {
		return Candidate{}, err
	}
	tx, err := s.Transaction(ctx, transactionID)
	if err != nil {
		return Candidate{}, err
	}
	now := s.now().UTC()
	sha := HashText(text)
	_, err = s.db.ExecContext(ctx, `INSERT INTO chapter_candidates(id, transaction_id, chapter, attempt, text_content, text_sha, source_version, created_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, id, transactionID, tx.Chapter, attempt, text, sha, sourceVersion, utcText(now))
	if err != nil {
		var existingID string
		if queryErr := s.db.QueryRowContext(ctx, `SELECT id FROM chapter_candidates WHERE transaction_id=? AND attempt=?`, transactionID, attempt).Scan(&existingID); queryErr == nil {
			return s.Candidate(ctx, existingID)
		}
		return Candidate{}, err
	}
	return s.Candidate(ctx, id)
}

func (s *Store) Candidate(ctx context.Context, id string) (Candidate, error) {
	return scanCandidate(s.db.QueryRowContext(ctx, `SELECT id, transaction_id, chapter, attempt, text_content, text_sha, source_version,
		continuity_status, editor_score, selected, selection_reason, created_at FROM chapter_candidates WHERE id=?`, id))
}

func scanCandidate(row rowScanner) (Candidate, error) {
	var c Candidate
	var status, createdAt string
	var score sql.NullFloat64
	var selected int
	if err := row.Scan(&c.ID, &c.TransactionID, &c.Chapter, &c.Attempt, &c.Text, &c.TextSHA, &c.SourceVersion,
		&status, &score, &selected, &c.SelectionReason, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Candidate{}, ErrNotFound
		}
		return Candidate{}, err
	}
	c.ContinuityStatus = ContinuityStatus(status)
	if score.Valid {
		value := score.Float64
		c.EditorScore = &value
	}
	c.Selected = selected != 0
	c.CreatedAt = parseUTC(createdAt)
	return c, nil
}

func (s *Store) Candidates(ctx context.Context, transactionID string) ([]Candidate, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, transaction_id, chapter, attempt, text_content, text_sha, source_version,
		continuity_status, editor_score, selected, selection_reason, created_at FROM chapter_candidates WHERE transaction_id=? ORDER BY attempt ASC, id ASC`, transactionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Candidate{}
	for rows.Next() {
		candidate, err := scanCandidate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, candidate)
	}
	return out, rows.Err()
}

func (s *Store) SaveProposal(ctx context.Context, transactionID, candidateID string, proposal FactProposal) error {
	if err := proposal.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(proposal)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO fact_proposals(proposal_id, transaction_id, candidate_id, payload_json, created_at)
		VALUES(?, ?, ?, ?, ?) ON CONFLICT(transaction_id, candidate_id) DO UPDATE SET payload_json=excluded.payload_json`,
		proposal.ProposalID, transactionID, candidateID, payload, utcText(s.now()))
	return err
}

func (s *Store) Proposal(ctx context.Context, transactionID, candidateID string) (FactProposal, error) {
	var payload []byte
	if err := s.db.QueryRowContext(ctx, `SELECT payload_json FROM fact_proposals WHERE transaction_id=? AND candidate_id=?`, transactionID, candidateID).Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FactProposal{}, ErrNotFound
		}
		return FactProposal{}, err
	}
	var proposal FactProposal
	if err := json.Unmarshal(payload, &proposal); err != nil {
		return FactProposal{}, err
	}
	return proposal, nil
}

func (s *Store) SaveContinuity(ctx context.Context, transactionID, candidateID string, result ContinuityResult) error {
	if err := result.Validate(); err != nil {
		return err
	}
	payload, _ := json.Marshal(result)
	blocking := 0
	if result.Blocking {
		blocking = 1
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO continuity_results(transaction_id, candidate_id, status, blocking, payload_json, created_at)
		VALUES(?, ?, ?, ?, ?, ?) ON CONFLICT(transaction_id, candidate_id) DO UPDATE SET status=excluded.status, blocking=excluded.blocking, payload_json=excluded.payload_json`,
		transactionID, candidateID, result.Status, blocking, payload, utcText(s.now())); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chapter_candidates SET continuity_status=? WHERE id=? AND transaction_id=?`, result.Status, candidateID, transactionID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Continuity(ctx context.Context, transactionID, candidateID string) (ContinuityResult, error) {
	var payload []byte
	if err := s.db.QueryRowContext(ctx, `SELECT payload_json FROM continuity_results WHERE transaction_id=? AND candidate_id=?`, transactionID, candidateID).Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ContinuityResult{}, ErrNotFound
		}
		return ContinuityResult{}, err
	}
	var result ContinuityResult
	return result, json.Unmarshal(payload, &result)
}

func (s *Store) SaveEditor(ctx context.Context, transactionID, candidateID string, review EditorReview) error {
	if err := review.Validate(); err != nil {
		return err
	}
	payload, _ := json.Marshal(review)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO editor_reviews(transaction_id, candidate_id, score, payload_json, created_at)
		VALUES(?, ?, ?, ?, ?) ON CONFLICT(transaction_id, candidate_id) DO UPDATE SET score=excluded.score, payload_json=excluded.payload_json`,
		transactionID, candidateID, review.Score, payload, utcText(s.now())); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chapter_candidates SET editor_score=? WHERE id=? AND transaction_id=?`, review.Score, candidateID, transactionID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Editor(ctx context.Context, transactionID, candidateID string) (EditorReview, error) {
	var payload []byte
	if err := s.db.QueryRowContext(ctx, `SELECT payload_json FROM editor_reviews WHERE transaction_id=? AND candidate_id=?`, transactionID, candidateID).Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EditorReview{}, ErrNotFound
		}
		return EditorReview{}, err
	}
	var review EditorReview
	return review, json.Unmarshal(payload, &review)
}
