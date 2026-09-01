package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/voocel/ainovel-cli/internal/server/repository"
)

const timestampFormat = "2006-01-02T15:04:05.000000000Z07:00"

var (
	ErrConflict   = errors.New("idempotency key is already bound to a different request")
	ErrInProgress = errors.New("idempotent request is already in progress")
)

// Store persists write request outcomes in the workspace control database.
type Store struct {
	DatabasePath string
	TTL          time.Duration
	Now          func() time.Time
}

// BeginResult describes whether a request should execute or replay.
type BeginResult struct {
	Execute        bool
	ResponseStatus int
	ResponseBody   []byte
}

// RequestHash binds an idempotency key to method, path, query and exact body.
func RequestHash(method, requestURI string, body []byte) string {
	sum := sha256.New()
	_, _ = sum.Write([]byte(method))
	_, _ = sum.Write([]byte{0})
	_, _ = sum.Write([]byte(requestURI))
	_, _ = sum.Write([]byte{0})
	_, _ = sum.Write(body)
	return hex.EncodeToString(sum.Sum(nil))
}

// Begin reserves a key or returns the previously completed response.
func (s Store) Begin(
	ctx context.Context,
	key string,
	operation string,
	projectID string,
	requestHash string,
) (BeginResult, error) {
	now := s.now()
	expires := now.Add(s.ttl())
	db, err := repository.OpenControl(s.DatabasePath)
	if err != nil {
		return BeginResult{}, err
	}
	defer db.Close()

	if _, err := db.ExecContext(
		ctx,
		`DELETE FROM idempotency_records WHERE expires_at <= ?`,
		formatTime(now),
	); err != nil {
		return BeginResult{}, fmt.Errorf("expire idempotency records: %w", err)
	}

	result, err := db.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO idempotency_records(
			key, operation, project_id, request_hash, status, created_at, expires_at
		) VALUES (?, ?, ?, ?, 'in_progress', ?, ?)`,
		key,
		operation,
		projectID,
		requestHash,
		formatTime(now),
		formatTime(expires),
	)
	if err != nil {
		return BeginResult{}, fmt.Errorf("reserve idempotency key: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return BeginResult{}, fmt.Errorf("read idempotency reservation: %w", err)
	}
	if affected == 1 {
		return BeginResult{Execute: true}, nil
	}

	var (
		existingOperation string
		existingProject   string
		existingHash      string
		status            string
		responseStatus    int
		responseBody      []byte
	)
	if err := db.QueryRowContext(
		ctx,
		`SELECT operation, project_id, request_hash, status,
		        COALESCE(response_status, 0), COALESCE(response_body, X'')
		   FROM idempotency_records
		  WHERE key = ?`,
		key,
	).Scan(
		&existingOperation,
		&existingProject,
		&existingHash,
		&status,
		&responseStatus,
		&responseBody,
	); err != nil {
		return BeginResult{}, fmt.Errorf("read idempotency record: %w", err)
	}
	if existingOperation != operation || existingProject != projectID || existingHash != requestHash {
		return BeginResult{}, ErrConflict
	}
	if status != "completed" {
		return BeginResult{}, ErrInProgress
	}
	return BeginResult{
		Execute:        false,
		ResponseStatus: responseStatus,
		ResponseBody:   append([]byte(nil), responseBody...),
	}, nil
}

// Complete atomically records the exact HTTP response for future replay.
func (s Store) Complete(
	ctx context.Context,
	key string,
	requestHash string,
	responseStatus int,
	responseBody []byte,
) error {
	db, err := repository.OpenControl(s.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := db.ExecContext(
		ctx,
		`UPDATE idempotency_records
		    SET status = 'completed', response_status = ?, response_body = ?
		  WHERE key = ? AND request_hash = ? AND status = 'in_progress'`,
		responseStatus,
		responseBody,
		key,
		requestHash,
	)
	if err != nil {
		return fmt.Errorf("complete idempotency record: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read idempotency completion: %w", err)
	}
	if affected != 1 {
		return ErrConflict
	}
	return nil
}

// Abandon removes an uncommitted reservation. It is used only when the request
// could not have caused a side effect.
func (s Store) Abandon(ctx context.Context, key string, requestHash string) error {
	db, err := repository.OpenControl(s.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(
		ctx,
		`DELETE FROM idempotency_records
		  WHERE key = ? AND request_hash = ? AND status = 'in_progress'`,
		key,
		requestHash,
	)
	if err != nil {
		return fmt.Errorf("abandon idempotency record: %w", err)
	}
	return nil
}

func (s Store) ttl() time.Duration {
	if s.TTL <= 0 {
		return 24 * time.Hour
	}
	return s.TTL
}

func (s Store) now() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}

func formatTime(value time.Time) string {
	return value.UTC().Format(timestampFormat)
}
