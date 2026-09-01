package eventstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/voocel/ainovel-cli/internal/server/repository"
)

const timestampFormat = "2006-01-02T15:04:05.000000000Z07:00"

// Record is the durable event representation shared with the SSE adapter.
type Record struct {
	ID      uint64
	Type    string
	Project string
	Time    time.Time
	Data    json.RawMessage
}

// Repository persists and replays ordered events.
type Repository interface {
	Append(ctx context.Context, eventType string, projectID string, data any) (Record, error)
	Replay(ctx context.Context, afterID uint64, projectID string, limit int) ([]Record, error)
}

// SQLiteRepository stores events in the workspace control database.
type SQLiteRepository struct {
	DatabasePath string
	Now          func() time.Time
}

func (r SQLiteRepository) Append(
	ctx context.Context,
	eventType string,
	projectID string,
	data any,
) (Record, error) {
	if eventType == "" {
		return Record{}, errors.New("event type is required")
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return Record{}, fmt.Errorf("encode event payload: %w", err)
	}
	now := r.now()
	db, err := repository.OpenControl(r.DatabasePath)
	if err != nil {
		return Record{}, err
	}
	defer db.Close()
	result, err := db.ExecContext(
		ctx,
		`INSERT INTO events(event_type, project_id, payload_json, created_at)
		 VALUES (?, ?, ?, ?)`,
		eventType,
		projectID,
		payload,
		now.Format(timestampFormat),
	)
	if err != nil {
		return Record{}, fmt.Errorf("append event: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Record{}, fmt.Errorf("read event id: %w", err)
	}
	return Record{
		ID:      uint64(id),
		Type:    eventType,
		Project: projectID,
		Time:    now,
		Data:    append(json.RawMessage(nil), payload...),
	}, nil
}

func (r SQLiteRepository) Replay(
	ctx context.Context,
	afterID uint64,
	projectID string,
	limit int,
) ([]Record, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	db, err := repository.OpenControl(r.DatabasePath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := `SELECT id, event_type, project_id, payload_json, created_at
		FROM events
		WHERE id > ?`
	args := []any{afterID}
	if projectID != "" {
		query += ` AND project_id = ?`
		args = append(args, projectID)
	}
	query += ` ORDER BY id ASC LIMIT ?`
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("replay events: %w", err)
	}
	defer rows.Close()
	records := make([]Record, 0)
	for rows.Next() {
		var (
			id        int64
			record    Record
			payload   []byte
			createdAt string
		)
		if err := rows.Scan(
			&id,
			&record.Type,
			&record.Project,
			&payload,
			&createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if id < 0 {
			return nil, errors.New("event id is invalid")
		}
		record.ID = uint64(id)
		record.Data = append(json.RawMessage(nil), payload...)
		record.Time, err = time.Parse(timestampFormat, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse event time: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return records, nil
}

func (r SQLiteRepository) now() time.Time {
	if r.Now == nil {
		return time.Now().UTC()
	}
	return r.Now().UTC()
}
