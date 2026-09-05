package qualitygate

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
)

var (
	ErrNotFound            = errors.New("quality transaction not found")
	ErrIdempotencyConflict = errors.New("model-call idempotency conflict")
	ErrRewriteLimit        = errors.New("rewrite limit reached")
	ErrNoSafeCandidate     = errors.New("no continuity-safe candidate")
)

type StoreOption func(*Store)

func WithClock(now func() time.Time) StoreOption {
	return func(s *Store) {
		if now != nil {
			s.now = now
		}
	}
}

func WithRandom(reader io.Reader) StoreOption {
	return func(s *Store) {
		if reader != nil {
			s.random = reader
		}
	}
}

type Store struct {
	db     *sql.DB
	now    func() time.Time
	random io.Reader
	mu     sync.Mutex
}

func OpenExisting(path string, busyTimeout time.Duration, options ...StoreOption) (*Store, error) {
	db, err := migrate.Open(path, busyTimeout)
	if err != nil {
		return nil, fmt.Errorf("open quality database: %w", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='chapter_transactions'`).Scan(&count); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("inspect quality schema: %w", err)
	}
	if count != 1 {
		_ = db.Close()
		return nil, errors.New("quality schema is not initialized")
	}
	s := &Store{db: db, now: time.Now, random: rand.Reader}
	for _, option := range options {
		option(s)
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func utcText(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseUTC(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed.UTC()
}

func (s *Store) newID(prefix string) (string, error) {
	buf := make([]byte, 12)
	if _, err := io.ReadFull(s.random, buf); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(buf), nil
}

func HashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

type Policy struct {
	MaxRewrites      int     `json:"max_rewrites"`
	QualityThreshold float64 `json:"quality_threshold"`
	AllowWarn        bool    `json:"allow_warn"`
}

func DefaultPolicy() Policy {
	return Policy{MaxRewrites: 2, QualityThreshold: 7.0, AllowWarn: true}
}

func (p Policy) Validate() error {
	if p.MaxRewrites < 0 || p.MaxRewrites > 10 {
		return errors.New("max_rewrites must be between 0 and 10")
	}
	if p.QualityThreshold < 0 || p.QualityThreshold > 10 {
		return errors.New("quality_threshold must be between 0 and 10")
	}
	return nil
}

func (p Policy) Allows(result ContinuityResult) bool {
	if result.Status == ContinuityFail || result.Blocking {
		return false
	}
	return result.Status == ContinuityPass || p.AllowWarn
}

func (s *Store) Begin(ctx context.Context, projectID string, chapter int, policy Policy) (Transaction, bool, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" || chapter <= 0 {
		return Transaction{}, false, errors.New("project and positive chapter are required")
	}
	if err := policy.Validate(); err != nil {
		return Transaction{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, err := s.transactionByProjectChapter(ctx, projectID, chapter); err == nil {
		return existing, true, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Transaction{}, false, err
	}
	id, err := s.newID("qtx_")
	if err != nil {
		return Transaction{}, false, err
	}
	now := s.now().UTC()
	_, err = s.db.ExecContext(ctx, `INSERT INTO chapter_transactions(
		id, project_id, chapter, state, attempt, max_rewrites, quality_threshold,
		created_at, updated_at) VALUES(?, ?, ?, ?, 0, ?, ?, ?, ?)`,
		id, projectID, chapter, StatePlanned, policy.MaxRewrites, policy.QualityThreshold, utcText(now), utcText(now))
	if err != nil {
		return Transaction{}, false, fmt.Errorf("create chapter transaction: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO chapter_state_changes(
		transaction_id, chapter, from_state, to_state, reason, actor, attempt, created_at)
		VALUES(?, ?, '', ?, 'transaction created', 'coordinator', 0, ?)`, id, chapter, StatePlanned, utcText(now)); err != nil {
		return Transaction{}, false, fmt.Errorf("record initial state: %w", err)
	}
	created, err := s.Transaction(ctx, id)
	return created, false, err
}

func (s *Store) transactionByProjectChapter(ctx context.Context, projectID string, chapter int) (Transaction, error) {
	return scanTransaction(s.db.QueryRowContext(ctx, `SELECT id, project_id, chapter, state, attempt,
		max_rewrites, quality_threshold, final_candidate_id, hold_reason, last_reason, created_at, updated_at
		FROM chapter_transactions WHERE project_id=? AND chapter=?`, projectID, chapter))
}

func (s *Store) Transaction(ctx context.Context, id string) (Transaction, error) {
	return scanTransaction(s.db.QueryRowContext(ctx, `SELECT id, project_id, chapter, state, attempt,
		max_rewrites, quality_threshold, final_candidate_id, hold_reason, last_reason, created_at, updated_at
		FROM chapter_transactions WHERE id=?`, id))
}

type rowScanner interface{ Scan(...any) error }

func scanTransaction(row rowScanner) (Transaction, error) {
	var tx Transaction
	var state, createdAt, updatedAt string
	if err := row.Scan(&tx.ID, &tx.ProjectID, &tx.Chapter, &state, &tx.Attempt,
		&tx.MaxRewrites, &tx.QualityThreshold, &tx.FinalCandidateID, &tx.HoldReason,
		&tx.LastReason, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Transaction{}, ErrNotFound
		}
		return Transaction{}, err
	}
	tx.State = TransactionState(state)
	tx.CreatedAt, tx.UpdatedAt = parseUTC(createdAt), parseUTC(updatedAt)
	return tx, nil
}

func (s *Store) Transition(ctx context.Context, transactionID string, to TransactionState, reason, actor string, attempt int) (Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Transaction{}, err
	}
	defer tx.Rollback()
	current, err := scanTransaction(tx.QueryRowContext(ctx, `SELECT id, project_id, chapter, state, attempt,
		max_rewrites, quality_threshold, final_candidate_id, hold_reason, last_reason, created_at, updated_at
		FROM chapter_transactions WHERE id=?`, transactionID))
	if err != nil {
		return Transaction{}, err
	}
	if err := ValidateTransition(current.State, to); err != nil {
		return Transaction{}, err
	}
	if attempt < current.Attempt {
		return Transaction{}, errors.New("attempt cannot move backwards")
	}
	if attempt > current.MaxRewrites && to == StateDrafting {
		return Transaction{}, ErrRewriteLimit
	}
	now := s.now().UTC()
	holdReason := current.HoldReason
	if to == StateHold {
		holdReason = strings.TrimSpace(reason)
	} else if current.State == StateHold {
		holdReason = ""
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chapter_transactions SET state=?, attempt=?, hold_reason=?, last_reason=?, updated_at=? WHERE id=?`,
		to, attempt, holdReason, strings.TrimSpace(reason), utcText(now), transactionID); err != nil {
		return Transaction{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO chapter_state_changes(transaction_id, chapter, from_state, to_state, reason, actor, attempt, created_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, transactionID, current.Chapter, current.State, to, strings.TrimSpace(reason), strings.TrimSpace(actor), attempt, utcText(now)); err != nil {
		return Transaction{}, err
	}
	if err := tx.Commit(); err != nil {
		return Transaction{}, err
	}
	return s.Transaction(ctx, transactionID)
}

// Database is borrowed by project-scoped observation recording; Store owns Close.
func (s *Store) Database() *sql.DB { return s.db }
