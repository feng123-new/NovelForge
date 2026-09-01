#!/usr/bin/env python3
from __future__ import annotations

import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def write(path: str, content: str) -> None:
    target = ROOT / path
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(content.rstrip() + "\n", encoding="utf-8")


write("internal/truth/types.go", r'''package truth

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrValidation          = errors.New("truth validation failed")
	ErrSchemaUnavailable   = errors.New("truth schema unavailable")
	ErrIdempotencyConflict = errors.New("truth idempotency conflict")
	ErrNotFound            = errors.New("truth record not found")
	ErrInvalidSupersede    = errors.New("invalid truth supersede")
)

type Error struct {
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Cause }

func newError(code, message string, cause error) error {
	return &Error{Code: code, Message: message, Cause: cause}
}

type Operation string

const (
	OperationAssert  Operation = "assert"
	OperationRetract Operation = "retract"
)

type Authority int

const (
	AuthorityImported  Authority = 10
	AuthorityAI        Authority = 20
	AuthorityHuman     Authority = 80
	AuthorityCanonical Authority = 100
)

type EventInput struct {
	EventID            string          `json:"event_id,omitempty"`
	IdempotencyKey      string          `json:"-"`
	EntityType          string          `json:"entity_type"`
	EntityID            string          `json:"entity_id"`
	Attribute           string          `json:"attribute"`
	Operation           Operation       `json:"operation"`
	Value               json.RawMessage `json:"value"`
	EffectiveChapter    int             `json:"effective_chapter"`
	SourceKind          string          `json:"source_kind"`
	SourceID            string          `json:"source_id"`
	SourceChapter       int             `json:"source_chapter"`
	Authority           Authority       `json:"authority"`
	Confidence          float64         `json:"confidence"`
	SupersedesEventID   string          `json:"supersedes_event_id,omitempty"`
	RecordedAt          time.Time       `json:"-"`
}

type Event struct {
	Sequence            int64           `json:"sequence"`
	EventID             string          `json:"event_id"`
	IdempotencyKey      string          `json:"idempotency_key"`
	PayloadHash         string          `json:"payload_hash"`
	EntityType          string          `json:"entity_type"`
	EntityID            string          `json:"entity_id"`
	Attribute           string          `json:"attribute"`
	Operation           Operation       `json:"operation"`
	Value               json.RawMessage `json:"value"`
	ValueHash           string          `json:"value_hash"`
	EffectiveChapter    int             `json:"effective_chapter"`
	SourceKind          string          `json:"source_kind"`
	SourceID            string          `json:"source_id"`
	SourceChapter       int             `json:"source_chapter"`
	Authority           Authority       `json:"authority"`
	Confidence          float64         `json:"confidence"`
	SupersedesEventID   string          `json:"supersedes_event_id,omitempty"`
	SupersededByEventID string          `json:"superseded_by_event_id,omitempty"`
	RecordedAt          time.Time       `json:"recorded_at"`
}

type Fact struct {
	EventID          string          `json:"event_id"`
	EntityType       string          `json:"entity_type"`
	EntityID         string          `json:"entity_id"`
	Attribute        string          `json:"attribute"`
	Value            json.RawMessage `json:"value"`
	ValidFromChapter int             `json:"valid_from_chapter"`
	ValidToChapter   *int            `json:"valid_to_chapter,omitempty"`
	SourceKind       string          `json:"source_kind"`
	SourceID         string          `json:"source_id"`
	SourceChapter    int             `json:"source_chapter"`
	Authority        Authority       `json:"authority"`
	Confidence       float64         `json:"confidence"`
}

type Conflict struct {
	ConflictID        string    `json:"conflict_id"`
	EntityType        string    `json:"entity_type"`
	EntityID          string    `json:"entity_id"`
	Attribute         string    `json:"attribute"`
	Chapter           int       `json:"chapter"`
	WinningEventID    string    `json:"winning_event_id"`
	ConflictingEventID string   `json:"conflicting_event_id"`
	Reason            string    `json:"reason"`
	CreatedAt         time.Time `json:"created_at"`
}

type AppendResult struct {
	Event     Event      `json:"event"`
	Replayed  bool       `json:"replayed"`
	Conflicts []Conflict `json:"conflicts"`
}

type Query struct {
	Chapter    int
	EntityType string
	EntityID   string
	Attribute  string
	Limit      int
	Offset     int
}

type FactPage struct {
	Facts      []Fact `json:"facts"`
	Total      int    `json:"total"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
	NextOffset *int   `json:"next_offset,omitempty"`
}

type EventQuery struct {
	ThroughChapter *int
	EntityType      string
	EntityID        string
	Attribute       string
	Limit           int
	Offset          int
}

type EventPage struct {
	Events     []Event `json:"events"`
	Total      int     `json:"total"`
	Limit      int     `json:"limit"`
	Offset     int     `json:"offset"`
	NextOffset *int    `json:"next_offset,omitempty"`
}

type ConflictPage struct {
	Conflicts  []Conflict `json:"conflicts"`
	Total      int        `json:"total"`
	Limit      int        `json:"limit"`
	Offset     int        `json:"offset"`
	NextOffset *int      `json:"next_offset,omitempty"`
}

type RebuildResult struct {
	FromChapter int       `json:"from_chapter"`
	KeysRebuilt int       `json:"keys_rebuilt"`
	CompletedAt time.Time `json:"completed_at"`
}

func errorCode(err error) string {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Code
	}
	return "TRUTH_INTERNAL_ERROR"
}

func formatKey(entityType, entityID, attribute string) string {
	return fmt.Sprintf("%s/%s/%s", entityType, entityID, attribute)
}
''')

write("internal/truth/schema.go", r'''package truth

import "github.com/voocel/ainovel-cli/internal/db/migrate"

var ProjectMigration = migrate.Migration{
	Version: 2,
	Name:    "structured_temporal_truth_store",
	SQL: `CREATE TABLE truth_events (
		sequence INTEGER PRIMARY KEY AUTOINCREMENT,
		event_id TEXT NOT NULL UNIQUE,
		idempotency_key TEXT NOT NULL UNIQUE,
		payload_hash TEXT NOT NULL,
		entity_type TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		attribute TEXT NOT NULL,
		operation TEXT NOT NULL CHECK(operation IN ('assert', 'retract')),
		value_json TEXT NOT NULL,
		value_hash TEXT NOT NULL,
		effective_chapter INTEGER NOT NULL CHECK(effective_chapter >= 0),
		source_kind TEXT NOT NULL,
		source_id TEXT NOT NULL,
		source_chapter INTEGER NOT NULL CHECK(source_chapter >= 0),
		authority INTEGER NOT NULL CHECK(authority >= 0 AND authority <= 100),
		confidence REAL NOT NULL CHECK(confidence >= 0 AND confidence <= 1),
		supersedes_event_id TEXT,
		superseded_by_event_id TEXT,
		recorded_at TEXT NOT NULL,
		FOREIGN KEY(supersedes_event_id) REFERENCES truth_events(event_id),
		FOREIGN KEY(superseded_by_event_id) REFERENCES truth_events(event_id)
	);
	CREATE INDEX truth_events_temporal_idx
		ON truth_events(entity_type, entity_id, attribute, effective_chapter, sequence);
	CREATE INDEX truth_events_source_idx
		ON truth_events(source_kind, source_id, source_chapter);
	CREATE INDEX truth_events_active_idx
		ON truth_events(entity_type, entity_id, attribute, superseded_by_event_id, effective_chapter);

	CREATE TABLE truth_projection (
		event_id TEXT PRIMARY KEY,
		entity_type TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		attribute TEXT NOT NULL,
		value_json TEXT NOT NULL,
		value_hash TEXT NOT NULL,
		valid_from_chapter INTEGER NOT NULL,
		valid_to_chapter INTEGER,
		source_kind TEXT NOT NULL,
		source_id TEXT NOT NULL,
		source_chapter INTEGER NOT NULL,
		authority INTEGER NOT NULL,
		confidence REAL NOT NULL,
		FOREIGN KEY(event_id) REFERENCES truth_events(event_id) ON DELETE CASCADE,
		CHECK(valid_to_chapter IS NULL OR valid_to_chapter >= valid_from_chapter)
	);
	CREATE INDEX truth_projection_asof_idx
		ON truth_projection(valid_from_chapter, valid_to_chapter, entity_type, entity_id, attribute);
	CREATE INDEX truth_projection_key_idx
		ON truth_projection(entity_type, entity_id, attribute, valid_from_chapter, valid_to_chapter);

	CREATE TABLE truth_conflicts (
		conflict_id TEXT PRIMARY KEY,
		entity_type TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		attribute TEXT NOT NULL,
		chapter INTEGER NOT NULL,
		winning_event_id TEXT NOT NULL,
		conflicting_event_id TEXT NOT NULL,
		reason TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(winning_event_id) REFERENCES truth_events(event_id) ON DELETE CASCADE,
		FOREIGN KEY(conflicting_event_id) REFERENCES truth_events(event_id) ON DELETE CASCADE,
		UNIQUE(winning_event_id, conflicting_event_id)
	);
	CREATE INDEX truth_conflicts_key_idx
		ON truth_conflicts(entity_type, entity_id, attribute, chapter);

	CREATE TABLE truth_rebuilds (
		rebuild_id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_chapter INTEGER NOT NULL,
		keys_rebuilt INTEGER NOT NULL,
		completed_at TEXT NOT NULL
	);`,
}
''')

write("internal/truth/store.go", r'''package truth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
)

type Store struct {
	db  *sql.DB
	now func() time.Time
}

func Open(ctx context.Context, databasePath string) (*Store, error) {
	db, err := migrate.Open(databasePath, 5*time.Second)
	if err != nil {
		return nil, newError("TRUTH_DATABASE_OPEN_FAILED", "truth database could not be opened", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='truth_events'`).Scan(&count); err != nil {
		_ = db.Close()
		return nil, newError("TRUTH_DATABASE_OPEN_FAILED", "truth schema could not be inspected", err)
	}
	if count != 1 {
		_ = db.Close()
		return nil, newError("TRUTH_SCHEMA_UNAVAILABLE", "truth schema is not initialized", ErrSchemaUnavailable)
	}
	return &Store{db: db, now: time.Now}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Append(ctx context.Context, input EventInput) (AppendResult, error) {
	normalized, payloadHash, err := normalizeInput(input, s.now)
	if err != nil {
		return AppendResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AppendResult{}, newError("TRUTH_WRITE_FAILED", "truth transaction could not start", err)
	}
	defer tx.Rollback()

	existing, found, err := eventByIdempotency(ctx, tx, normalized.IdempotencyKey)
	if err != nil {
		return AppendResult{}, err
	}
	if found {
		if existing.PayloadHash != payloadHash {
			return AppendResult{}, newError("TRUTH_IDEMPOTENCY_CONFLICT", "idempotency key was already used for a different truth event", ErrIdempotencyConflict)
		}
		conflicts, err := conflictsForEvent(ctx, tx, existing.EventID)
		if err != nil {
			return AppendResult{}, err
		}
		return AppendResult{Event: existing, Replayed: true, Conflicts: conflicts}, nil
	}

	if normalized.SupersedesEventID != "" {
		target, found, err := eventByID(ctx, tx, normalized.SupersedesEventID)
		if err != nil {
			return AppendResult{}, err
		}
		if !found || target.SupersededByEventID != "" || target.EntityType != normalized.EntityType || target.EntityID != normalized.EntityID || target.Attribute != normalized.Attribute {
			return AppendResult{}, newError("TRUTH_INVALID_SUPERSEDE", "superseded event must be active and use the same truth key", ErrInvalidSupersede)
		}
	}

	valueHash := sha256Hex(normalized.Value)
	result, err := tx.ExecContext(ctx, `INSERT INTO truth_events(
		event_id, idempotency_key, payload_hash, entity_type, entity_id, attribute,
		operation, value_json, value_hash, effective_chapter, source_kind, source_id,
		source_chapter, authority, confidence, supersedes_event_id, recorded_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)`,
		normalized.EventID, normalized.IdempotencyKey, payloadHash, normalized.EntityType,
		normalized.EntityID, normalized.Attribute, normalized.Operation, string(normalized.Value),
		valueHash, normalized.EffectiveChapter, normalized.SourceKind, normalized.SourceID,
		normalized.SourceChapter, normalized.Authority, normalized.Confidence,
		normalized.SupersedesEventID, normalized.RecordedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return AppendResult{}, newError("TRUTH_WRITE_FAILED", "truth event could not be appended", err)
	}
	sequence, _ := result.LastInsertId()
	if normalized.SupersedesEventID != "" {
		changed, err := tx.ExecContext(ctx, `UPDATE truth_events SET superseded_by_event_id=? WHERE event_id=? AND superseded_by_event_id IS NULL`, normalized.EventID, normalized.SupersedesEventID)
		if err != nil {
			return AppendResult{}, newError("TRUTH_WRITE_FAILED", "truth supersede could not be recorded", err)
		}
		rows, _ := changed.RowsAffected()
		if rows != 1 {
			return AppendResult{}, newError("TRUTH_INVALID_SUPERSEDE", "superseded event changed concurrently", ErrInvalidSupersede)
		}
	}
	if err := rebuildKey(ctx, tx, normalized.EntityType, normalized.EntityID, normalized.Attribute, s.now); err != nil {
		return AppendResult{}, err
	}
	event, found, err := eventByID(ctx, tx, normalized.EventID)
	if err != nil || !found {
		if err == nil {
			err = ErrNotFound
		}
		return AppendResult{}, newError("TRUTH_WRITE_FAILED", "appended truth event could not be read", err)
	}
	event.Sequence = sequence
	conflicts, err := conflictsForEvent(ctx, tx, event.EventID)
	if err != nil {
		return AppendResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AppendResult{}, newError("TRUTH_WRITE_FAILED", "truth transaction could not commit", err)
	}
	return AppendResult{Event: event, Conflicts: conflicts}, nil
}

func (s *Store) FactsAsOf(ctx context.Context, query Query) (FactPage, error) {
	if query.Chapter < 0 || query.Offset < 0 {
		return FactPage{}, newError("TRUTH_VALIDATION_FAILED", "chapter and offset must not be negative", ErrValidation)
	}
	query.Limit = normalizeLimit(query.Limit, 100, 1000)
	where := []string{"valid_from_chapter <= ?", "(valid_to_chapter IS NULL OR valid_to_chapter >= ?)"}
	args := []any{query.Chapter, query.Chapter}
	addFilter := func(column, value string) {
		if strings.TrimSpace(value) != "" {
			where = append(where, column+" = ?")
			args = append(args, strings.TrimSpace(value))
		}
	}
	addFilter("entity_type", query.EntityType)
	addFilter("entity_id", query.EntityID)
	addFilter("attribute", query.Attribute)
	clause := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM truth_projection WHERE `+clause, args...).Scan(&total); err != nil {
		return FactPage{}, newError("TRUTH_QUERY_FAILED", "truth facts could not be counted", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event_id, entity_type, entity_id, attribute, value_json,
		valid_from_chapter, valid_to_chapter, source_kind, source_id, source_chapter,
		authority, confidence FROM truth_projection WHERE `+clause+`
		ORDER BY entity_type, entity_id, attribute, valid_from_chapter, event_id LIMIT ? OFFSET ?`, append(args, query.Limit, query.Offset)...)
	if err != nil {
		return FactPage{}, newError("TRUTH_QUERY_FAILED", "truth facts could not be queried", err)
	}
	defer rows.Close()
	page := FactPage{Facts: []Fact{}, Total: total, Limit: query.Limit, Offset: query.Offset}
	for rows.Next() {
		var fact Fact
		var value string
		var validTo sql.NullInt64
		if err := rows.Scan(&fact.EventID, &fact.EntityType, &fact.EntityID, &fact.Attribute, &value,
			&fact.ValidFromChapter, &validTo, &fact.SourceKind, &fact.SourceID, &fact.SourceChapter,
			&fact.Authority, &fact.Confidence); err != nil {
			return FactPage{}, newError("TRUTH_QUERY_FAILED", "truth fact could not be decoded", err)
		}
		fact.Value = json.RawMessage(value)
		if validTo.Valid {
			v := int(validTo.Int64)
			fact.ValidToChapter = &v
		}
		page.Facts = append(page.Facts, fact)
	}
	if err := rows.Err(); err != nil {
		return FactPage{}, newError("TRUTH_QUERY_FAILED", "truth facts could not be iterated", err)
	}
	if query.Offset+len(page.Facts) < total {
		next := query.Offset + len(page.Facts)
		page.NextOffset = &next
	}
	return page, nil
}

func (s *Store) Events(ctx context.Context, query EventQuery) (EventPage, error) {
	if query.Offset < 0 || (query.ThroughChapter != nil && *query.ThroughChapter < 0) {
		return EventPage{}, newError("TRUTH_VALIDATION_FAILED", "event query bounds must not be negative", ErrValidation)
	}
	query.Limit = normalizeLimit(query.Limit, 100, 1000)
	where := []string{"1=1"}
	args := []any{}
	if query.ThroughChapter != nil {
		where = append(where, "effective_chapter <= ?")
		args = append(args, *query.ThroughChapter)
	}
	for _, item := range []struct{ column, value string }{{"entity_type", query.EntityType}, {"entity_id", query.EntityID}, {"attribute", query.Attribute}} {
		if strings.TrimSpace(item.value) != "" {
			where = append(where, item.column+" = ?")
			args = append(args, strings.TrimSpace(item.value))
		}
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM truth_events WHERE `+clause, args...).Scan(&total); err != nil {
		return EventPage{}, newError("TRUTH_QUERY_FAILED", "truth events could not be counted", err)
	}
	rows, err := s.db.QueryContext(ctx, eventSelect+` WHERE `+clause+` ORDER BY sequence LIMIT ? OFFSET ?`, append(args, query.Limit, query.Offset)...)
	if err != nil {
		return EventPage{}, newError("TRUTH_QUERY_FAILED", "truth events could not be queried", err)
	}
	defer rows.Close()
	page := EventPage{Events: []Event{}, Total: total, Limit: query.Limit, Offset: query.Offset}
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return EventPage{}, err
		}
		page.Events = append(page.Events, event)
	}
	if err := rows.Err(); err != nil {
		return EventPage{}, newError("TRUTH_QUERY_FAILED", "truth events could not be iterated", err)
	}
	if query.Offset+len(page.Events) < total {
		next := query.Offset + len(page.Events)
		page.NextOffset = &next
	}
	return page, nil
}

func (s *Store) Conflicts(ctx context.Context, limit, offset int) (ConflictPage, error) {
	if offset < 0 {
		return ConflictPage{}, newError("TRUTH_VALIDATION_FAILED", "offset must not be negative", ErrValidation)
	}
	limit = normalizeLimit(limit, 100, 1000)
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM truth_conflicts`).Scan(&total); err != nil {
		return ConflictPage{}, newError("TRUTH_QUERY_FAILED", "truth conflicts could not be counted", err)
	}
	rows, err := s.db.QueryContext(ctx, conflictSelect+` ORDER BY chapter, entity_type, entity_id, attribute, conflict_id LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return ConflictPage{}, newError("TRUTH_QUERY_FAILED", "truth conflicts could not be queried", err)
	}
	defer rows.Close()
	page := ConflictPage{Conflicts: []Conflict{}, Total: total, Limit: limit, Offset: offset}
	for rows.Next() {
		conflict, err := scanConflict(rows)
		if err != nil {
			return ConflictPage{}, err
		}
		page.Conflicts = append(page.Conflicts, conflict)
	}
	if err := rows.Err(); err != nil {
		return ConflictPage{}, newError("TRUTH_QUERY_FAILED", "truth conflicts could not be iterated", err)
	}
	if offset+len(page.Conflicts) < total {
		next := offset + len(page.Conflicts)
		page.NextOffset = &next
	}
	return page, nil
}

func (s *Store) RebuildFrom(ctx context.Context, fromChapter int) (RebuildResult, error) {
	if fromChapter < 0 {
		return RebuildResult{}, newError("TRUTH_VALIDATION_FAILED", "from_chapter must not be negative", ErrValidation)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RebuildResult{}, newError("TRUTH_REBUILD_FAILED", "truth rebuild transaction could not start", err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT entity_type, entity_id, attribute FROM truth_events
		WHERE effective_chapter >= ? OR (entity_type, entity_id, attribute) IN (
			SELECT entity_type, entity_id, attribute FROM truth_projection
			WHERE valid_to_chapter IS NULL OR valid_to_chapter >= ?
		) ORDER BY entity_type, entity_id, attribute`, fromChapter, fromChapter)
	if err != nil {
		return RebuildResult{}, newError("TRUTH_REBUILD_FAILED", "truth rebuild keys could not be selected", err)
	}
	type key struct{ entityType, entityID, attribute string }
	keys := []key{}
	for rows.Next() {
		var k key
		if err := rows.Scan(&k.entityType, &k.entityID, &k.attribute); err != nil {
			rows.Close()
			return RebuildResult{}, newError("TRUTH_REBUILD_FAILED", "truth rebuild key could not be decoded", err)
		}
		keys = append(keys, k)
	}
	if err := rows.Close(); err != nil {
		return RebuildResult{}, newError("TRUTH_REBUILD_FAILED", "truth rebuild key cursor could not close", err)
	}
	for _, k := range keys {
		if err := rebuildKey(ctx, tx, k.entityType, k.entityID, k.attribute, s.now); err != nil {
			return RebuildResult{}, err
		}
	}
	completed := s.now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO truth_rebuilds(from_chapter, keys_rebuilt, completed_at) VALUES (?, ?, ?)`, fromChapter, len(keys), completed.Format(time.RFC3339Nano)); err != nil {
		return RebuildResult{}, newError("TRUTH_REBUILD_FAILED", "truth rebuild audit record could not be written", err)
	}
	if err := tx.Commit(); err != nil {
		return RebuildResult{}, newError("TRUTH_REBUILD_FAILED", "truth rebuild could not commit", err)
	}
	return RebuildResult{FromChapter: fromChapter, KeysRebuilt: len(keys), CompletedAt: completed}, nil
}

const eventSelect = `SELECT sequence, event_id, idempotency_key, payload_hash, entity_type, entity_id,
	attribute, operation, value_json, value_hash, effective_chapter, source_kind, source_id,
	source_chapter, authority, confidence, COALESCE(supersedes_event_id,''),
	COALESCE(superseded_by_event_id,''), recorded_at FROM truth_events`

const conflictSelect = `SELECT conflict_id, entity_type, entity_id, attribute, chapter,
	winning_event_id, conflicting_event_id, reason, created_at FROM truth_conflicts`

type rowScanner interface{ Scan(...any) error }

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func scanEvent(row rowScanner) (Event, error) {
	var event Event
	var value, recorded string
	if err := row.Scan(&event.Sequence, &event.EventID, &event.IdempotencyKey, &event.PayloadHash,
		&event.EntityType, &event.EntityID, &event.Attribute, &event.Operation, &value,
		&event.ValueHash, &event.EffectiveChapter, &event.SourceKind, &event.SourceID,
		&event.SourceChapter, &event.Authority, &event.Confidence, &event.SupersedesEventID,
		&event.SupersededByEventID, &recorded); err != nil {
		return Event{}, newError("TRUTH_QUERY_FAILED", "truth event could not be decoded", err)
	}
	event.Value = json.RawMessage(value)
	parsed, err := time.Parse(time.RFC3339Nano, recorded)
	if err != nil {
		return Event{}, newError("TRUTH_QUERY_FAILED", "truth event timestamp is invalid", err)
	}
	event.RecordedAt = parsed
	return event, nil
}

func eventByIdempotency(ctx context.Context, q queryer, key string) (Event, bool, error) {
	event, err := scanEvent(q.QueryRowContext(ctx, eventSelect+` WHERE idempotency_key=?`, key))
	if errors.Is(err, sql.ErrNoRows) || errors.Is(errors.Unwrap(err), sql.ErrNoRows) {
		return Event{}, false, nil
	}
	if err != nil {
		return Event{}, false, err
	}
	return event, true, nil
}

func eventByID(ctx context.Context, q queryer, id string) (Event, bool, error) {
	event, err := scanEvent(q.QueryRowContext(ctx, eventSelect+` WHERE event_id=?`, id))
	if errors.Is(err, sql.ErrNoRows) || errors.Is(errors.Unwrap(err), sql.ErrNoRows) {
		return Event{}, false, nil
	}
	if err != nil {
		return Event{}, false, err
	}
	return event, true, nil
}

func conflictsForEvent(ctx context.Context, q *sql.Tx, eventID string) ([]Conflict, error) {
	rows, err := q.QueryContext(ctx, conflictSelect+` WHERE winning_event_id=? OR conflicting_event_id=? ORDER BY conflict_id`, eventID, eventID)
	if err != nil {
		return nil, newError("TRUTH_QUERY_FAILED", "truth event conflicts could not be queried", err)
	}
	defer rows.Close()
	result := []Conflict{}
	for rows.Next() {
		conflict, err := scanConflict(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, conflict)
	}
	return result, rows.Err()
}

func scanConflict(row rowScanner) (Conflict, error) {
	var conflict Conflict
	var created string
	if err := row.Scan(&conflict.ConflictID, &conflict.EntityType, &conflict.EntityID,
		&conflict.Attribute, &conflict.Chapter, &conflict.WinningEventID,
		&conflict.ConflictingEventID, &conflict.Reason, &created); err != nil {
		return Conflict{}, newError("TRUTH_QUERY_FAILED", "truth conflict could not be decoded", err)
	}
	parsed, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Conflict{}, newError("TRUTH_QUERY_FAILED", "truth conflict timestamp is invalid", err)
	}
	conflict.CreatedAt = parsed
	return conflict, nil
}

type segment struct {
	event Event
	to    *int
}

func rebuildKey(ctx context.Context, tx *sql.Tx, entityType, entityID, attribute string, now func() time.Time) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM truth_projection WHERE entity_type=? AND entity_id=? AND attribute=?`, entityType, entityID, attribute); err != nil {
		return newError("TRUTH_REBUILD_FAILED", "truth projection could not be cleared", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM truth_conflicts WHERE entity_type=? AND entity_id=? AND attribute=?`, entityType, entityID, attribute); err != nil {
		return newError("TRUTH_REBUILD_FAILED", "truth conflicts could not be cleared", err)
	}
	rows, err := tx.QueryContext(ctx, eventSelect+` WHERE entity_type=? AND entity_id=? AND attribute=? AND superseded_by_event_id IS NULL ORDER BY effective_chapter, sequence`, entityType, entityID, attribute)
	if err != nil {
		return newError("TRUTH_REBUILD_FAILED", "truth events could not be loaded for projection", err)
	}
	events := []Event{}
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			rows.Close()
			return err
		}
		events = append(events, event)
	}
	if err := rows.Close(); err != nil {
		return newError("TRUTH_REBUILD_FAILED", "truth event cursor could not close", err)
	}

	segments := []segment{}
	conflicts := []Conflict{}
	var current *Event
	for index := 0; index < len(events); {
		end := index + 1
		for end < len(events) && events[end].EffectiveChapter == events[index].EffectiveChapter {
			end++
		}
		group := append([]Event(nil), events[index:end]...)
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].Authority != group[j].Authority {
				return group[i].Authority > group[j].Authority
			}
			return group[i].Sequence > group[j].Sequence
		})
		candidate := group[0]
		winner := current
		change := false
		if current == nil {
			if candidate.Operation == OperationAssert {
				winner = &candidate
				change = true
			}
		} else if candidate.Operation == OperationRetract {
			if candidate.SupersedesEventID == current.EventID || candidate.Authority >= current.Authority {
				winner = nil
				change = true
			} else {
				conflicts = append(conflicts, makeConflict(*current, candidate, "lower-authority retraction rejected", now()))
			}
		} else if candidate.ValueHash == current.ValueHash {
			if candidate.Authority >= current.Authority {
				winner = &candidate
				change = true
			}
		} else if candidate.SupersedesEventID == current.EventID || candidate.Authority >= current.Authority {
			winner = &candidate
			change = true
		} else {
			conflicts = append(conflicts, makeConflict(*current, candidate, "lower-authority assertion rejected", now()))
		}

		if change {
			if current != nil && len(segments) > 0 {
				to := candidate.EffectiveChapter - 1
				if to >= segments[len(segments)-1].event.EffectiveChapter {
					segments[len(segments)-1].to = &to
				} else {
					segments = segments[:len(segments)-1]
				}
			}
			current = winner
			if current != nil {
				segments = append(segments, segment{event: *current})
			}
		}
		effectiveWinner := current
		for _, competing := range group {
			if effectiveWinner == nil || competing.EventID == effectiveWinner.EventID || competing.ValueHash == effectiveWinner.ValueHash {
				continue
			}
			conflicts = append(conflicts, makeConflict(*effectiveWinner, competing, "competing assertion at the same temporal key", now()))
		}
		index = end
	}

	for _, projected := range segments {
		var validTo any
		if projected.to != nil {
			validTo = *projected.to
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO truth_projection(
			event_id, entity_type, entity_id, attribute, value_json, value_hash,
			valid_from_chapter, valid_to_chapter, source_kind, source_id, source_chapter,
			authority, confidence
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			projected.event.EventID, projected.event.EntityType, projected.event.EntityID,
			projected.event.Attribute, string(projected.event.Value), projected.event.ValueHash,
			projected.event.EffectiveChapter, validTo, projected.event.SourceKind,
			projected.event.SourceID, projected.event.SourceChapter, projected.event.Authority,
			projected.event.Confidence); err != nil {
			return newError("TRUTH_REBUILD_FAILED", "truth projection could not be written", err)
		}
	}
	seen := map[string]struct{}{}
	for _, conflict := range conflicts {
		if _, ok := seen[conflict.ConflictID]; ok {
			continue
		}
		seen[conflict.ConflictID] = struct{}{}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO truth_conflicts(
			conflict_id, entity_type, entity_id, attribute, chapter, winning_event_id,
			conflicting_event_id, reason, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, conflict.ConflictID, conflict.EntityType,
			conflict.EntityID, conflict.Attribute, conflict.Chapter, conflict.WinningEventID,
			conflict.ConflictingEventID, conflict.Reason, conflict.CreatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return newError("TRUTH_REBUILD_FAILED", "truth conflict could not be written", err)
		}
	}
	return nil
}

func makeConflict(winner, competing Event, reason string, at time.Time) Conflict {
	left, right := winner.EventID, competing.EventID
	if right < left {
		left, right = right, left
	}
	id := sha256Hex([]byte(formatKey(winner.EntityType, winner.EntityID, winner.Attribute) + "\x00" + left + "\x00" + right))
	return Conflict{ConflictID: id, EntityType: winner.EntityType, EntityID: winner.EntityID,
		Attribute: winner.Attribute, Chapter: competing.EffectiveChapter, WinningEventID: winner.EventID,
		ConflictingEventID: competing.EventID, Reason: reason, CreatedAt: at.UTC()}
}

func normalizeInput(input EventInput, now func() time.Time) (EventInput, string, error) {
	input.EntityType = strings.TrimSpace(input.EntityType)
	input.EntityID = strings.TrimSpace(input.EntityID)
	input.Attribute = strings.TrimSpace(input.Attribute)
	input.SourceKind = strings.TrimSpace(input.SourceKind)
	input.SourceID = strings.TrimSpace(input.SourceID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.SupersedesEventID = strings.TrimSpace(input.SupersedesEventID)
	for name, value := range map[string]string{"entity_type": input.EntityType, "entity_id": input.EntityID, "attribute": input.Attribute, "source_kind": input.SourceKind, "source_id": input.SourceID, "idempotency_key": input.IdempotencyKey} {
		if value == "" || len(value) > 240 {
			return EventInput{}, "", newError("TRUTH_VALIDATION_FAILED", name+" is required and must be at most 240 bytes", ErrValidation)
		}
	}
	if input.EffectiveChapter < 0 || input.SourceChapter < 0 || input.SourceChapter > input.EffectiveChapter {
		return EventInput{}, "", newError("TRUTH_VALIDATION_FAILED", "chapter provenance must not be negative or come from the future", ErrValidation)
	}
	if input.Operation == "" {
		input.Operation = OperationAssert
	}
	if input.Operation != OperationAssert && input.Operation != OperationRetract {
		return EventInput{}, "", newError("TRUTH_VALIDATION_FAILED", "operation must be assert or retract", ErrValidation)
	}
	if input.Authority == 0 {
		input.Authority = AuthorityAI
	}
	if input.Authority < 0 || input.Authority > 100 {
		return EventInput{}, "", newError("TRUTH_VALIDATION_FAILED", "authority must be between 0 and 100", ErrValidation)
	}
	if input.Confidence == 0 {
		input.Confidence = 1
	}
	if input.Confidence < 0 || input.Confidence > 1 {
		return EventInput{}, "", newError("TRUTH_VALIDATION_FAILED", "confidence must be between 0 and 1", ErrValidation)
	}
	if input.Operation == OperationRetract && len(input.Value) == 0 {
		input.Value = json.RawMessage("null")
	}
	var decoded any
	if len(input.Value) == 0 || json.Unmarshal(input.Value, &decoded) != nil {
		return EventInput{}, "", newError("TRUTH_VALIDATION_FAILED", "value must be valid JSON", ErrValidation)
	}
	canonical, err := json.Marshal(decoded)
	if err != nil || len(canonical) > 1<<20 {
		return EventInput{}, "", newError("TRUTH_VALIDATION_FAILED", "value exceeds the 1 MiB canonical JSON limit", ErrValidation)
	}
	input.Value = canonical
	if input.EventID == "" {
		input.EventID, err = randomID()
		if err != nil {
			return EventInput{}, "", newError("TRUTH_WRITE_FAILED", "truth event id could not be generated", err)
		}
	}
	if input.RecordedAt.IsZero() {
		input.RecordedAt = now().UTC()
	}
	payload, _ := json.Marshal(struct {
		EntityType, EntityID, Attribute string
		Operation Operation
		Value json.RawMessage
		EffectiveChapter int
		SourceKind, SourceID string
		SourceChapter int
		Authority Authority
		Confidence float64
		Supersedes string
	}{input.EntityType, input.EntityID, input.Attribute, input.Operation, input.Value,
		input.EffectiveChapter, input.SourceKind, input.SourceID, input.SourceChapter,
		input.Authority, input.Confidence, input.SupersedesEventID})
	return input, sha256Hex(payload), nil
}

func randomID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func normalizeLimit(value, fallback, maximum int) int {
	if value <= 0 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func ErrorCode(err error) string { return errorCode(err) }
''')

write("internal/truth/store_test.go", r'''package truth

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "truth.db")
	if err := (migrate.Runner{Path: path, Migrations: []migrate.Migration{ProjectMigration}}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func appendJSON(t *testing.T, store *Store, key, entityType, entityID, attribute string, chapter int, value any, authority Authority, supersedes string) AppendResult {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Append(context.Background(), EventInput{IdempotencyKey: key, EntityType: entityType,
		EntityID: entityID, Attribute: attribute, Operation: OperationAssert, Value: encoded,
		EffectiveChapter: chapter, SourceKind: "chapter", SourceID: fmt.Sprintf("chapter-%d", chapter),
		SourceChapter: chapter, Authority: authority, Confidence: 0.95, SupersedesEventID: supersedes})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func factString(t *testing.T, page FactPage) string {
	t.Helper()
	if len(page.Facts) != 1 {
		t.Fatalf("facts=%d, want 1", len(page.Facts))
	}
	var value string
	if err := json.Unmarshal(page.Facts[0].Value, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestTemporalQueriesNeverLeakFutureState(t *testing.T) {
	store := newTestStore(t)
	appendJSON(t, store, "health-1", "character", "lin", "health", 1, "alive", AuthorityCanonical, "")
	appendJSON(t, store, "health-10", "character", "lin", "health", 10, "dead", AuthorityCanonical, "")
	before, err := store.FactsAsOf(context.Background(), Query{Chapter: 9, EntityType: "character", EntityID: "lin", Attribute: "health"})
	if err != nil || factString(t, before) != "alive" {
		t.Fatalf("chapter 9=%v, err=%v", before.Facts, err)
	}
	after, err := store.FactsAsOf(context.Background(), Query{Chapter: 10, EntityType: "character", EntityID: "lin", Attribute: "health"})
	if err != nil || factString(t, after) != "dead" {
		t.Fatalf("chapter 10=%v, err=%v", after.Facts, err)
	}
}

func TestInventoryAndKnowledgeBoundaries(t *testing.T) {
	store := newTestStore(t)
	appendJSON(t, store, "item-2", "item", "moon-key", "holder", 2, "lin", AuthorityHuman, "")
	appendJSON(t, store, "item-7", "item", "moon-key", "holder", 7, "mei", AuthorityHuman, "")
	appendJSON(t, store, "know-12", "knowledge", "mei", "royal-secret", 12, true, AuthorityCanonical, "")
	item, _ := store.FactsAsOf(context.Background(), Query{Chapter: 6, EntityType: "item", EntityID: "moon-key", Attribute: "holder"})
	if got := factString(t, item); got != "lin" {
		t.Fatalf("future inventory leaked: %q", got)
	}
	knowledge, _ := store.FactsAsOf(context.Background(), Query{Chapter: 11, EntityType: "knowledge", EntityID: "mei"})
	if knowledge.Total != 0 {
		t.Fatalf("future knowledge leaked: %+v", knowledge.Facts)
	}
}

func TestIdempotentReplayAndPayloadConflict(t *testing.T) {
	store := newTestStore(t)
	first := appendJSON(t, store, "same-key", "location", "tower", "weather", 3, "snow", AuthorityAI, "")
	value, _ := json.Marshal("snow")
	replayed, err := store.Append(context.Background(), EventInput{IdempotencyKey: "same-key", EntityType: "location", EntityID: "tower", Attribute: "weather", Operation: OperationAssert, Value: value, EffectiveChapter: 3, SourceKind: "chapter", SourceID: "chapter-3", SourceChapter: 3, Authority: AuthorityAI, Confidence: 0.95})
	if err != nil || !replayed.Replayed || replayed.Event.EventID != first.Event.EventID {
		t.Fatalf("replay=%+v err=%v", replayed, err)
	}
	other, _ := json.Marshal("rain")
	_, err = store.Append(context.Background(), EventInput{IdempotencyKey: "same-key", EntityType: "location", EntityID: "tower", Attribute: "weather", Operation: OperationAssert, Value: other, EffectiveChapter: 3, SourceKind: "chapter", SourceID: "chapter-3", SourceChapter: 3, Authority: AuthorityAI, Confidence: 0.95})
	if ErrorCode(err) != "TRUTH_IDEMPOTENCY_CONFLICT" {
		t.Fatalf("err=%v code=%s", err, ErrorCode(err))
	}
}

func TestAuthorityConflictAndExplicitSupersede(t *testing.T) {
	store := newTestStore(t)
	canonical := appendJSON(t, store, "canon", "organization", "guild", "leader", 1, "Aria", AuthorityCanonical, "")
	lower := appendJSON(t, store, "ai", "organization", "guild", "leader", 4, "Bram", AuthorityAI, "")
	if len(lower.Conflicts) == 0 {
		t.Fatal("expected authority conflict")
	}
	page, _ := store.FactsAsOf(context.Background(), Query{Chapter: 5, EntityType: "organization", EntityID: "guild", Attribute: "leader"})
	if got := factString(t, page); got != "Aria" {
		t.Fatalf("lower authority replaced canonical value: %q", got)
	}
	appendJSON(t, store, "explicit", "organization", "guild", "leader", 6, "Bram", AuthorityAI, canonical.Event.EventID)
	page, _ = store.FactsAsOf(context.Background(), Query{Chapter: 6, EntityType: "organization", EntityID: "guild", Attribute: "leader"})
	if got := factString(t, page); got != "Bram" {
		t.Fatalf("explicit supersede did not win: %q", got)
	}
}

func TestBoundedRebuildIsDeterministic(t *testing.T) {
	store := newTestStore(t)
	appendJSON(t, store, "r1", "character", "a", "location", 1, "home", AuthorityHuman, "")
	appendJSON(t, store, "r2", "character", "a", "location", 3, "road", AuthorityHuman, "")
	before, _ := store.FactsAsOf(context.Background(), Query{Chapter: 4, EntityID: "a", Attribute: "location"})
	result, err := store.RebuildFrom(context.Background(), 2)
	if err != nil || result.KeysRebuilt != 1 {
		t.Fatalf("rebuild=%+v err=%v", result, err)
	}
	after, _ := store.FactsAsOf(context.Background(), Query{Chapter: 4, EntityID: "a", Attribute: "location"})
	if string(before.Facts[0].Value) != string(after.Facts[0].Value) || before.Facts[0].EventID != after.Facts[0].EventID {
		t.Fatalf("projection changed after rebuild: before=%+v after=%+v", before, after)
	}
}

func TestConcurrentAppendsRemainComplete(t *testing.T) {
	store := newTestStore(t)
	var wg sync.WaitGroup
	errors := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			value, _ := json.Marshal(index)
			_, err := store.Append(context.Background(), EventInput{IdempotencyKey: fmt.Sprintf("concurrent-%d", index), EntityType: "state", EntityID: fmt.Sprintf("entity-%d", index), Attribute: "value", Operation: OperationAssert, Value: value, EffectiveChapter: index, SourceKind: "test", SourceID: fmt.Sprintf("source-%d", index), SourceChapter: index, Authority: AuthorityHuman, Confidence: 1})
			errors <- err
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err := store.FactsAsOf(context.Background(), Query{Chapter: 30, EntityType: "state", Limit: 100})
	if err != nil || page.Total != 20 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}

func TestQueryPlanAt100K(t *testing.T) {
	if testing.Short() || strings.TrimSpace(testEnv("NOVELFORGE_HEAVY_TESTS")) != "1" {
		t.Skip("set NOVELFORGE_HEAVY_TESTS=1 for the 100k-row index regression")
	}
	store := newTestStore(t)
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	eventStmt, _ := tx.Prepare(`INSERT INTO truth_events(event_id,idempotency_key,payload_hash,entity_type,entity_id,attribute,operation,value_json,value_hash,effective_chapter,source_kind,source_id,source_chapter,authority,confidence,recorded_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	projectionStmt, _ := tx.Prepare(`INSERT INTO truth_projection(event_id,entity_type,entity_id,attribute,value_json,value_hash,valid_from_chapter,valid_to_chapter,source_kind,source_id,source_chapter,authority,confidence) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := 0; i < 100000; i++ {
		id := fmt.Sprintf("bulk-%06d", i)
		entity := fmt.Sprintf("entity-%06d", i)
		if _, err := eventStmt.Exec(id, id, id, "character", entity, "status", "assert", `"ok"`, id, i%1000, "benchmark", id, i%1000, 80, 1, now); err != nil {
			t.Fatal(err)
		}
		if _, err := projectionStmt.Exec(id, "character", entity, "status", `"ok"`, id, i%1000, nil, "benchmark", id, i%1000, 80, 1); err != nil {
			t.Fatal(err)
		}
	}
	_ = eventStmt.Close()
	_ = projectionStmt.Close()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	rows, err := store.db.Query(`EXPLAIN QUERY PLAN SELECT event_id FROM truth_projection WHERE entity_type=? AND entity_id=? AND attribute=? AND valid_from_chapter<=? AND (valid_to_chapter IS NULL OR valid_to_chapter>=?)`, "character", "entity-099999", "status", 1000, 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	detail := ""
	for rows.Next() {
		var id, parent, unused int
		var line string
		_ = rows.Scan(&id, &parent, &unused, &line)
		detail += line
	}
	if !strings.Contains(detail, "truth_projection_key_idx") {
		t.Fatalf("temporal query does not use key index: %s", detail)
	}
}

var testEnv = func(key string) string {
	return getenv(key)
}
''')

write("internal/truth/env.go", r'''package truth

import "os"

func getenv(key string) string { return os.Getenv(key) }
''')

write("internal/project/truth.go", r'''package project

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ProjectDatabasePath returns the internal per-project database path after
// enforcing the workspace boundary and applying every registered migration.
// Transport code must never serialize this path.
func (r *Repository) ProjectDatabasePath(ctx context.Context, id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", newError("PROJECT_VALIDATION_FAILED", "project id is required", ErrValidation)
	}
	roots := []string{r.workspace}
	children, err := os.ReadDir(r.workspace)
	if err != nil {
		return "", newError("PROJECT_STORAGE_ERROR", "workspace could not be inspected", err)
	}
	for _, child := range children {
		if child.Type()&os.ModeSymlink != 0 || !child.IsDir() {
			continue
		}
		roots = append(roots, filepath.Join(r.workspace, child.Name()))
	}
	for _, root := range roots {
		info, err := os.Lstat(root)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			continue
		}
		metadataBytes, err := os.ReadFile(filepath.Join(root, projectMetadataRelative))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", newError("PROJECT_STORAGE_ERROR", "project metadata could not be read", err)
		}
		var metadata Metadata
		if err := json.Unmarshal(metadataBytes, &metadata); err != nil || metadata.ID != id {
			continue
		}
		if filepath.Clean(root) != filepath.Clean(r.workspace) {
			if err := ensureChildPath(r.workspace, root); err != nil {
				return "", err
			}
		}
		if err := r.initializeProjectDatabase(ctx, root); err != nil {
			return "", err
		}
		return filepath.Join(root, projectDatabaseRelative), nil
	}
	return "", newError("PROJECT_NOT_FOUND", "project not found", ErrNotFound)
}
''')

write("internal/project/truth_test.go", r'''package project

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
)

func TestProjectDatabaseIncludesTruthMigration(t *testing.T) {
	repository, err := NewRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project, err := repository.Create(context.Background(), CreateInput{Title: "Temporal Test"})
	if err != nil {
		t.Fatal(err)
	}
	path, err := repository.ProjectDatabasePath(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	db, err := migrate.Open(path, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"truth_events", "truth_projection", "truth_conflicts", "truth_rebuilds"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("table %s count=%d err=%v database=%s", table, count, err, filepath.Base(path))
		}
	}
	var version int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil || version != 2 {
		t.Fatalf("migration version=%d err=%v", version, err)
	}
}
''')

write("internal/server/truth.go", r'''package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/voocel/ainovel-cli/internal/project"
	truthstore "github.com/voocel/ainovel-cli/internal/truth"
)

var truthRepositories sync.Map

func configureTruthAPI(server *Server, mux *http.ServeMux, workspace string) error {
	repository, err := project.NewRepository(workspace)
	if err != nil {
		return fmt.Errorf("configure truth repository: %w", err)
	}
	truthRepositories.Store(server, repository)
	mux.HandleFunc("GET /api/projects/{id}/truth", server.handleTruthFacts)
	mux.HandleFunc("GET /api/projects/{id}/truth/events", server.handleTruthEvents)
	mux.HandleFunc("POST /api/projects/{id}/truth/events", server.handleTruthAppend)
	mux.HandleFunc("GET /api/projects/{id}/truth/conflicts", server.handleTruthConflicts)
	mux.HandleFunc("POST /api/projects/{id}/truth/rebuild", server.handleTruthRebuild)
	return nil
}

func (s *Server) truthStore(ctx context.Context, projectID string) (*truthstore.Store, error) {
	value, ok := truthRepositories.Load(s)
	if !ok {
		return nil, errors.New("truth repository is not configured")
	}
	path, err := value.(*project.Repository).ProjectDatabasePath(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return truthstore.Open(ctx, path)
}

func (s *Server) handleTruthAppend(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		writeTruthError(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required", false)
		return
	}
	var input truthstore.EventInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeTruthError(w, http.StatusBadRequest, "TRUTH_VALIDATION_FAILED", "request body must be valid JSON", false)
		return
	}
	input.IdempotencyKey = key
	store, err := s.truthStore(r.Context(), r.PathValue("id"))
	if err != nil {
		writeTruthDomainError(w, err)
		return
	}
	defer store.Close()
	result, err := store.Append(r.Context(), input)
	if err != nil {
		writeTruthDomainError(w, err)
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	writeTruthJSON(w, status, result)
}

func (s *Server) handleTruthFacts(w http.ResponseWriter, r *http.Request) {
	chapter, err := requiredNonNegativeInt(r, "chapter")
	if err != nil {
		writeTruthError(w, http.StatusBadRequest, "TRUTH_VALIDATION_FAILED", err.Error(), false)
		return
	}
	store, err := s.truthStore(r.Context(), r.PathValue("id"))
	if err != nil {
		writeTruthDomainError(w, err)
		return
	}
	defer store.Close()
	page, err := store.FactsAsOf(r.Context(), truthstore.Query{Chapter: chapter,
		EntityType: r.URL.Query().Get("entity_type"), EntityID: r.URL.Query().Get("entity_id"),
		Attribute: r.URL.Query().Get("attribute"), Limit: queryInt(r, "limit", 100), Offset: queryInt(r, "offset", 0)})
	if err != nil {
		writeTruthDomainError(w, err)
		return
	}
	writeTruthJSON(w, http.StatusOK, page)
}

func (s *Server) handleTruthEvents(w http.ResponseWriter, r *http.Request) {
	var through *int
	if raw := strings.TrimSpace(r.URL.Query().Get("through_chapter")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			writeTruthError(w, http.StatusBadRequest, "TRUTH_VALIDATION_FAILED", "through_chapter must be a non-negative integer", false)
			return
		}
		through = &value
	}
	store, err := s.truthStore(r.Context(), r.PathValue("id"))
	if err != nil {
		writeTruthDomainError(w, err)
		return
	}
	defer store.Close()
	page, err := store.Events(r.Context(), truthstore.EventQuery{ThroughChapter: through,
		EntityType: r.URL.Query().Get("entity_type"), EntityID: r.URL.Query().Get("entity_id"),
		Attribute: r.URL.Query().Get("attribute"), Limit: queryInt(r, "limit", 100), Offset: queryInt(r, "offset", 0)})
	if err != nil {
		writeTruthDomainError(w, err)
		return
	}
	writeTruthJSON(w, http.StatusOK, page)
}

func (s *Server) handleTruthConflicts(w http.ResponseWriter, r *http.Request) {
	store, err := s.truthStore(r.Context(), r.PathValue("id"))
	if err != nil {
		writeTruthDomainError(w, err)
		return
	}
	defer store.Close()
	page, err := store.Conflicts(r.Context(), queryInt(r, "limit", 100), queryInt(r, "offset", 0))
	if err != nil {
		writeTruthDomainError(w, err)
		return
	}
	writeTruthJSON(w, http.StatusOK, page)
}

func (s *Server) handleTruthRebuild(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("Idempotency-Key")) == "" {
		writeTruthError(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required", false)
		return
	}
	var input struct { FromChapter int `json:"from_chapter"` }
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input.FromChapter < 0 {
		writeTruthError(w, http.StatusBadRequest, "TRUTH_VALIDATION_FAILED", "from_chapter must be a non-negative integer", false)
		return
	}
	store, err := s.truthStore(r.Context(), r.PathValue("id"))
	if err != nil {
		writeTruthDomainError(w, err)
		return
	}
	defer store.Close()
	result, err := store.RebuildFrom(r.Context(), input.FromChapter)
	if err != nil {
		writeTruthDomainError(w, err)
		return
	}
	writeTruthJSON(w, http.StatusOK, result)
}

func requiredNonNegativeInt(r *http.Request, name string) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return 0, fmt.Errorf("%s is required", name)
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return value, nil
}

func queryInt(r *http.Request, name string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func writeTruthDomainError(w http.ResponseWriter, err error) {
	code := truthstore.ErrorCode(err)
	status := http.StatusInternalServerError
	retryable := false
	switch code {
	case "TRUTH_VALIDATION_FAILED", "IDEMPOTENCY_KEY_REQUIRED":
		status = http.StatusBadRequest
	case "TRUTH_IDEMPOTENCY_CONFLICT", "TRUTH_INVALID_SUPERSEDE":
		status = http.StatusConflict
	case "PROJECT_NOT_FOUND", "TRUTH_NOT_FOUND":
		status = http.StatusNotFound
	case "TRUTH_DATABASE_OPEN_FAILED", "TRUTH_WRITE_FAILED", "TRUTH_REBUILD_FAILED":
		retryable = true
	}
	message := "truth operation failed"
	var typed *truthstore.Error
	if errors.As(err, &typed) {
		message = typed.Message
	}
	writeTruthError(w, status, code, message, retryable)
}

func writeTruthError(w http.ResponseWriter, status int, code, message string, retryable bool) {
	writeTruthJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message,
		"details": map[string]any{}, "retryable": retryable, "trace_id": truthTraceID()}})
}

func writeTruthJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func truthTraceID() string {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "truth-trace"
	}
	return hex.EncodeToString(value)
}
''')

write("internal/server/truth_test.go", r'''package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/voocel/ainovel-cli/internal/project"
)

func TestTruthAPIAppendQueryReplayAndFutureBoundary(t *testing.T) {
	workspace := t.TempDir()
	repository, err := project.NewRepository(workspace)
	if err != nil {
		t.Fatal(err)
	}
	created, err := repository.Create(context.Background(), project.CreateInput{Title: "Truth API"})
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(Config{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	appendEvent := func(key string, chapter int, value string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"entity_type": "character", "entity_id": "lin", "attribute": "health", "operation": "assert", "value": value, "effective_chapter": chapter, "source_kind": "chapter", "source_id": key, "source_chapter": chapter, "authority": 100, "confidence": 1})
		request := httptest.NewRequest(http.MethodPost, "/api/projects/"+created.ID+"/truth/events", bytes.NewReader(body))
		request.Header.Set("Idempotency-Key", key)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		return response
	}
	if response := appendEvent("health-1", 1, "alive"); response.Code != http.StatusCreated {
		t.Fatalf("append status=%d body=%s", response.Code, response.Body.String())
	}
	if response := appendEvent("health-10", 10, "dead"); response.Code != http.StatusCreated {
		t.Fatalf("append status=%d body=%s", response.Code, response.Body.String())
	}
	if response := appendEvent("health-1", 1, "alive"); response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"replayed":true`)) {
		t.Fatalf("replay status=%d body=%s", response.Code, response.Body.String())
	}
	query := httptest.NewRecorder()
	server.Handler().ServeHTTP(query, httptest.NewRequest(http.MethodGet, "/api/projects/"+created.ID+"/truth?chapter=5&entity_type=character&entity_id=lin&attribute=health", nil))
	if query.Code != http.StatusOK || !bytes.Contains(query.Body.Bytes(), []byte(`"alive"`)) || bytes.Contains(query.Body.Bytes(), []byte(`"dead"`)) {
		t.Fatalf("query status=%d body=%s", query.Code, query.Body.String())
	}
}

func TestTruthWritesRequireIdempotencyKey(t *testing.T) {
	workspace := t.TempDir()
	repository, _ := project.NewRepository(workspace)
	created, _ := repository.Create(context.Background(), project.CreateInput{Title: "Headers"})
	server, _ := New(Config{Workspace: workspace})
	request := httptest.NewRequest(http.MethodPost, "/api/projects/"+created.ID+"/truth/events", bytes.NewReader([]byte(`{}`)))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte("IDEMPOTENCY_KEY_REQUIRED")) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
''')

write("docs/TRUTH_STORE.md", r'''# Structured Truth Store

Phase 4 introduces a per-project, append-only temporal Truth Store in `.novelforge/project.db`.

## Model

Every accepted assertion or retraction is recorded as an immutable `truth_events` row with:

- opaque event and idempotency identifiers;
- entity type, entity id and attribute key;
- canonical JSON value and payload hash;
- effective chapter and source chapter;
- source kind/source id provenance;
- authority and confidence;
- optional explicit supersede edge;
- durable recorded timestamp.

`truth_projection` is derived state. It stores chapter validity intervals and can always be rebuilt from the event log. `truth_conflicts` records competing assertions instead of silently discarding them. `truth_rebuilds` audits bounded rebuild requests.

## Temporal safety

A Chapter-N query only returns projection intervals where `valid_from_chapter <= N` and `valid_to_chapter` is absent or at least N. The same boundary is used for characters, inventory, relations, locations and knowledge, preventing future state from leaking into earlier chapter context.

## Authority and supersede

Later assertions of equal or greater authority advance the projection. A lower-authority contradictory assertion becomes a durable conflict and does not silently replace the authoritative fact. An explicit supersede edge may intentionally replace an active event when the entity/attribute key matches.

## APIs

```text
GET  /api/projects/{id}/truth?chapter=N
GET  /api/projects/{id}/truth/events
POST /api/projects/{id}/truth/events
GET  /api/projects/{id}/truth/conflicts
POST /api/projects/{id}/truth/rebuild
```

All writes require `Idempotency-Key`. Reusing a key with the same canonical payload returns the original event; reusing it with a different payload returns a conflict. Absolute paths, SQL details and credentials never enter the response envelope.

## Rebuild boundary

`POST .../truth/rebuild` selects only keys whose events or active projection overlap the requested chapter boundary. It recomputes those keys transactionally and records the completed boundary. Source events are never deleted by rebuild.
''')

# Add the immutable migration to the existing per-project migration list.
repository = ROOT / "internal/project/repository.go"
text = repository.read_text(encoding="utf-8")
if 'internal/truth' not in text:
    text = text.replace('"github.com/voocel/ainovel-cli/internal/db/migrate"', '"github.com/voocel/ainovel-cli/internal/db/migrate"\n\ttruthstore "github.com/voocel/ainovel-cli/internal/truth"')
if 'truthstore.ProjectMigration' not in text:
    match = re.search(r'var projectMigrations = \[\]migrate\.Migration\{.*?\n\}', text, re.S)
    if not match:
        raise SystemExit('projectMigrations block not found')
    block = match.group(0)
    block = block[:-2] + '\ttruthstore.ProjectMigration,\n}'
    text = text[:match.start()] + block + text[match.end():]
repository.write_text(text, encoding="utf-8")

# Register the API against the existing server and ServeMux without changing
# the Server struct or its established repositories.
server_path = ROOT / "internal/server/server.go"
server_text = server_path.read_text(encoding="utf-8")
if 'configureTruthAPI(' not in server_text:
    mux_match = re.search(r'(\w+)\s*:=\s*http\.NewServeMux\(\)', server_text)
    server_match = re.search(r'(\w+)\s*:=\s*&Server\{', server_text)
    if not mux_match or not server_match:
        raise SystemExit('server or ServeMux construction not found')
    mux_name = mux_match.group(1)
    server_name = server_match.group(1)
    return_pattern = re.compile(r'\n\s*return\s+' + re.escape(server_name) + r'\s*,\s*nil')
    return_match = return_pattern.search(server_text)
    if not return_match:
        raise SystemExit('server return not found')
    insertion = f'\n\tif err := configureTruthAPI({server_name}, {mux_name}, config.Workspace); err != nil {{\n\t\treturn nil, err\n\t}}'
    server_text = server_text[:return_match.start()] + insertion + server_text[return_match.start():]
server_path.write_text(server_text, encoding="utf-8")

# Extend OpenAPI 3.1 deterministically.
openapi_path = ROOT / "internal/server/openapi.json"
doc = json.loads(openapi_path.read_text(encoding="utf-8"))
components = doc.setdefault("components", {}).setdefault("schemas", {})
components.update({
    "TruthEventInput": {"type": "object", "required": ["entity_type", "entity_id", "attribute", "value", "effective_chapter", "source_kind", "source_id", "source_chapter"], "properties": {
        "event_id": {"type": "string"}, "entity_type": {"type": "string"}, "entity_id": {"type": "string"}, "attribute": {"type": "string"},
        "operation": {"type": "string", "enum": ["assert", "retract"], "default": "assert"}, "value": {},
        "effective_chapter": {"type": "integer", "minimum": 0}, "source_kind": {"type": "string"}, "source_id": {"type": "string"},
        "source_chapter": {"type": "integer", "minimum": 0}, "authority": {"type": "integer", "minimum": 0, "maximum": 100},
        "confidence": {"type": "number", "minimum": 0, "maximum": 1}, "supersedes_event_id": {"type": "string"}
    }},
    "TruthEvent": {"allOf": [{"$ref": "#/components/schemas/TruthEventInput"}, {"type": "object", "properties": {
        "sequence": {"type": "integer"}, "event_id": {"type": "string"}, "payload_hash": {"type": "string"}, "recorded_at": {"type": "string", "format": "date-time"},
        "superseded_by_event_id": {"type": "string"}
    }}]},
    "TruthFact": {"type": "object", "required": ["event_id", "entity_type", "entity_id", "attribute", "value", "valid_from_chapter"], "properties": {
        "event_id": {"type": "string"}, "entity_type": {"type": "string"}, "entity_id": {"type": "string"}, "attribute": {"type": "string"}, "value": {},
        "valid_from_chapter": {"type": "integer"}, "valid_to_chapter": {"type": ["integer", "null"]}, "source_kind": {"type": "string"}, "source_id": {"type": "string"},
        "source_chapter": {"type": "integer"}, "authority": {"type": "integer"}, "confidence": {"type": "number"}
    }},
    "TruthConflict": {"type": "object", "properties": {"conflict_id": {"type": "string"}, "entity_type": {"type": "string"}, "entity_id": {"type": "string"},
        "attribute": {"type": "string"}, "chapter": {"type": "integer"}, "winning_event_id": {"type": "string"}, "conflicting_event_id": {"type": "string"}, "reason": {"type": "string"}}},
})
idem = {"name": "Idempotency-Key", "in": "header", "required": True, "schema": {"type": "string", "minLength": 1}}
project_id = {"name": "id", "in": "path", "required": True, "schema": {"type": "string"}}
json_response = lambda schema: {"200": {"description": "Success", "content": {"application/json": {"schema": schema}}}, "400": {"description": "Validation error", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/ErrorEnvelope"}}}}}
paths = doc.setdefault("paths", {})
paths["/api/projects/{id}/truth"] = {"get": {"operationId": "queryTruthAsOfChapter", "parameters": [project_id, {"name": "chapter", "in": "query", "required": True, "schema": {"type": "integer", "minimum": 0}}, {"name": "entity_type", "in": "query", "schema": {"type": "string"}}, {"name": "entity_id", "in": "query", "schema": {"type": "string"}}, {"name": "attribute", "in": "query", "schema": {"type": "string"}}], "responses": json_response({"type": "object", "properties": {"facts": {"type": "array", "items": {"$ref": "#/components/schemas/TruthFact"}}, "total": {"type": "integer"}}})}}
paths["/api/projects/{id}/truth/events"] = {
    "get": {"operationId": "listTruthEvents", "parameters": [project_id, {"name": "through_chapter", "in": "query", "schema": {"type": "integer", "minimum": 0}}], "responses": json_response({"type": "object", "properties": {"events": {"type": "array", "items": {"$ref": "#/components/schemas/TruthEvent"}}}})},
    "post": {"operationId": "appendTruthEvent", "parameters": [project_id, idem], "requestBody": {"required": True, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/TruthEventInput"}}}}, "responses": {"201": {"description": "Truth event appended"}, "200": {"description": "Idempotent replay"}, "400": {"description": "Validation error"}, "409": {"description": "Conflict"}}}
}
paths["/api/projects/{id}/truth/conflicts"] = {"get": {"operationId": "listTruthConflicts", "parameters": [project_id], "responses": json_response({"type": "object", "properties": {"conflicts": {"type": "array", "items": {"$ref": "#/components/schemas/TruthConflict"}}}})}}
paths["/api/projects/{id}/truth/rebuild"] = {"post": {"operationId": "rebuildTruthProjection", "parameters": [project_id, idem], "requestBody": {"required": True, "content": {"application/json": {"schema": {"type": "object", "required": ["from_chapter"], "properties": {"from_chapter": {"type": "integer", "minimum": 0}}}}}}, "responses": json_response({"type": "object", "properties": {"from_chapter": {"type": "integer"}, "keys_rebuilt": {"type": "integer"}, "completed_at": {"type": "string", "format": "date-time"}})}}}
openapi_path.write_text(json.dumps(doc, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

# Keep the OpenAPI route-drift map synchronized.
test_path = ROOT / "internal/server/openapi_test.go"
test_text = test_path.read_text(encoding="utf-8")
if '"/api/projects/{id}/truth"' not in test_text:
    marker = '"/api/projects/{id}/foundation"'
    position = test_text.find(marker)
    if position < 0:
        raise SystemExit('OpenAPI expected route map marker not found')
    line_end = test_text.find('\n', position)
    addition = '\n\t\t"/api/projects/{id}/truth":           {"get"},\n\t\t"/api/projects/{id}/truth/events":    {"get", "post"},\n\t\t"/api/projects/{id}/truth/conflicts": {"get"},\n\t\t"/api/projects/{id}/truth/rebuild":   {"post"},'
    test_text = test_text[:line_end] + addition + test_text[line_end:]
test_path.write_text(test_text, encoding="utf-8")

# Document the implementation state without claiming formal acceptance before CI.
status_path = ROOT / "docs/IMPLEMENTATION_STATUS.md"
status = status_path.read_text(encoding="utf-8")
status = status.replace('| 4–13 | not started | Start Phase 4 from the actual remote main after running the required baseline. |', '| 4 — Structured Truth Store | implementation complete; acceptance pending | Temporal event log, projections, provenance, conflicts, supersede, rebuild and Truth APIs are implemented on PR #10. |\n| 5–13 | not started | Begin only after Phase 4 PR and merged-main CI pass. |')
if '## Phase 4 implementation' not in status:
    status += '''\n## Phase 4 implementation\n\n- Added migration 2 with append-only Truth events, temporal projection intervals, durable conflicts and rebuild audit records.\n- Added deterministic idempotent append, authority precedence, explicit supersede and bounded projection rebuild.\n- Added Chapter-N safe fact/event queries that do not expose future character, inventory or knowledge state.\n- Added provenance fields for source kind, source id, source chapter, authority and confidence.\n- Added Truth REST routes and synchronized OpenAPI 3.1 contracts.\n- Added migration, API, concurrency, temporal leakage, authority, idempotency, rebuild and optional 100k-row index regression tests.\n- Formal completion remains blocked until PR #10 and the merged-main workflow both pass.\n'''
status_path.write_text(status, encoding="utf-8")

readme_path = ROOT / "README.md"
readme = readme_path.read_text(encoding="utf-8")
if 'Structured Truth Store' not in readme:
    readme += '\n## Structured Truth Store\n\nPhase 4 provides append-only, provenance-bearing Truth events and Chapter-N temporal projections. See [`docs/TRUTH_STORE.md`](docs/TRUTH_STORE.md).\n'
readme_path.write_text(readme, encoding="utf-8")

print('Phase 4 source applied')
