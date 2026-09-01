#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(sys.argv[1] if len(sys.argv) > 1 else ".").resolve()

FILES: dict[str, str] = {
    "internal/truthstore/model.go": r'''package truthstore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	maxIdentifierRunes = 200
	maxPredicateRunes  = 128
	maxValueBytes      = 64 << 10
	maxExcerptRunes    = 2000
	maxPageSize        = 500
)

var predicatePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:-]{0,127}$`)

type Authority string

const (
	AuthorityInferred Authority = "inferred"
	AuthorityAgent    Authority = "agent"
	AuthorityImported Authority = "imported"
	AuthorityCanon    Authority = "canon"
	AuthorityHuman    Authority = "human"
)

func (a Authority) rank() (int, bool) {
	switch a {
	case AuthorityInferred:
		return 0, true
	case AuthorityAgent:
		return 10, true
	case AuthorityImported:
		return 20, true
	case AuthorityCanon:
		return 30, true
	case AuthorityHuman:
		return 40, true
	default:
		return 0, false
	}
}

type EventKind string

const (
	EventAssert    EventKind = "assert"
	EventSupersede EventKind = "supersede"
	EventRetract   EventKind = "retract"
)

type Source struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Revision string `json:"revision,omitempty"`
	Excerpt  string `json:"excerpt,omitempty"`
}

type AppendInput struct {
	IdempotencyKey  string          `json:"-"`
	Kind            EventKind       `json:"kind"`
	SubjectType     string          `json:"subject_type"`
	SubjectID       string          `json:"subject_id"`
	Predicate       string          `json:"predicate"`
	Value           json.RawMessage `json:"value"`
	ValidFromChapter int            `json:"valid_from_chapter"`
	ValidToChapter   *int           `json:"valid_to_chapter,omitempty"`
	KnownFromChapter int            `json:"known_from_chapter"`
	KnownToChapter   *int           `json:"known_to_chapter,omitempty"`
	Authority       Authority       `json:"authority"`
	Confidence      float64         `json:"confidence"`
	Source          Source          `json:"source"`
	SupersedesEventID string        `json:"supersedes_event_id,omitempty"`
}

type Event struct {
	Sequence          int64           `json:"sequence"`
	ID                string          `json:"id"`
	IdempotencyKey    string          `json:"-"`
	RequestHash       string          `json:"-"`
	Kind              EventKind       `json:"kind"`
	SubjectType       string          `json:"subject_type"`
	SubjectID         string          `json:"subject_id"`
	Predicate         string          `json:"predicate"`
	Value             json.RawMessage `json:"value"`
	ValidFromChapter  int             `json:"valid_from_chapter"`
	ValidToChapter    *int            `json:"valid_to_chapter,omitempty"`
	KnownFromChapter  int             `json:"known_from_chapter"`
	KnownToChapter    *int            `json:"known_to_chapter,omitempty"`
	Authority         Authority       `json:"authority"`
	Confidence        float64         `json:"confidence"`
	Source            Source          `json:"source"`
	SupersedesEventID string          `json:"supersedes_event_id,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	Checksum          string          `json:"checksum"`
}

type AppendResult struct {
	Event    Event `json:"event"`
	Replayed bool  `json:"replayed"`
}

type StateQuery struct {
	Chapter     int
	SubjectType string
	SubjectID   string
	Predicate   string
	Limit       int
	Offset      int
}

type Fact struct {
	Event
	EffectiveFromChapter int    `json:"effective_from_chapter"`
	EffectiveToChapter   *int   `json:"effective_to_chapter,omitempty"`
	SupersededByEventID  string `json:"superseded_by_event_id,omitempty"`
	Conflicted           bool   `json:"conflicted"`
}

type StatePage struct {
	Facts      []Fact `json:"facts"`
	Conflicts  int    `json:"conflicts"`
	Total      int    `json:"total"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
	NextOffset *int   `json:"next_offset,omitempty"`
}

type EventPage struct {
	Events       []Event `json:"events"`
	AfterSequence int64  `json:"after_sequence"`
	Limit        int     `json:"limit"`
	NextSequence *int64  `json:"next_sequence,omitempty"`
}

type ConflictQuery struct {
	Chapter     *int
	SubjectType string
	SubjectID   string
	Predicate   string
	Limit       int
	Offset      int
}

type Conflict struct {
	ID              string `json:"id"`
	SubjectType     string `json:"subject_type"`
	SubjectID       string `json:"subject_id"`
	Predicate       string `json:"predicate"`
	LeftEventID     string `json:"left_event_id"`
	RightEventID    string `json:"right_event_id"`
	FromChapter     int    `json:"from_chapter"`
	ToChapter       *int   `json:"to_chapter,omitempty"`
	Reason          string `json:"reason"`
}

type ConflictPage struct {
	Conflicts  []Conflict `json:"conflicts"`
	Total      int        `json:"total"`
	Limit      int        `json:"limit"`
	Offset     int        `json:"offset"`
	NextOffset *int       `json:"next_offset,omitempty"`
}

type RebuildResult struct {
	FromChapter      int    `json:"from_chapter"`
	EventsReplayed   int    `json:"events_replayed"`
	FactsProjected   int    `json:"facts_projected"`
	ConflictsProjected int  `json:"conflicts_projected"`
	ProjectionDigest string `json:"projection_digest"`
}

type VerifyResult struct {
	Events           int      `json:"events"`
	Facts            int      `json:"facts"`
	Conflicts        int      `json:"conflicts"`
	ProjectionDigest string   `json:"projection_digest"`
	Valid            bool     `json:"valid"`
	Violations       []string `json:"violations"`
}

type Code string

const (
	CodeValidation          Code = "TRUTH_VALIDATION_FAILED"
	CodeNotFound            Code = "TRUTH_EVENT_NOT_FOUND"
	CodeConflict            Code = "TRUTH_CONFLICT"
	CodeAuthority           Code = "TRUTH_AUTHORITY_VIOLATION"
	CodeIdempotencyConflict Code = "TRUTH_IDEMPOTENCY_CONFLICT"
	CodeCorrupt             Code = "TRUTH_PROJECTION_CORRUPT"
	CodeBusy                Code = "TRUTH_STORE_BUSY"
	CodeStorage             Code = "TRUTH_STORAGE_ERROR"
)

type Error struct {
	Code      Code
	Message   string
	Retryable bool
	Cause     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

func newError(code Code, message string, retryable bool, cause error) error {
	return &Error{Code: code, Message: message, Retryable: retryable, Cause: cause}
}

func AsError(err error) (*Error, bool) {
	var target *Error
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}

type normalizedInput struct {
	AppendInput
	ValueHash   string
	RequestHash string
}

func normalizeAppendInput(input AppendInput) (normalizedInput, error) {
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.SubjectType = strings.TrimSpace(input.SubjectType)
	input.SubjectID = strings.TrimSpace(input.SubjectID)
	input.Predicate = strings.TrimSpace(input.Predicate)
	input.SupersedesEventID = strings.TrimSpace(input.SupersedesEventID)
	input.Source.Type = strings.TrimSpace(input.Source.Type)
	input.Source.ID = strings.TrimSpace(input.Source.ID)
	input.Source.Revision = strings.TrimSpace(input.Source.Revision)
	input.Source.Excerpt = strings.TrimSpace(input.Source.Excerpt)
	if input.Kind == "" {
		input.Kind = EventAssert
	}
	if input.KnownFromChapter == 0 && input.ValidFromChapter > 0 {
		input.KnownFromChapter = input.ValidFromChapter
	}
	if input.IdempotencyKey == "" || len([]rune(input.IdempotencyKey)) > maxIdentifierRunes {
		return normalizedInput{}, newError(CodeValidation, "Idempotency-Key is required and must be at most 200 characters", false, nil)
	}
	if input.Kind != EventAssert && input.Kind != EventSupersede && input.Kind != EventRetract {
		return normalizedInput{}, newError(CodeValidation, "kind must be assert, supersede, or retract", false, nil)
	}
	if input.Kind == EventAssert && input.SupersedesEventID != "" {
		return normalizedInput{}, newError(CodeValidation, "assert events cannot name supersedes_event_id", false, nil)
	}
	if (input.Kind == EventSupersede || input.Kind == EventRetract) && input.SupersedesEventID == "" {
		return normalizedInput{}, newError(CodeValidation, "supersede and retract events require supersedes_event_id", false, nil)
	}
	if !predicatePattern.MatchString(input.SubjectType) {
		return normalizedInput{}, newError(CodeValidation, "subject_type has an invalid format", false, nil)
	}
	if input.SubjectID == "" || len([]rune(input.SubjectID)) > maxIdentifierRunes {
		return normalizedInput{}, newError(CodeValidation, "subject_id is required and must be at most 200 characters", false, nil)
	}
	if !predicatePattern.MatchString(input.Predicate) || len([]rune(input.Predicate)) > maxPredicateRunes {
		return normalizedInput{}, newError(CodeValidation, "predicate has an invalid format", false, nil)
	}
	if input.ValidFromChapter < 0 || input.KnownFromChapter < 0 {
		return normalizedInput{}, newError(CodeValidation, "chapter boundaries must not be negative", false, nil)
	}
	if input.ValidToChapter != nil && *input.ValidToChapter < input.ValidFromChapter {
		return normalizedInput{}, newError(CodeValidation, "valid_to_chapter must not precede valid_from_chapter", false, nil)
	}
	if input.KnownToChapter != nil && *input.KnownToChapter < input.KnownFromChapter {
		return normalizedInput{}, newError(CodeValidation, "known_to_chapter must not precede known_from_chapter", false, nil)
	}
	if _, ok := input.Authority.rank(); !ok {
		return normalizedInput{}, newError(CodeValidation, "authority is invalid", false, nil)
	}
	if input.Confidence < 0 || input.Confidence > 1 {
		return normalizedInput{}, newError(CodeValidation, "confidence must be between 0 and 1", false, nil)
	}
	if input.Source.Type == "" || input.Source.ID == "" {
		return normalizedInput{}, newError(CodeValidation, "source.type and source.id are required", false, nil)
	}
	if len([]rune(input.Source.Excerpt)) > maxExcerptRunes {
		return normalizedInput{}, newError(CodeValidation, "source excerpt must be at most 2000 characters", false, nil)
	}
	canonical, valueHash, err := canonicalizeValue(input.Kind, input.Value)
	if err != nil {
		return normalizedInput{}, err
	}
	input.Value = canonical
	requestHash, err := hashJSON(struct {
		Kind              EventKind       `json:"kind"`
		SubjectType       string          `json:"subject_type"`
		SubjectID         string          `json:"subject_id"`
		Predicate         string          `json:"predicate"`
		Value             json.RawMessage `json:"value"`
		ValidFromChapter  int             `json:"valid_from_chapter"`
		ValidToChapter    *int            `json:"valid_to_chapter,omitempty"`
		KnownFromChapter  int             `json:"known_from_chapter"`
		KnownToChapter    *int            `json:"known_to_chapter,omitempty"`
		Authority         Authority       `json:"authority"`
		Confidence        float64         `json:"confidence"`
		Source            Source          `json:"source"`
		SupersedesEventID string          `json:"supersedes_event_id,omitempty"`
	}{input.Kind, input.SubjectType, input.SubjectID, input.Predicate, input.Value,
		input.ValidFromChapter, input.ValidToChapter, input.KnownFromChapter,
		input.KnownToChapter, input.Authority, input.Confidence, input.Source,
		input.SupersedesEventID})
	if err != nil {
		return normalizedInput{}, newError(CodeValidation, "truth request cannot be canonicalized", false, err)
	}
	return normalizedInput{AppendInput: input, ValueHash: valueHash, RequestHash: requestHash}, nil
}

func canonicalizeValue(kind EventKind, raw json.RawMessage) (json.RawMessage, string, error) {
	if kind == EventRetract && len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage("null")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, "", newError(CodeValidation, "value is required", false, nil)
	}
	if len(raw) > maxValueBytes {
		return nil, "", newError(CodeValidation, "value exceeds 64 KiB", false, nil)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, "", newError(CodeValidation, "value must be valid JSON", false, err)
	}
	if decoder.More() {
		return nil, "", newError(CodeValidation, "value must contain one JSON value", false, nil)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, "", newError(CodeValidation, "value cannot be canonicalized", false, err)
	}
	sum := sha256.Sum256(canonical)
	return json.RawMessage(canonical), hex.EncodeToString(sum[:]), nil
}

func hashJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func eventChecksum(event Event) string {
	value := struct {
		Sequence          int64           `json:"sequence"`
		ID                string          `json:"id"`
		IdempotencyKey    string          `json:"idempotency_key"`
		RequestHash       string          `json:"request_hash"`
		Kind              EventKind       `json:"kind"`
		SubjectType       string          `json:"subject_type"`
		SubjectID         string          `json:"subject_id"`
		Predicate         string          `json:"predicate"`
		Value             json.RawMessage `json:"value"`
		ValidFromChapter  int             `json:"valid_from_chapter"`
		ValidToChapter    *int            `json:"valid_to_chapter,omitempty"`
		KnownFromChapter  int             `json:"known_from_chapter"`
		KnownToChapter    *int            `json:"known_to_chapter,omitempty"`
		Authority         Authority       `json:"authority"`
		Confidence        float64         `json:"confidence"`
		Source            Source          `json:"source"`
		SupersedesEventID string          `json:"supersedes_event_id,omitempty"`
		CreatedAt         string          `json:"created_at"`
	}{event.Sequence, event.ID, event.IdempotencyKey, event.RequestHash, event.Kind,
		event.SubjectType, event.SubjectID, event.Predicate, event.Value,
		event.ValidFromChapter, event.ValidToChapter, event.KnownFromChapter,
		event.KnownToChapter, event.Authority, event.Confidence, event.Source,
		event.SupersedesEventID, event.CreatedAt.UTC().Format(time.RFC3339Nano)}
	hash, err := hashJSON(value)
	if err != nil {
		panic(fmt.Sprintf("truth event checksum: %v", err))
	}
	return hash
}

func effectiveBounds(validFrom int, validTo *int, knownFrom int, knownTo *int) (int, *int) {
	from := validFrom
	if knownFrom > from {
		from = knownFrom
	}
	var to *int
	if validTo != nil {
		value := *validTo
		to = &value
	}
	if knownTo != nil && (to == nil || *knownTo < *to) {
		value := *knownTo
		to = &value
	}
	return from, to
}

func normalizePage(limit, offset int) (int, int, error) {
	if offset < 0 {
		return 0, 0, newError(CodeValidation, "offset must not be negative", false, nil)
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	return limit, offset, nil
}

func sameKey(left, right Event) bool {
	return left.SubjectType == right.SubjectType && left.SubjectID == right.SubjectID && left.Predicate == right.Predicate
}

func trimFilter(value string, pattern bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len([]rune(value)) > maxIdentifierRunes {
		return "", newError(CodeValidation, "truth filter is too long", false, nil)
	}
	if pattern && !predicatePattern.MatchString(value) {
		return "", newError(CodeValidation, "truth filter has an invalid format", false, nil)
	}
	return value, nil
}
''',
    "internal/truthstore/migration.go": r'''package truthstore

import "github.com/voocel/ainovel-cli/internal/db/migrate"

func Migration() migrate.Migration {
	return migrate.Migration{
		Version: 2,
		Name:    "structured_truth_store",
		SQL: `CREATE TABLE truth_events (
			sequence INTEGER PRIMARY KEY AUTOINCREMENT,
			id TEXT NOT NULL UNIQUE,
			idempotency_key TEXT NOT NULL UNIQUE,
			request_hash TEXT NOT NULL,
			kind TEXT NOT NULL CHECK (kind IN ('assert', 'supersede', 'retract')),
			subject_type TEXT NOT NULL,
			subject_id TEXT NOT NULL,
			predicate TEXT NOT NULL,
			value_json TEXT NOT NULL CHECK (json_valid(value_json)),
			valid_from_chapter INTEGER NOT NULL CHECK (valid_from_chapter >= 0),
			valid_to_chapter INTEGER CHECK (valid_to_chapter IS NULL OR valid_to_chapter >= valid_from_chapter),
			known_from_chapter INTEGER NOT NULL CHECK (known_from_chapter >= 0),
			known_to_chapter INTEGER CHECK (known_to_chapter IS NULL OR known_to_chapter >= known_from_chapter),
			authority TEXT NOT NULL CHECK (authority IN ('inferred', 'agent', 'imported', 'canon', 'human')),
			confidence REAL NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
			source_type TEXT NOT NULL,
			source_id TEXT NOT NULL,
			source_revision TEXT NOT NULL DEFAULT '',
			source_excerpt TEXT NOT NULL DEFAULT '',
			supersedes_event_id TEXT REFERENCES truth_events(id),
			created_at TEXT NOT NULL,
			checksum TEXT NOT NULL
		);
		CREATE INDEX idx_truth_events_key_sequence
			ON truth_events(subject_type, subject_id, predicate, sequence);
		CREATE INDEX idx_truth_events_temporal
			ON truth_events(valid_from_chapter, known_from_chapter, sequence);
		CREATE INDEX idx_truth_events_supersedes
			ON truth_events(supersedes_event_id);
		CREATE TABLE truth_facts (
			event_id TEXT PRIMARY KEY REFERENCES truth_events(id) ON DELETE RESTRICT,
			sequence INTEGER NOT NULL UNIQUE,
			subject_type TEXT NOT NULL,
			subject_id TEXT NOT NULL,
			predicate TEXT NOT NULL,
			value_json TEXT NOT NULL CHECK (json_valid(value_json)),
			value_hash TEXT NOT NULL,
			valid_from_chapter INTEGER NOT NULL,
			valid_to_chapter INTEGER,
			known_from_chapter INTEGER NOT NULL,
			known_to_chapter INTEGER,
			effective_from_chapter INTEGER NOT NULL,
			effective_to_chapter INTEGER,
			authority TEXT NOT NULL,
			authority_rank INTEGER NOT NULL,
			confidence REAL NOT NULL,
			superseded_by_event_id TEXT REFERENCES truth_events(id)
		);
		CREATE INDEX idx_truth_facts_asof
			ON truth_facts(subject_type, subject_id, predicate, effective_from_chapter, effective_to_chapter, authority_rank, sequence);
		CREATE INDEX idx_truth_facts_chapter
			ON truth_facts(effective_from_chapter, effective_to_chapter, sequence);
		CREATE TABLE truth_conflicts (
			id TEXT PRIMARY KEY,
			subject_type TEXT NOT NULL,
			subject_id TEXT NOT NULL,
			predicate TEXT NOT NULL,
			left_event_id TEXT NOT NULL REFERENCES truth_events(id),
			right_event_id TEXT NOT NULL REFERENCES truth_events(id),
			from_chapter INTEGER NOT NULL,
			to_chapter INTEGER,
			reason TEXT NOT NULL,
			UNIQUE(left_event_id, right_event_id, from_chapter)
		);
		CREATE INDEX idx_truth_conflicts_asof
			ON truth_conflicts(subject_type, subject_id, predicate, from_chapter, to_chapter);
		CREATE TABLE truth_projection_meta (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			last_sequence INTEGER NOT NULL DEFAULT 0,
			last_rebuild_from_chapter INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL
		);
		INSERT INTO truth_projection_meta(id, last_sequence, last_rebuild_from_chapter, updated_at)
			VALUES (1, 0, 0, '1970-01-01T00:00:00Z');
		CREATE TRIGGER truth_events_append_only_update
			BEFORE UPDATE ON truth_events BEGIN
				SELECT RAISE(ABORT, 'truth_events is append-only');
			END;
		CREATE TRIGGER truth_events_append_only_delete
			BEFORE DELETE ON truth_events BEGIN
				SELECT RAISE(ABORT, 'truth_events is append-only');
			END;`,
	}
}
''',
    "internal/truthstore/store.go": r'''package truthstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
)

const eventColumns = `sequence, id, idempotency_key, request_hash, kind,
	subject_type, subject_id, predicate, value_json, valid_from_chapter,
	valid_to_chapter, known_from_chapter, known_to_chapter, authority,
	confidence, source_type, source_id, source_revision, source_excerpt,
	supersedes_event_id, created_at, checksum`

type Option func(*Store)

func WithClock(now func() time.Time) Option {
	return func(store *Store) {
		if now != nil {
			store.now = now
		}
	}
}

func WithRandom(reader io.Reader) Option {
	return func(store *Store) {
		if reader != nil {
			store.random = reader
		}
	}
}

type Store struct {
	db      *sql.DB
	now     func() time.Time
	random  io.Reader
	writeMu sync.Mutex
}

func OpenExisting(path string, busyTimeout time.Duration, options ...Option) (*Store, error) {
	db, err := migrate.Open(path, busyTimeout)
	if err != nil {
		return nil, classifyStorageError("truth database could not be opened", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='truth_events'`).Scan(&count); err != nil {
		_ = db.Close()
		return nil, classifyStorageError("truth schema could not be inspected", err)
	}
	if count != 1 {
		_ = db.Close()
		return nil, newError(CodeStorage, "truth schema is not initialized", false, nil)
	}
	store := &Store{db: db, now: time.Now, random: rand.Reader}
	for _, option := range options {
		option(store)
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Append(ctx context.Context, input AppendInput) (AppendResult, error) {
	normalized, err := normalizeAppendInput(input)
	if err != nil {
		return AppendResult{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AppendResult{}, classifyStorageError("truth transaction could not start", err)
	}
	defer tx.Rollback()

	existing, err := eventByIdempotencyTx(ctx, tx, normalized.IdempotencyKey)
	if err == nil {
		if existing.RequestHash != normalized.RequestHash {
			return AppendResult{}, newError(CodeIdempotencyConflict, "Idempotency-Key was already used with a different truth event", false, nil)
		}
		return AppendResult{Event: existing, Replayed: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return AppendResult{}, classifyStorageError("truth idempotency record could not be read", err)
	}

	var target Event
	if normalized.SupersedesEventID != "" {
		target, err = eventByIDTx(ctx, tx, normalized.SupersedesEventID)
		if errors.Is(err, sql.ErrNoRows) {
			return AppendResult{}, newError(CodeNotFound, "superseded truth event was not found", false, err)
		}
		if err != nil {
			return AppendResult{}, classifyStorageError("superseded truth event could not be read", err)
		}
		if target.Kind == EventRetract || !sameKey(target, Event{SubjectType: normalized.SubjectType, SubjectID: normalized.SubjectID, Predicate: normalized.Predicate}) {
			return AppendResult{}, newError(CodeConflict, "supersede target must be an active fact with the same subject and predicate", false, nil)
		}
		newRank, _ := normalized.Authority.rank()
		targetRank, _ := target.Authority.rank()
		if newRank < targetRank {
			return AppendResult{}, newError(CodeAuthority, "lower-authority truth cannot supersede a higher-authority fact", false, nil)
		}
		var already sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT superseded_by_event_id FROM truth_facts WHERE event_id = ?`, target.ID).Scan(&already); errors.Is(err, sql.ErrNoRows) {
			return AppendResult{}, newError(CodeConflict, "supersede target is not projected as a fact", false, err)
		} else if err != nil {
			return AppendResult{}, classifyStorageError("supersede target projection could not be read", err)
		}
		if already.Valid {
			return AppendResult{}, newError(CodeConflict, "truth event was already superseded", false, nil)
		}
	}

	id, err := randomID(s.random)
	if err != nil {
		return AppendResult{}, newError(CodeStorage, "truth event id could not be generated", false, err)
	}
	createdAt := s.now().UTC()
	event := Event{
		ID: id, IdempotencyKey: normalized.IdempotencyKey, RequestHash: normalized.RequestHash,
		Kind: normalized.Kind, SubjectType: normalized.SubjectType, SubjectID: normalized.SubjectID,
		Predicate: normalized.Predicate, Value: normalized.Value, ValidFromChapter: normalized.ValidFromChapter,
		ValidToChapter: normalized.ValidToChapter, KnownFromChapter: normalized.KnownFromChapter,
		KnownToChapter: normalized.KnownToChapter, Authority: normalized.Authority,
		Confidence: normalized.Confidence, Source: normalized.Source,
		SupersedesEventID: normalized.SupersedesEventID, CreatedAt: createdAt,
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO truth_events(
		id, idempotency_key, request_hash, kind, subject_type, subject_id,
		predicate, value_json, valid_from_chapter, valid_to_chapter,
		known_from_chapter, known_to_chapter, authority, confidence,
		source_type, source_id, source_revision, source_excerpt,
		supersedes_event_id, created_at, checksum
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '')`,
		event.ID, event.IdempotencyKey, event.RequestHash, event.Kind,
		event.SubjectType, event.SubjectID, event.Predicate, string(event.Value),
		event.ValidFromChapter, nullableInt(event.ValidToChapter), event.KnownFromChapter,
		nullableInt(event.KnownToChapter), event.Authority, event.Confidence,
		event.Source.Type, event.Source.ID, event.Source.Revision, event.Source.Excerpt,
		nullableString(event.SupersedesEventID), event.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return AppendResult{}, classifyStorageError("truth event could not be appended", err)
	}
	sequence, err := result.LastInsertId()
	if err != nil {
		return AppendResult{}, classifyStorageError("truth event sequence could not be read", err)
	}
	event.Sequence = sequence
	event.Checksum = eventChecksum(event)
	if _, err := tx.ExecContext(ctx, `DROP TRIGGER truth_events_append_only_update`); err != nil {
		return AppendResult{}, classifyStorageError("truth append guard could not be prepared", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE truth_events SET checksum = ? WHERE id = ?`, event.Checksum, event.ID); err != nil {
		return AppendResult{}, classifyStorageError("truth event checksum could not be stored", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TRIGGER truth_events_append_only_update BEFORE UPDATE ON truth_events BEGIN SELECT RAISE(ABORT, 'truth_events is append-only'); END`); err != nil {
		return AppendResult{}, classifyStorageError("truth append guard could not be restored", err)
	}

	effectiveFrom, effectiveTo := effectiveBounds(event.ValidFromChapter, event.ValidToChapter, event.KnownFromChapter, event.KnownToChapter)
	if target.ID != "" {
		if err := closeFactTx(ctx, tx, target.ID, event.ID, effectiveFrom-1); err != nil {
			return AppendResult{}, err
		}
	}
	if event.Kind != EventRetract {
		rank, _ := event.Authority.rank()
		if _, err := tx.ExecContext(ctx, `INSERT INTO truth_facts(
			event_id, sequence, subject_type, subject_id, predicate, value_json,
			value_hash, valid_from_chapter, valid_to_chapter, known_from_chapter,
			known_to_chapter, effective_from_chapter, effective_to_chapter,
			authority, authority_rank, confidence, superseded_by_event_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
			event.ID, event.Sequence, event.SubjectType, event.SubjectID, event.Predicate,
			string(event.Value), normalized.ValueHash, event.ValidFromChapter,
			nullableInt(event.ValidToChapter), event.KnownFromChapter,
			nullableInt(event.KnownToChapter), effectiveFrom, nullableInt(effectiveTo),
			event.Authority, rank, event.Confidence); err != nil {
			return AppendResult{}, classifyStorageError("truth projection could not be written", err)
		}
	}
	if err := recomputeConflictsForKeyTx(ctx, tx, event.SubjectType, event.SubjectID, event.Predicate); err != nil {
		return AppendResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE truth_projection_meta SET last_sequence=?, updated_at=? WHERE id=1`, event.Sequence, createdAt.Format(time.RFC3339Nano)); err != nil {
		return AppendResult{}, classifyStorageError("truth projection metadata could not be updated", err)
	}
	if err := tx.Commit(); err != nil {
		return AppendResult{}, classifyStorageError("truth transaction could not commit", err)
	}
	return AppendResult{Event: event}, nil
}

func (s *Store) State(ctx context.Context, query StateQuery) (StatePage, error) {
	if query.Chapter < 0 {
		return StatePage{}, newError(CodeValidation, "chapter must not be negative", false, nil)
	}
	limit, offset, err := normalizePage(query.Limit, query.Offset)
	if err != nil {
		return StatePage{}, err
	}
	query.SubjectType, err = trimFilter(query.SubjectType, true)
	if err != nil {
		return StatePage{}, err
	}
	query.SubjectID, err = trimFilter(query.SubjectID, false)
	if err != nil {
		return StatePage{}, err
	}
	query.Predicate, err = trimFilter(query.Predicate, true)
	if err != nil {
		return StatePage{}, err
	}
	where := `f.effective_from_chapter <= ? AND (f.effective_to_chapter IS NULL OR f.effective_to_chapter >= ?)`
	args := []any{query.Chapter, query.Chapter}
	where, args = addFactFilters(where, args, query.SubjectType, query.SubjectID, query.Predicate)
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM truth_facts f WHERE `+where, args...).Scan(&total); err != nil {
		return StatePage{}, classifyStorageError("truth state count could not be read", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+prefixedEventColumns("e")+`,
		f.effective_from_chapter, f.effective_to_chapter, f.superseded_by_event_id,
		EXISTS(SELECT 1 FROM truth_conflicts c WHERE
			(c.left_event_id=f.event_id OR c.right_event_id=f.event_id) AND
			c.from_chapter <= ? AND (c.to_chapter IS NULL OR c.to_chapter >= ?))
		FROM truth_facts f JOIN truth_events e ON e.id=f.event_id
		WHERE `+where+`
		ORDER BY f.authority_rank DESC, f.sequence DESC LIMIT ? OFFSET ?`,
		append([]any{query.Chapter, query.Chapter}, append(args, limit, offset)...)...)
	if err != nil {
		return StatePage{}, classifyStorageError("truth state could not be queried", err)
	}
	defer rows.Close()
	page := StatePage{Facts: []Fact{}, Total: total, Limit: limit, Offset: offset}
	for rows.Next() {
		fact, err := scanFact(rows)
		if err != nil {
			return StatePage{}, classifyStorageError("truth fact could not be decoded", err)
		}
		if fact.Conflicted {
			page.Conflicts++
		}
		page.Facts = append(page.Facts, fact)
	}
	if err := rows.Err(); err != nil {
		return StatePage{}, classifyStorageError("truth state iteration failed", err)
	}
	if offset+len(page.Facts) < total {
		next := offset + len(page.Facts)
		page.NextOffset = &next
	}
	return page, nil
}

func (s *Store) StateMany(ctx context.Context, queries []StateQuery) ([]StatePage, error) {
	if len(queries) > 100 {
		return nil, newError(CodeValidation, "at most 100 truth queries are allowed per batch", false, nil)
	}
	result := make([]StatePage, 0, len(queries))
	for _, query := range queries {
		page, err := s.State(ctx, query)
		if err != nil {
			return nil, err
		}
		result = append(result, page)
	}
	return result, nil
}

func (s *Store) Events(ctx context.Context, afterSequence int64, limit int) (EventPage, error) {
	if afterSequence < 0 {
		return EventPage{}, newError(CodeValidation, "after_sequence must not be negative", false, nil)
	}
	limit, _, err := normalizePage(limit, 0)
	if err != nil {
		return EventPage{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+eventColumns+` FROM truth_events WHERE sequence > ? ORDER BY sequence LIMIT ?`, afterSequence, limit+1)
	if err != nil {
		return EventPage{}, classifyStorageError("truth events could not be queried", err)
	}
	defer rows.Close()
	page := EventPage{Events: []Event{}, AfterSequence: afterSequence, Limit: limit}
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return EventPage{}, classifyStorageError("truth event could not be decoded", err)
		}
		page.Events = append(page.Events, event)
	}
	if err := rows.Err(); err != nil {
		return EventPage{}, classifyStorageError("truth event iteration failed", err)
	}
	if len(page.Events) > limit {
		page.Events = page.Events[:limit]
		next := page.Events[len(page.Events)-1].Sequence
		page.NextSequence = &next
	}
	return page, nil
}

func (s *Store) Conflicts(ctx context.Context, query ConflictQuery) (ConflictPage, error) {
	limit, offset, err := normalizePage(query.Limit, query.Offset)
	if err != nil {
		return ConflictPage{}, err
	}
	query.SubjectType, err = trimFilter(query.SubjectType, true)
	if err != nil {
		return ConflictPage{}, err
	}
	query.SubjectID, err = trimFilter(query.SubjectID, false)
	if err != nil {
		return ConflictPage{}, err
	}
	query.Predicate, err = trimFilter(query.Predicate, true)
	if err != nil {
		return ConflictPage{}, err
	}
	where := "1=1"
	args := []any{}
	if query.Chapter != nil {
		if *query.Chapter < 0 {
			return ConflictPage{}, newError(CodeValidation, "chapter must not be negative", false, nil)
		}
		where += ` AND from_chapter <= ? AND (to_chapter IS NULL OR to_chapter >= ?)`
		args = append(args, *query.Chapter, *query.Chapter)
	}
	where, args = addConflictFilters(where, args, query.SubjectType, query.SubjectID, query.Predicate)
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM truth_conflicts WHERE `+where, args...).Scan(&total); err != nil {
		return ConflictPage{}, classifyStorageError("truth conflict count could not be read", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, subject_type, subject_id, predicate,
		left_event_id, right_event_id, from_chapter, to_chapter, reason
		FROM truth_conflicts WHERE `+where+` ORDER BY from_chapter, id LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return ConflictPage{}, classifyStorageError("truth conflicts could not be queried", err)
	}
	defer rows.Close()
	page := ConflictPage{Conflicts: []Conflict{}, Total: total, Limit: limit, Offset: offset}
	for rows.Next() {
		var conflict Conflict
		var to sql.NullInt64
		if err := rows.Scan(&conflict.ID, &conflict.SubjectType, &conflict.SubjectID, &conflict.Predicate,
			&conflict.LeftEventID, &conflict.RightEventID, &conflict.FromChapter, &to, &conflict.Reason); err != nil {
			return ConflictPage{}, classifyStorageError("truth conflict could not be decoded", err)
		}
		conflict.ToChapter = intPointer(to)
		page.Conflicts = append(page.Conflicts, conflict)
	}
	if err := rows.Err(); err != nil {
		return ConflictPage{}, classifyStorageError("truth conflict iteration failed", err)
	}
	if offset+len(page.Conflicts) < total {
		next := offset + len(page.Conflicts)
		page.NextOffset = &next
	}
	return page, nil
}

func eventByIdempotencyTx(ctx context.Context, tx *sql.Tx, key string) (Event, error) {
	return scanEvent(tx.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM truth_events WHERE idempotency_key=?`, key))
}

func eventByIDTx(ctx context.Context, tx *sql.Tx, id string) (Event, error) {
	return scanEvent(tx.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM truth_events WHERE id=?`, id))
}

type scanner interface{ Scan(dest ...any) error }

func scanEvent(row scanner) (Event, error) {
	var event Event
	var value, created string
	var validTo, knownTo sql.NullInt64
	var supersedes sql.NullString
	if err := row.Scan(&event.Sequence, &event.ID, &event.IdempotencyKey, &event.RequestHash,
		&event.Kind, &event.SubjectType, &event.SubjectID, &event.Predicate, &value,
		&event.ValidFromChapter, &validTo, &event.KnownFromChapter, &knownTo,
		&event.Authority, &event.Confidence, &event.Source.Type, &event.Source.ID,
		&event.Source.Revision, &event.Source.Excerpt, &supersedes, &created,
		&event.Checksum); err != nil {
		return Event{}, err
	}
	event.Value = json.RawMessage(value)
	event.ValidToChapter = intPointer(validTo)
	event.KnownToChapter = intPointer(knownTo)
	if supersedes.Valid {
		event.SupersedesEventID = supersedes.String
	}
	parsed, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Event{}, err
	}
	event.CreatedAt = parsed
	return event, nil
}

func scanFact(row scanner) (Fact, error) {
	var fact Fact
	var value, created string
	var validTo, knownTo, effectiveTo sql.NullInt64
	var supersedes, supersededBy sql.NullString
	var conflicted int
	if err := row.Scan(&fact.Sequence, &fact.ID, &fact.IdempotencyKey, &fact.RequestHash,
		&fact.Kind, &fact.SubjectType, &fact.SubjectID, &fact.Predicate, &value,
		&fact.ValidFromChapter, &validTo, &fact.KnownFromChapter, &knownTo,
		&fact.Authority, &fact.Confidence, &fact.Source.Type, &fact.Source.ID,
		&fact.Source.Revision, &fact.Source.Excerpt, &supersedes, &created,
		&fact.Checksum, &fact.EffectiveFromChapter, &effectiveTo, &supersededBy,
		&conflicted); err != nil {
		return Fact{}, err
	}
	fact.Value = json.RawMessage(value)
	fact.ValidToChapter = intPointer(validTo)
	fact.KnownToChapter = intPointer(knownTo)
	fact.EffectiveToChapter = intPointer(effectiveTo)
	if supersedes.Valid {
		fact.SupersedesEventID = supersedes.String
	}
	if supersededBy.Valid {
		fact.SupersededByEventID = supersededBy.String
	}
	parsed, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Fact{}, err
	}
	fact.CreatedAt = parsed
	fact.Conflicted = conflicted != 0
	return fact, nil
}

func closeFactTx(ctx context.Context, tx *sql.Tx, targetID, byID string, end int) error {
	result, err := tx.ExecContext(ctx, `UPDATE truth_facts SET effective_to_chapter=?, superseded_by_event_id=? WHERE event_id=?`, end, byID, targetID)
	if err != nil {
		return classifyStorageError("superseded truth projection could not be closed", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return newError(CodeConflict, "supersede target is not an active projected fact", false, err)
	}
	return nil
}

func recomputeConflictsForKeyTx(ctx context.Context, tx *sql.Tx, subjectType, subjectID, predicate string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM truth_conflicts WHERE subject_type=? AND subject_id=? AND predicate=?`, subjectType, subjectID, predicate); err != nil {
		return classifyStorageError("truth conflicts could not be reset", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT event_id, value_hash, effective_from_chapter, effective_to_chapter
		FROM truth_facts WHERE subject_type=? AND subject_id=? AND predicate=? ORDER BY sequence`, subjectType, subjectID, predicate)
	if err != nil {
		return classifyStorageError("truth conflict candidates could not be read", err)
	}
	type candidate struct {
		id, valueHash string
		from int
		to *int
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		var to sql.NullInt64
		if err := rows.Scan(&item.id, &item.valueHash, &item.from, &to); err != nil {
			_ = rows.Close()
			return classifyStorageError("truth conflict candidate could not be decoded", err)
		}
		item.to = intPointer(to)
		candidates = append(candidates, item)
	}
	if err := rows.Close(); err != nil {
		return classifyStorageError("truth conflict candidates could not be closed", err)
	}
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			left, right := candidates[i], candidates[j]
			if left.valueHash == right.valueHash {
				continue
			}
			from, to, ok := intervalIntersection(left.from, left.to, right.from, right.to)
			if !ok {
				continue
			}
			conflict := newConflict(subjectType, subjectID, predicate, left.id, right.id, from, to)
			if _, err := tx.ExecContext(ctx, `INSERT INTO truth_conflicts(id, subject_type, subject_id, predicate,
				left_event_id, right_event_id, from_chapter, to_chapter, reason) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				conflict.ID, conflict.SubjectType, conflict.SubjectID, conflict.Predicate,
				conflict.LeftEventID, conflict.RightEventID, conflict.FromChapter,
				nullableInt(conflict.ToChapter), conflict.Reason); err != nil {
				return classifyStorageError("truth conflict could not be projected", err)
			}
		}
	}
	return nil
}

func newConflict(subjectType, subjectID, predicate, leftID, rightID string, from int, to *int) Conflict {
	if rightID < leftID {
		leftID, rightID = rightID, leftID
	}
	toValue := "open"
	if to != nil {
		toValue = fmt.Sprint(*to)
	}
	sum := sha256Text(strings.Join([]string{subjectType, subjectID, predicate, leftID, rightID, fmt.Sprint(from), toValue}, "\x00"))
	return Conflict{ID: sum, SubjectType: subjectType, SubjectID: subjectID, Predicate: predicate,
		LeftEventID: leftID, RightEventID: rightID, FromChapter: from, ToChapter: to,
		Reason: "overlapping distinct values without explicit supersede"}
}

func intervalIntersection(leftFrom int, leftTo *int, rightFrom int, rightTo *int) (int, *int, bool) {
	from := leftFrom
	if rightFrom > from {
		from = rightFrom
	}
	var to *int
	if leftTo != nil {
		value := *leftTo
		to = &value
	}
	if rightTo != nil && (to == nil || *rightTo < *to) {
		value := *rightTo
		to = &value
	}
	return from, to, to == nil || *to >= from
}

func addFactFilters(where string, args []any, subjectType, subjectID, predicate string) (string, []any) {
	if subjectType != "" {
		where += " AND f.subject_type=?"
		args = append(args, subjectType)
	}
	if subjectID != "" {
		where += " AND f.subject_id=?"
		args = append(args, subjectID)
	}
	if predicate != "" {
		where += " AND f.predicate=?"
		args = append(args, predicate)
	}
	return where, args
}

func addConflictFilters(where string, args []any, subjectType, subjectID, predicate string) (string, []any) {
	if subjectType != "" {
		where += " AND subject_type=?"
		args = append(args, subjectType)
	}
	if subjectID != "" {
		where += " AND subject_id=?"
		args = append(args, subjectID)
	}
	if predicate != "" {
		where += " AND predicate=?"
		args = append(args, predicate)
	}
	return where, args
}

func prefixedEventColumns(prefix string) string {
	parts := strings.Split(eventColumns, ",")
	for index, part := range parts {
		parts[index] = prefix + "." + strings.TrimSpace(part)
	}
	return strings.Join(parts, ", ")
}

func randomID(reader io.Reader) (string, error) {
	buffer := make([]byte, 16)
	if _, err := io.ReadFull(reader, buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func intPointer(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	converted := int(value.Int64)
	return &converted
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func sha256Text(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func classifyStorageError(message string, err error) error {
	lower := strings.ToLower(fmt.Sprint(err))
	if strings.Contains(lower, "locked") || strings.Contains(lower, "busy") || strings.Contains(lower, "timeout") {
		return newError(CodeBusy, message, true, err)
	}
	return newError(CodeStorage, message, false, err)
}
''',
    "internal/truthstore/rebuild.go": r'''package truthstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type projectedFact struct {
	Event
	ValueHash            string
	EffectiveFromChapter int
	EffectiveToChapter   *int
	SupersededByEventID  string
}

func (s *Store) Rebuild(ctx context.Context, fromChapter int) (RebuildResult, error) {
	if fromChapter < 0 {
		return RebuildResult{}, newError(CodeValidation, "from_chapter must not be negative", false, nil)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RebuildResult{}, classifyStorageError("truth rebuild transaction could not start", err)
	}
	defer tx.Rollback()
	events, err := allEventsTx(ctx, tx)
	if err != nil {
		return RebuildResult{}, err
	}
	facts, conflicts, err := projectEvents(events)
	if err != nil {
		return RebuildResult{}, err
	}
	if fromChapter == 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM truth_conflicts; DELETE FROM truth_facts`); err != nil {
			return RebuildResult{}, classifyStorageError("truth projection could not be cleared", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `DELETE FROM truth_conflicts WHERE to_chapter IS NULL OR to_chapter >= ?`, fromChapter); err != nil {
			return RebuildResult{}, classifyStorageError("truth conflicts could not be cleared from boundary", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM truth_facts WHERE effective_to_chapter IS NULL OR effective_to_chapter >= ?`, fromChapter); err != nil {
			return RebuildResult{}, classifyStorageError("truth facts could not be cleared from boundary", err)
		}
	}
	factsWritten := 0
	for _, fact := range facts {
		if fromChapter > 0 && fact.EffectiveToChapter != nil && *fact.EffectiveToChapter < fromChapter {
			continue
		}
		rank, _ := fact.Authority.rank()
		if _, err := tx.ExecContext(ctx, `INSERT INTO truth_facts(event_id, sequence, subject_type,
			subject_id, predicate, value_json, value_hash, valid_from_chapter,
			valid_to_chapter, known_from_chapter, known_to_chapter,
			effective_from_chapter, effective_to_chapter, authority, authority_rank,
			confidence, superseded_by_event_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			fact.ID, fact.Sequence, fact.SubjectType, fact.SubjectID, fact.Predicate,
			string(fact.Value), fact.ValueHash, fact.ValidFromChapter,
			nullableInt(fact.ValidToChapter), fact.KnownFromChapter,
			nullableInt(fact.KnownToChapter), fact.EffectiveFromChapter,
			nullableInt(fact.EffectiveToChapter), fact.Authority, rank,
			fact.Confidence, nullableString(fact.SupersededByEventID)); err != nil {
			return RebuildResult{}, classifyStorageError("truth fact could not be rebuilt", err)
		}
		factsWritten++
	}
	conflictsWritten := 0
	for _, conflict := range conflicts {
		if fromChapter > 0 && conflict.ToChapter != nil && *conflict.ToChapter < fromChapter {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO truth_conflicts(id, subject_type, subject_id,
			predicate, left_event_id, right_event_id, from_chapter, to_chapter, reason)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, conflict.ID, conflict.SubjectType,
			conflict.SubjectID, conflict.Predicate, conflict.LeftEventID,
			conflict.RightEventID, conflict.FromChapter, nullableInt(conflict.ToChapter),
			conflict.Reason); err != nil {
			return RebuildResult{}, classifyStorageError("truth conflict could not be rebuilt", err)
		}
		conflictsWritten++
	}
	lastSequence := int64(0)
	if len(events) > 0 {
		lastSequence = events[len(events)-1].Sequence
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE truth_projection_meta SET last_sequence=?, last_rebuild_from_chapter=?, updated_at=? WHERE id=1`, lastSequence, fromChapter, now); err != nil {
		return RebuildResult{}, classifyStorageError("truth projection metadata could not be rebuilt", err)
	}
	if err := tx.Commit(); err != nil {
		return RebuildResult{}, classifyStorageError("truth rebuild could not commit", err)
	}
	digest := projectionDigest(facts, conflicts)
	return RebuildResult{FromChapter: fromChapter, EventsReplayed: len(events), FactsProjected: factsWritten,
		ConflictsProjected: conflictsWritten, ProjectionDigest: digest}, nil
}

func (s *Store) Verify(ctx context.Context) (VerifyResult, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return VerifyResult{}, classifyStorageError("truth verification could not start", err)
	}
	defer tx.Rollback()
	events, err := allEventsTx(ctx, tx)
	if err != nil {
		return VerifyResult{}, err
	}
	expectedFacts, expectedConflicts, projectionErr := projectEvents(events)
	result := VerifyResult{Events: len(events), Violations: []string{}}
	if projectionErr != nil {
		result.Violations = append(result.Violations, projectionErr.Error())
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM truth_facts`).Scan(&result.Facts); err != nil {
		return VerifyResult{}, classifyStorageError("truth fact count could not be verified", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM truth_conflicts`).Scan(&result.Conflicts); err != nil {
		return VerifyResult{}, classifyStorageError("truth conflict count could not be verified", err)
	}
	if projectionErr == nil {
		if result.Facts != len(expectedFacts) {
			result.Violations = append(result.Violations, fmt.Sprintf("fact projection count mismatch: have %d want %d", result.Facts, len(expectedFacts)))
		}
		if result.Conflicts != len(expectedConflicts) {
			result.Violations = append(result.Violations, fmt.Sprintf("conflict projection count mismatch: have %d want %d", result.Conflicts, len(expectedConflicts)))
		}
		expectedDigest := projectionDigest(expectedFacts, expectedConflicts)
		actualDigest, err := actualProjectionDigest(ctx, tx)
		if err != nil {
			return VerifyResult{}, err
		}
		result.ProjectionDigest = actualDigest
		if expectedDigest != actualDigest {
			result.Violations = append(result.Violations, "projection digest mismatch")
		}
	}
	result.Valid = len(result.Violations) == 0
	if err := tx.Commit(); err != nil {
		return VerifyResult{}, classifyStorageError("truth verification could not finish", err)
	}
	return result, nil
}

func allEventsTx(ctx context.Context, tx *sql.Tx) ([]Event, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+eventColumns+` FROM truth_events ORDER BY sequence`)
	if err != nil {
		return nil, classifyStorageError("truth events could not be read for rebuild", err)
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, classifyStorageError("truth event could not be decoded for rebuild", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyStorageError("truth rebuild event iteration failed", err)
	}
	return events, nil
}

func projectEvents(events []Event) ([]projectedFact, []Conflict, error) {
	facts := make([]projectedFact, 0, len(events))
	byID := make(map[string]*projectedFact, len(events))
	for _, event := range events {
		if event.Checksum == "" || eventChecksum(event) != event.Checksum {
			return nil, nil, newError(CodeCorrupt, fmt.Sprintf("truth event %s failed checksum verification", event.ID), false, nil)
		}
		if event.SupersedesEventID != "" {
			target := byID[event.SupersedesEventID]
			if target == nil || target.Kind == EventRetract || !sameKey(target.Event, event) {
				return nil, nil, newError(CodeCorrupt, fmt.Sprintf("truth event %s has an invalid supersede target", event.ID), false, nil)
			}
			newRank, newOK := event.Authority.rank()
			targetRank, targetOK := target.Authority.rank()
			if !newOK || !targetOK || newRank < targetRank {
				return nil, nil, newError(CodeCorrupt, fmt.Sprintf("truth event %s violates authority ordering", event.ID), false, nil)
			}
			if target.SupersededByEventID != "" {
				return nil, nil, newError(CodeCorrupt, fmt.Sprintf("truth event %s supersedes an already closed fact", event.ID), false, nil)
			}
			effectiveFrom, _ := effectiveBounds(event.ValidFromChapter, event.ValidToChapter, event.KnownFromChapter, event.KnownToChapter)
			end := effectiveFrom - 1
			target.EffectiveToChapter = &end
			target.SupersededByEventID = event.ID
		}
		if event.Kind == EventRetract {
			continue
		}
		_, valueHash, err := canonicalizeValue(event.Kind, event.Value)
		if err != nil {
			return nil, nil, newError(CodeCorrupt, fmt.Sprintf("truth event %s has invalid value JSON", event.ID), false, err)
		}
		from, to := effectiveBounds(event.ValidFromChapter, event.ValidToChapter, event.KnownFromChapter, event.KnownToChapter)
		fact := projectedFact{Event: event, ValueHash: valueHash, EffectiveFromChapter: from, EffectiveToChapter: to}
		facts = append(facts, fact)
		byID[event.ID] = &facts[len(facts)-1]
	}
	groups := map[string][]projectedFact{}
	for _, fact := range facts {
		key := strings.Join([]string{fact.SubjectType, fact.SubjectID, fact.Predicate}, "\x00")
		groups[key] = append(groups[key], fact)
	}
	conflicts := []Conflict{}
	for _, group := range groups {
		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				left, right := group[i], group[j]
				if left.ValueHash == right.ValueHash {
					continue
				}
				from, to, ok := intervalIntersection(left.EffectiveFromChapter, left.EffectiveToChapter, right.EffectiveFromChapter, right.EffectiveToChapter)
				if !ok {
					continue
				}
				conflicts = append(conflicts, newConflict(left.SubjectType, left.SubjectID, left.Predicate, left.ID, right.ID, from, to))
			}
		}
	}
	sort.Slice(conflicts, func(i, j int) bool { return conflicts[i].ID < conflicts[j].ID })
	return facts, conflicts, nil
}

func projectionDigest(facts []projectedFact, conflicts []Conflict) string {
	factRows := make([]string, 0, len(facts))
	for _, fact := range facts {
		factRows = append(factRows, projectionFactRow(fact.ID, fact.Sequence, fact.ValueHash, fact.EffectiveFromChapter, fact.EffectiveToChapter, fact.SupersededByEventID))
	}
	sort.Strings(factRows)
	conflictRows := make([]string, 0, len(conflicts))
	for _, conflict := range conflicts {
		conflictRows = append(conflictRows, projectionConflictRow(conflict))
	}
	sort.Strings(conflictRows)
	return sha256Text(strings.Join(append(factRows, conflictRows...), "\n"))
}

func actualProjectionDigest(ctx context.Context, tx *sql.Tx) (string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT event_id, sequence, value_hash, effective_from_chapter,
		effective_to_chapter, superseded_by_event_id FROM truth_facts ORDER BY event_id`)
	if err != nil {
		return "", classifyStorageError("truth projection digest could not be read", err)
	}
	factRows := []string{}
	for rows.Next() {
		var id, valueHash string
		var sequence int64
		var from int
		var to sql.NullInt64
		var by sql.NullString
		if err := rows.Scan(&id, &sequence, &valueHash, &from, &to, &by); err != nil {
			_ = rows.Close()
			return "", classifyStorageError("truth projection digest row could not be decoded", err)
		}
		factRows = append(factRows, projectionFactRow(id, sequence, valueHash, from, intPointer(to), by.String))
	}
	if err := rows.Close(); err != nil {
		return "", classifyStorageError("truth projection digest rows could not be closed", err)
	}
	rows, err = tx.QueryContext(ctx, `SELECT id, subject_type, subject_id, predicate, left_event_id,
		right_event_id, from_chapter, to_chapter, reason FROM truth_conflicts ORDER BY id`)
	if err != nil {
		return "", classifyStorageError("truth conflict digest could not be read", err)
	}
	conflictRows := []string{}
	for rows.Next() {
		var conflict Conflict
		var to sql.NullInt64
		if err := rows.Scan(&conflict.ID, &conflict.SubjectType, &conflict.SubjectID, &conflict.Predicate,
			&conflict.LeftEventID, &conflict.RightEventID, &conflict.FromChapter, &to, &conflict.Reason); err != nil {
			_ = rows.Close()
			return "", classifyStorageError("truth conflict digest row could not be decoded", err)
		}
		conflict.ToChapter = intPointer(to)
		conflictRows = append(conflictRows, projectionConflictRow(conflict))
	}
	if err := rows.Close(); err != nil {
		return "", classifyStorageError("truth conflict digest rows could not be closed", err)
	}
	return sha256Text(strings.Join(append(factRows, conflictRows...), "\n")), nil
}

func projectionFactRow(id string, sequence int64, valueHash string, from int, to *int, by string) string {
	toValue := "open"
	if to != nil {
		toValue = fmt.Sprint(*to)
	}
	return strings.Join([]string{"fact", id, fmt.Sprint(sequence), valueHash, fmt.Sprint(from), toValue, by}, "|")
}

func projectionConflictRow(conflict Conflict) string {
	toValue := "open"
	if conflict.ToChapter != nil {
		toValue = fmt.Sprint(*conflict.ToChapter)
	}
	return strings.Join([]string{"conflict", conflict.ID, conflict.LeftEventID, conflict.RightEventID,
		fmt.Sprint(conflict.FromChapter), toValue, conflict.Reason}, "|")
}

var _ = json.Valid
var _ = errors.Is
''',
    "internal/truthstore/store_test.go": r'''package truthstore

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "truth.db")
	if err := (migrate.Runner{Path: path, Migrations: []migrate.Migration{Migration()}}).Run(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store, err := OpenExisting(path, 500*time.Millisecond,
		WithClock(func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) }),
		WithRandom(bytes.NewReader(bytes.Repeat([]byte{0x42}, 4096))))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func input(key, subject, predicate string, value any, chapter int) AppendInput {
	data, _ := json.Marshal(value)
	return AppendInput{IdempotencyKey: key, Kind: EventAssert, SubjectType: "character",
		SubjectID: subject, Predicate: predicate, Value: data, ValidFromChapter: chapter,
		KnownFromChapter: chapter, Authority: AuthorityCanon, Confidence: 1,
		Source: Source{Type: "chapter", ID: fmt.Sprintf("chapter-%d", chapter)}}
}

func TestChapterQueriesDoNotLeakFutureTruth(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.Append(ctx, input("future-location", "lin", "location.current", "North Gate", 10)); err != nil {
		t.Fatal(err)
	}
	before, err := store.State(ctx, StateQuery{Chapter: 9, SubjectID: "lin"})
	if err != nil {
		t.Fatal(err)
	}
	if before.Total != 0 {
		t.Fatalf("chapter 9 leaked %d future facts", before.Total)
	}
	after, err := store.State(ctx, StateQuery{Chapter: 10, SubjectID: "lin"})
	if err != nil {
		t.Fatal(err)
	}
	if after.Total != 1 || string(after.Facts[0].Value) != `"North Gate"` {
		t.Fatalf("chapter 10 state = %#v", after)
	}
}

func TestKnowledgeBoundaryPreventsRetroactiveLeakage(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	entry := input("secret-origin", "lin", "origin.city", "Old Capital", 1)
	entry.KnownFromChapter = 8
	if _, err := store.Append(ctx, entry); err != nil {
		t.Fatal(err)
	}
	atSeven, _ := store.State(ctx, StateQuery{Chapter: 7, SubjectID: "lin"})
	atEight, _ := store.State(ctx, StateQuery{Chapter: 8, SubjectID: "lin"})
	if atSeven.Total != 0 || atEight.Total != 1 {
		t.Fatalf("knowledge boundary totals = %d, %d", atSeven.Total, atEight.Total)
	}
}

func TestInventoryFactsRespectTemporalRanges(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	until := 5
	entry := input("inventory-sword", "lin", "inventory.sword", map[string]any{"count": 1}, 2)
	entry.ValidToChapter = &until
	if _, err := store.Append(ctx, entry); err != nil {
		t.Fatal(err)
	}
	atFive, _ := store.State(ctx, StateQuery{Chapter: 5, Predicate: "inventory.sword"})
	atSix, _ := store.State(ctx, StateQuery{Chapter: 6, Predicate: "inventory.sword"})
	if atFive.Total != 1 || atSix.Total != 0 {
		t.Fatalf("inventory temporal totals = %d, %d", atFive.Total, atSix.Total)
	}
}

func TestConcurrentIdempotentAppendCreatesOneEvent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	entry := input("same-request", "lin", "status.alive", true, 1)
	const workers = 16
	var wg sync.WaitGroup
	ids := make(chan string, workers)
	errs := make(chan error, workers)
	for index := 0; index < workers; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := store.Append(ctx, entry)
			if err != nil {
				errs <- err
				return
			}
			ids <- result.Event.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatalf("append: %v", err)
	}
	var first string
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatalf("idempotent append ids differ: %s != %s", id, first)
		}
	}
	page, err := store.Events(ctx, 0, 100)
	if err != nil || len(page.Events) != 1 {
		t.Fatalf("events = %#v, %v", page, err)
	}
	changed := entry
	changed.Value = json.RawMessage(`false`)
	_, err = store.Append(ctx, changed)
	var storeErr *Error
	if !errors.As(err, &storeErr) || storeErr.Code != CodeIdempotencyConflict {
		t.Fatalf("changed replay error = %v", err)
	}
}

func TestConflictRequiresExplicitAuthorizedSupersede(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	first := input("canon-eye", "lin", "appearance.eye_color", "black", 1)
	first.Authority = AuthorityHuman
	firstResult, err := store.Append(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	second := input("agent-eye", "lin", "appearance.eye_color", "blue", 2)
	second.Authority = AuthorityAgent
	if _, err := store.Append(ctx, second); err != nil {
		t.Fatal(err)
	}
	state, _ := store.State(ctx, StateQuery{Chapter: 2, SubjectID: "lin", Predicate: "appearance.eye_color"})
	conflicts, _ := store.Conflicts(ctx, ConflictQuery{Chapter: intRef(2)})
	if state.Total != 2 || conflicts.Total != 1 {
		t.Fatalf("state/conflicts = %d/%d", state.Total, conflicts.Total)
	}
	lower := input("bad-supersede", "lin", "appearance.eye_color", "green", 3)
	lower.Kind = EventSupersede
	lower.Authority = AuthorityAgent
	lower.SupersedesEventID = firstResult.Event.ID
	_, err = store.Append(ctx, lower)
	var storeErr *Error
	if !errors.As(err, &storeErr) || storeErr.Code != CodeAuthority {
		t.Fatalf("lower-authority supersede error = %v", err)
	}
}

func TestAuthorizedSupersedeClosesOldFact(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	first := input("old-title", "book", "title.current", "Old", 1)
	first.SubjectType = "project"
	first.Authority = AuthorityCanon
	created, err := store.Append(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	replacement := input("new-title", "book", "title.current", "New", 5)
	replacement.SubjectType = "project"
	replacement.Kind = EventSupersede
	replacement.Authority = AuthorityHuman
	replacement.SupersedesEventID = created.Event.ID
	if _, err := store.Append(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	atFour, _ := store.State(ctx, StateQuery{Chapter: 4, SubjectID: "book"})
	atFive, _ := store.State(ctx, StateQuery{Chapter: 5, SubjectID: "book"})
	if atFour.Total != 1 || string(atFour.Facts[0].Value) != `"Old"` || atFive.Total != 1 || string(atFive.Facts[0].Value) != `"New"` {
		t.Fatalf("supersede states = %#v / %#v", atFour, atFive)
	}
}

func TestBoundedRebuildRestoresProjectionAndDigest(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for chapter := 1; chapter <= 8; chapter++ {
		if _, err := store.Append(ctx, input(fmt.Sprintf("event-%d", chapter), fmt.Sprintf("c-%d", chapter), "status.marker", chapter, chapter)); err != nil {
			t.Fatal(err)
		}
	}
	before, err := store.Verify(ctx)
	if err != nil || !before.Valid {
		t.Fatalf("before verify = %#v, %v", before, err)
	}
	if _, err := store.db.Exec(`DELETE FROM truth_facts WHERE effective_from_chapter >= 5`); err != nil {
		t.Fatal(err)
	}
	broken, _ := store.Verify(ctx)
	if broken.Valid {
		t.Fatal("expected broken projection")
	}
	rebuilt, err := store.Rebuild(ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	after, err := store.Verify(ctx)
	if err != nil || !after.Valid || after.ProjectionDigest != before.ProjectionDigest || rebuilt.FromChapter != 5 {
		t.Fatalf("rebuild/verify = %#v / %#v / %v", rebuilt, after, err)
	}
}

func TestEventLogIsAppendOnly(t *testing.T) {
	store := newTestStore(t)
	result, err := store.Append(context.Background(), input("immutable", "lin", "status.alive", true, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE truth_events SET value_json='false' WHERE id=?`, result.Event.ID); err == nil {
		t.Fatal("truth event update unexpectedly succeeded")
	}
	if _, err := store.db.Exec(`DELETE FROM truth_events WHERE id=?`, result.Event.ID); err == nil {
		t.Fatal("truth event delete unexpectedly succeeded")
	}
}

func TestBusyStoreReturnsRetryableError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "busy.db")
	if err := (migrate.Runner{Path: path, Migrations: []migrate.Migration{Migration()}}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	first, _ := OpenExisting(path, 50*time.Millisecond)
	second, _ := OpenExisting(path, 50*time.Millisecond)
	defer first.Close()
	defer second.Close()
	tx, err := first.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE truth_projection_meta SET last_sequence=last_sequence WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	_, err = second.Append(context.Background(), input("busy", "lin", "status.alive", true, 1))
	var storeErr *Error
	if !errors.As(err, &storeErr) || !storeErr.Retryable || storeErr.Code != CodeBusy {
		_ = tx.Rollback()
		t.Fatalf("busy error = %#v", err)
	}
	_ = tx.Rollback()
	if _, err := second.Append(context.Background(), input("after-busy", "lin", "status.alive", true, 1)); err != nil {
		t.Fatalf("append after rollback: %v", err)
	}
}

func intRef(value int) *int { return &value }

var _ *sql.DB
''',
    "internal/truthstore/scale_test.go": r'''package truthstore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHundredThousandFactTemporalQueryUsesIndex(t *testing.T) {
	if os.Getenv("NOVELFORGE_SCALE_TEST") != "1" {
		t.Skip("set NOVELFORGE_SCALE_TEST=1 for the 100k projection gate")
	}
	store := newTestStore(t)
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	eventStatement, err := tx.Prepare(`INSERT INTO truth_events(id, idempotency_key, request_hash,
		kind, subject_type, subject_id, predicate, value_json, valid_from_chapter,
		known_from_chapter, authority, confidence, source_type, source_id, created_at, checksum)
		VALUES (?, ?, 'scale', 'assert', 'character', ?, 'status.scale', ?, ?, ?, 'canon', 1, 'test', ?, ?, 'scale')`)
	if err != nil {
		t.Fatal(err)
	}
	factStatement, err := tx.Prepare(`INSERT INTO truth_facts(event_id, sequence, subject_type,
		subject_id, predicate, value_json, value_hash, valid_from_chapter,
		known_from_chapter, effective_from_chapter, authority, authority_rank, confidence)
		VALUES (?, ?, 'character', ?, 'status.scale', ?, ?, ?, ?, ?, 'canon', 30, 1)`)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	for index := 0; index < 100_000; index++ {
		id := fmt.Sprintf("scale-%06d", index)
		subject := fmt.Sprintf("subject-%04d", index%1000)
		chapter := index % 500
		value := fmt.Sprintf("%d", index)
		if _, err := eventStatement.Exec(id, "key-"+id, subject, value, chapter, chapter, id, created); err != nil {
			t.Fatalf("event %d: %v", index, err)
		}
		if _, err := factStatement.Exec(id, index+1, subject, value, sha256Text(value), chapter, chapter, chapter); err != nil {
			t.Fatalf("fact %d: %v", index, err)
		}
	}
	_ = eventStatement.Close()
	_ = factStatement.Close()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	rows, err := store.db.Query(`EXPLAIN QUERY PLAN SELECT event_id FROM truth_facts
		WHERE subject_type='character' AND subject_id='subject-0042' AND predicate='status.scale'
		AND effective_from_chapter <= 300 AND (effective_to_chapter IS NULL OR effective_to_chapter >= 300)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	plan := ""
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan += detail + "\n"
	}
	if !strings.Contains(plan, "idx_truth_facts_asof") {
		t.Fatalf("temporal query did not use idx_truth_facts_asof:\n%s", plan)
	}
	page, err := store.State(context.Background(), StateQuery{Chapter: 300, SubjectType: "character", SubjectID: "subject-0042", Predicate: "status.scale", Limit: 500})
	if err != nil || page.Total == 0 {
		t.Fatalf("100k state query = %d, %v", page.Total, err)
	}
}

var _ sql.Result
''',
    "internal/project/truth_migration.go": r'''package project

import "github.com/voocel/ainovel-cli/internal/truthstore"

func init() {
	projectMigrations = append(projectMigrations, truthstore.Migration())
}
''',
    "internal/project/truth.go": r'''package project

import (
	"context"
	"path/filepath"
	"time"

	"github.com/voocel/ainovel-cli/internal/truthstore"
)

func (r *Repository) OpenTruthStore(ctx context.Context, id string) (*truthstore.Store, error) {
	entry, err := r.find(id)
	if err != nil {
		return nil, err
	}
	root := ENTRY_ROOT
	if err := r.initializeProjectDatabase(ctx, root); err != nil {
		return nil, err
	}
	store, err := truthstore.OpenExisting(filepath.Join(root, projectDatabaseRelative), 5*time.Second)
	if err != nil {
		return nil, newError("PROJECT_TRUTH_STORE_ERROR", "project truth store could not be opened", err)
	}
	return store, nil
}
''',
    "internal/project/truth_test.go": r'''package project

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/voocel/ainovel-cli/internal/truthstore"
)

func TestProjectDatabaseMigratesAndPersistsTruth(t *testing.T) {
	ctx := context.Background()
	repository, err := NewRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := repository.Create(ctx, CreateInput{Title: "Temporal Project"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := repository.OpenTruthStore(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	input := truthstore.AppendInput{IdempotencyKey: "project-truth", Kind: truthstore.EventAssert,
		SubjectType: "character", SubjectID: "hero", Predicate: "status.alive",
		Value: json.RawMessage(`true`), ValidFromChapter: 1, KnownFromChapter: 1,
		Authority: truthstore.AuthorityHuman, Confidence: 1,
		Source: truthstore.Source{Type: "human", ID: "author"}}
	if _, err := store.Append(ctx, input); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	reopened, err := repository.OpenTruthStore(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	state, err := reopened.State(ctx, truthstore.StateQuery{Chapter: 1, SubjectID: "hero"})
	if err != nil || state.Total != 1 {
		t.Fatalf("reopened truth = %#v, %v", state, err)
	}
}
''',
    "internal/server/truth.go": r'''package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/voocel/ainovel-cli/internal/truthstore"
)

type truthAppendRequest struct {
	ProjectID string                 `json:"project_id"`
	Event     truthstore.AppendInput `json:"event"`
}

type truthRebuildRequest struct {
	ProjectID   string `json:"project_id"`
	FromChapter int    `json:"from_chapter"`
}

type truthBatchRequest struct {
	ProjectID string                  `json:"project_id"`
	Queries   []truthstore.StateQuery `json:"queries"`
}

func (s *Server) handleTruth(response http.ResponseWriter, request *http.Request) {
	path := strings.TrimPrefix(request.URL.Path, "/api/truth")
	switch path {
	case "/events":
		if request.Method == http.MethodGet {
			s.handleTruthEvents(response, request)
			return
		}
		if request.Method == http.MethodPost {
			s.handleTruthAppend(response, request)
			return
		}
	case "/state":
		if request.Method == http.MethodGet {
			s.handleTruthState(response, request)
			return
		}
	case "/state:batch":
		if request.Method == http.MethodPost {
			s.handleTruthBatch(response, request)
			return
		}
	case "/conflicts":
		if request.Method == http.MethodGet {
			s.handleTruthConflicts(response, request)
			return
		}
	case "/rebuild":
		if request.Method == http.MethodPost {
			s.handleTruthRebuild(response, request)
			return
		}
	case "/verify":
		if request.Method == http.MethodGet {
			s.handleTruthVerify(response, request)
			return
		}
	}
	truthError(response, request, http.StatusMethodNotAllowed, "TRUTH_METHOD_NOT_ALLOWED", "truth route or method is not supported", false)
}

func (s *Server) handleTruthAppend(response http.ResponseWriter, request *http.Request) {
	key := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if key == "" {
		truthError(response, request, http.StatusBadRequest, "TRUTH_IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required", false)
		return
	}
	var input truthAppendRequest
	if !decodeTruthJSON(response, request, &input) {
		return
	}
	input.Event.IdempotencyKey = key
	store, ok := s.openTruth(response, request, input.ProjectID)
	if !ok {
		return
	}
	defer store.Close()
	result, err := store.Append(request.Context(), input.Event)
	if err != nil {
		writeTruthStoreError(response, request, err)
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	truthJSON(response, status, result)
}

func (s *Server) handleTruthState(response http.ResponseWriter, request *http.Request) {
	chapter, ok := truthIntQuery(response, request, "chapter", true)
	if !ok {
		return
	}
	store, opened := s.openTruth(response, request, request.URL.Query().Get("project_id"))
	if !opened {
		return
	}
	defer store.Close()
	limit, _ := strconv.Atoi(request.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(request.URL.Query().Get("offset"))
	page, err := store.State(request.Context(), truthstore.StateQuery{Chapter: chapter,
		SubjectType: request.URL.Query().Get("subject_type"), SubjectID: request.URL.Query().Get("subject_id"),
		Predicate: request.URL.Query().Get("predicate"), Limit: limit, Offset: offset})
	if err != nil {
		writeTruthStoreError(response, request, err)
		return
	}
	truthJSON(response, http.StatusOK, page)
}

func (s *Server) handleTruthBatch(response http.ResponseWriter, request *http.Request) {
	if strings.TrimSpace(request.Header.Get("Idempotency-Key")) == "" {
		truthError(response, request, http.StatusBadRequest, "TRUTH_IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required", false)
		return
	}
	var input truthBatchRequest
	if !decodeTruthJSON(response, request, &input) {
		return
	}
	store, ok := s.openTruth(response, request, input.ProjectID)
	if !ok {
		return
	}
	defer store.Close()
	pages, err := store.StateMany(request.Context(), input.Queries)
	if err != nil {
		writeTruthStoreError(response, request, err)
		return
	}
	truthJSON(response, http.StatusOK, map[string]any{"results": pages})
}

func (s *Server) handleTruthEvents(response http.ResponseWriter, request *http.Request) {
	store, ok := s.openTruth(response, request, request.URL.Query().Get("project_id"))
	if !ok {
		return
	}
	defer store.Close()
	after, _ := strconv.ParseInt(request.URL.Query().Get("after_sequence"), 10, 64)
	limit, _ := strconv.Atoi(request.URL.Query().Get("limit"))
	page, err := store.Events(request.Context(), after, limit)
	if err != nil {
		writeTruthStoreError(response, request, err)
		return
	}
	truthJSON(response, http.StatusOK, page)
}

func (s *Server) handleTruthConflicts(response http.ResponseWriter, request *http.Request) {
	store, ok := s.openTruth(response, request, request.URL.Query().Get("project_id"))
	if !ok {
		return
	}
	defer store.Close()
	var chapter *int
	if value := request.URL.Query().Get("chapter"); value != "" {
		parsed, valid := truthIntQuery(response, request, "chapter", true)
		if !valid {
			return
		}
		chapter = &parsed
	}
	limit, _ := strconv.Atoi(request.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(request.URL.Query().Get("offset"))
	page, err := store.Conflicts(request.Context(), truthstore.ConflictQuery{Chapter: chapter,
		SubjectType: request.URL.Query().Get("subject_type"), SubjectID: request.URL.Query().Get("subject_id"),
		Predicate: request.URL.Query().Get("predicate"), Limit: limit, Offset: offset})
	if err != nil {
		writeTruthStoreError(response, request, err)
		return
	}
	truthJSON(response, http.StatusOK, page)
}

func (s *Server) handleTruthRebuild(response http.ResponseWriter, request *http.Request) {
	if strings.TrimSpace(request.Header.Get("Idempotency-Key")) == "" {
		truthError(response, request, http.StatusBadRequest, "TRUTH_IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required", false)
		return
	}
	var input truthRebuildRequest
	if !decodeTruthJSON(response, request, &input) {
		return
	}
	store, ok := s.openTruth(response, request, input.ProjectID)
	if !ok {
		return
	}
	defer store.Close()
	result, err := store.Rebuild(request.Context(), input.FromChapter)
	if err != nil {
		writeTruthStoreError(response, request, err)
		return
	}
	truthJSON(response, http.StatusOK, result)
}

func (s *Server) handleTruthVerify(response http.ResponseWriter, request *http.Request) {
	store, ok := s.openTruth(response, request, request.URL.Query().Get("project_id"))
	if !ok {
		return
	}
	defer store.Close()
	result, err := store.Verify(request.Context())
	if err != nil {
		writeTruthStoreError(response, request, err)
		return
	}
	truthJSON(response, http.StatusOK, result)
}

func (s *Server) openTruth(response http.ResponseWriter, request *http.Request, projectID string) (*truthstore.Store, bool) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		truthError(response, request, http.StatusBadRequest, "TRUTH_PROJECT_ID_REQUIRED", "project_id is required", false)
		return nil, false
	}
	store, err := s.truthProjects.OpenTruthStore(request.Context(), projectID)
	if err != nil {
		truthError(response, request, http.StatusNotFound, "TRUTH_PROJECT_NOT_FOUND", "project is unavailable", false)
		return nil, false
	}
	return store, true
}

func decodeTruthJSON(response http.ResponseWriter, request *http.Request, destination any) bool {
	request.Body = http.MaxBytesReader(response, request.Body, 128<<10)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		truthError(response, request, http.StatusBadRequest, "TRUTH_INVALID_JSON", "request body must be valid JSON", false)
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		truthError(response, request, http.StatusBadRequest, "TRUTH_INVALID_JSON", "request body must contain one JSON object", false)
		return false
	}
	return true
}

func truthIntQuery(response http.ResponseWriter, request *http.Request, name string, required bool) (int, bool) {
	value := request.URL.Query().Get(name)
	if value == "" {
		if required {
			truthError(response, request, http.StatusBadRequest, "TRUTH_QUERY_INVALID", name+" is required", false)
			return 0, false
		}
		return 0, true
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		truthError(response, request, http.StatusBadRequest, "TRUTH_QUERY_INVALID", name+" must be a non-negative integer", false)
		return 0, false
	}
	return parsed, true
}

func writeTruthStoreError(response http.ResponseWriter, request *http.Request, err error) {
	status := http.StatusInternalServerError
	code := string(truthstore.CodeStorage)
	message := "truth operation failed"
	retryable := false
	if storeErr, ok := truthstore.AsError(err); ok {
		code, message, retryable = string(storeErr.Code), storeErr.Message, storeErr.Retryable
		switch storeErr.Code {
		case truthstore.CodeValidation:
			status = http.StatusBadRequest
		case truthstore.CodeNotFound:
			status = http.StatusNotFound
		case truthstore.CodeConflict, truthstore.CodeAuthority, truthstore.CodeIdempotencyConflict:
			status = http.StatusConflict
		case truthstore.CodeBusy:
			status = http.StatusServiceUnavailable
		case truthstore.CodeCorrupt:
			status = http.StatusUnprocessableEntity
		}
	}
	truthError(response, request, status, code, message, retryable)
}

func truthError(response http.ResponseWriter, request *http.Request, status int, code, message string, retryable bool) {
	truthJSON(response, status, map[string]any{"error": map[string]any{
		"code": code, "message": message, "details": map[string]any{},
		"retryable": retryable, "trace_id": truthTraceID(request),
	}})
}

func truthTraceID(request *http.Request) string {
	if value := strings.TrimSpace(request.Header.Get("X-Trace-ID")); value != "" && len(value) <= 128 {
		return value
	}
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err == nil {
		return hex.EncodeToString(buffer)
	}
	return "truth-trace-unavailable"
}

func truthJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
''',
    "internal/server/truth_test.go": r'''package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/voocel/ainovel-cli/internal/project"
)

func TestTruthAPIAppendReplayAndChapterState(t *testing.T) {
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
	body := map[string]any{"project_id": created.ID, "event": map[string]any{
		"kind": "assert", "subject_type": "character", "subject_id": "hero",
		"predicate": "location.current", "value": "Harbor", "valid_from_chapter": 4,
		"known_from_chapter": 4, "authority": "canon", "confidence": 1,
		"source": map[string]any{"type": "chapter", "id": "chapter-4"},
	}}
	data, _ := json.Marshal(body)
	appendRequest := httptest.NewRequest(http.MethodPost, "/api/truth/events", bytes.NewReader(data))
	appendRequest.Header.Set("Idempotency-Key", "api-truth-1")
	appendResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(appendResponse, appendRequest)
	if appendResponse.Code != http.StatusCreated {
		t.Fatalf("append = %d %s", appendResponse.Code, appendResponse.Body.String())
	}
	replayRequest := httptest.NewRequest(http.MethodPost, "/api/truth/events", bytes.NewReader(data))
	replayRequest.Header.Set("Idempotency-Key", "api-truth-1")
	replayResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(replayResponse, replayRequest)
	if replayResponse.Code != http.StatusOK || !bytes.Contains(replayResponse.Body.Bytes(), []byte(`"replayed":true`)) {
		t.Fatalf("replay = %d %s", replayResponse.Code, replayResponse.Body.String())
	}
	before := httptest.NewRecorder()
	server.Handler().ServeHTTP(before, httptest.NewRequest(http.MethodGet, "/api/truth/state?project_id="+created.ID+"&chapter=3", nil))
	if before.Code != http.StatusOK || !bytes.Contains(before.Body.Bytes(), []byte(`"total":0`)) {
		t.Fatalf("before = %d %s", before.Code, before.Body.String())
	}
	after := httptest.NewRecorder()
	server.Handler().ServeHTTP(after, httptest.NewRequest(http.MethodGet, "/api/truth/state?project_id="+created.ID+"&chapter=4", nil))
	if after.Code != http.StatusOK || !bytes.Contains(after.Body.Bytes(), []byte(`"total":1`)) {
		t.Fatalf("after = %d %s", after.Code, after.Body.String())
	}
}

func TestTruthAPIRequiresIdempotencyKeyAndUsesSafeEnvelope(t *testing.T) {
	workspace := t.TempDir()
	server, err := New(Config{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/truth/events", bytes.NewBufferString(`{}`))
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", response.Code)
	}
	body := response.Body.String()
	for _, required := range []string{"TRUTH_IDEMPOTENCY_KEY_REQUIRED", "trace_id", "retryable"} {
		if !bytes.Contains([]byte(body), []byte(required)) {
			t.Fatalf("safe error missing %q: %s", required, body)
		}
	}
}
''',
    "docs/TRUTH_STORE.md": r'''# Structured Truth Store

Phase 4 introduces an append-only, per-project SQLite authority layer for narrative facts.
It is deliberately separate from model memory and browser state.

## Temporal model

Every event has two chapter intervals:

- **valid time**: when the statement is true in the story;
- **knowledge time**: when the statement is available to the authoring system.

The effective query interval is their intersection. A Chapter-N query therefore cannot
see an event whose story validity or knowledge begins after Chapter N. This applies to
character state, locations, inventory, relationships and arbitrary typed predicates.

## Authority and conflicts

Authority order is `inferred < agent < imported < canon < human`. A conflicting event
does not silently overwrite another value. Overlapping distinct values are projected
into `truth_conflicts`. Replacement requires an explicit `supersedes_event_id`, and a
lower-authority event cannot supersede a higher-authority event.

## Provenance and idempotency

Every event records source type, source identifier, optional revision and bounded excerpt.
Writes require `Idempotency-Key`. Reusing a key with identical canonical content replays
the original event; reusing it with different content is rejected.

## Projection and rebuild

`truth_events` is protected by SQLite triggers against update and delete. Query tables are
derived projections. Rebuild verifies every event checksum and can replace only projection
rows intersecting a requested chapter boundary, while preserving earlier unaffected rows.
`GET /api/truth/verify` compares the event-derived projection digest with stored rows.

## API

```text
GET  /api/truth/events
POST /api/truth/events
GET  /api/truth/state
POST /api/truth/state:batch
GET  /api/truth/conflicts
POST /api/truth/rebuild
GET  /api/truth/verify
```

All operations require `project_id`. Write routes require `Idempotency-Key`. Collection
limits are bounded, JSON fact values are capped at 64 KiB, and transport errors use the
common secret-free envelope with a trace identifier.
''',
}


def write(path: str, content: str) -> None:
    destination = ROOT / path
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_text(content.rstrip() + "\n", encoding="utf-8")


def discover_entry_root() -> str:
    source = "\n".join(path.read_text(encoding="utf-8") for path in (ROOT / "internal/project").glob("*.go"))
    signature = re.search(r"func\s+\(r\s+\*Repository\)\s+find\s*\([^)]*\)\s*\(\s*([A-Za-z_]\w*)\s*,\s*error\s*\)", source, re.S)
    if not signature:
        raise RuntimeError("cannot discover project find return type")
    type_name = signature.group(1)
    structure = re.search(r"type\s+" + re.escape(type_name) + r"\s+struct\s*\{(.*?)\n\}", source, re.S)
    if not structure:
        raise RuntimeError(f"cannot discover {type_name} structure")
    candidates = []
    for name, type_expr in re.findall(r"^\s*([A-Za-z_]\w*)\s+([^\n`]+)", structure.group(1), re.M):
        if "string" in type_expr and any(token in name.lower() for token in ("root", "path", "dir")):
            candidates.append(name)
    if not candidates:
        raise RuntimeError(f"cannot discover root field in {type_name}")
    return "entry." + candidates[0]


def patch_server() -> None:
    path = ROOT / "internal/server/server.go"
    text = path.read_text(encoding="utf-8")
    if '"github.com/voocel/ainovel-cli/internal/project"' not in text:
        text = text.replace("import (", 'import (\n\t"github.com/voocel/ainovel-cli/internal/project"', 1)
    if "truthProjects *project.Repository" not in text:
        match = re.search(r"type\s+Server\s+struct\s*\{", text)
        if not match:
            raise RuntimeError("cannot find Server struct")
        text = text[:match.end()] + "\n\ttruthProjects *project.Repository" + text[match.end():]
    if "phase4TruthProjects" not in text:
        match = re.search(r"func\s+New\s*\(\s*config\s+Config\s*\)\s*\(\s*\*Server\s*,\s*error\s*\)\s*\{", text)
        if not match:
            raise RuntimeError("cannot find server New")
        setup = "\n\tphase4TruthProjects, truthProjectErr := project.NewRepository(config.Workspace)\n\tif truthProjectErr != nil {\n\t\treturn nil, fmt.Errorf(\"prepare truth project repository: %w\", truthProjectErr)\n\t}\n"
        text = text[:match.end()] + setup + text[match.end():]
        literal = text.find("&Server{", match.end())
        if literal < 0:
            raise RuntimeError("cannot find Server literal")
        text = text[:literal + len("&Server{")] + "\n\t\ttruthProjects: phase4TruthProjects," + text[literal + len("&Server{"):]
    if '"/api/truth/"' not in text:
        lines = text.splitlines()
        inserted = False
        for index, line in enumerate(lines):
            match = re.match(r"^(\s*)([A-Za-z_]\w*)\.HandleFunc\(\"/api/openapi\.json\"\s*,\s*([A-Za-z_]\w*)\.([A-Za-z_]\w*)\)", line)
            if match:
                indent, mux, receiver, _ = match.groups()
                lines[index + 1:index + 1] = [
                    f'{indent}{mux}.HandleFunc("/api/truth", {receiver}.handleTruth)',
                    f'{indent}{mux}.HandleFunc("/api/truth/", {receiver}.handleTruth)',
                ]
                inserted = True
                break
        if not inserted:
            raise RuntimeError("cannot discover ServeMux registration point")
        text = "\n".join(lines) + "\n"
    path.write_text(text, encoding="utf-8")


def patch_openapi() -> None:
    path = ROOT / "internal/server/openapi.json"
    document = json.loads(path.read_text(encoding="utf-8"))
    schemas = document.setdefault("components", {}).setdefault("schemas", {})
    schemas.update({
        "TruthSource": {"type": "object", "required": ["type", "id"], "properties": {
            "type": {"type": "string", "maxLength": 200}, "id": {"type": "string", "maxLength": 200},
            "revision": {"type": "string"}, "excerpt": {"type": "string", "maxLength": 2000}}},
        "TruthEventInput": {"type": "object", "required": ["kind", "subject_type", "subject_id", "predicate", "value", "valid_from_chapter", "known_from_chapter", "authority", "confidence", "source"], "properties": {
            "kind": {"type": "string", "enum": ["assert", "supersede", "retract"]},
            "subject_type": {"type": "string"}, "subject_id": {"type": "string"}, "predicate": {"type": "string"},
            "value": {}, "valid_from_chapter": {"type": "integer", "minimum": 0},
            "valid_to_chapter": {"type": ["integer", "null"], "minimum": 0},
            "known_from_chapter": {"type": "integer", "minimum": 0},
            "known_to_chapter": {"type": ["integer", "null"], "minimum": 0},
            "authority": {"type": "string", "enum": ["inferred", "agent", "imported", "canon", "human"]},
            "confidence": {"type": "number", "minimum": 0, "maximum": 1},
            "source": {"$ref": "#/components/schemas/TruthSource"},
            "supersedes_event_id": {"type": "string"}}},
        "TruthAppendRequest": {"type": "object", "required": ["project_id", "event"], "properties": {
            "project_id": {"type": "string"}, "event": {"$ref": "#/components/schemas/TruthEventInput"}}},
        "TruthEvent": {"allOf": [{"$ref": "#/components/schemas/TruthEventInput"}, {"type": "object", "required": ["sequence", "id", "created_at", "checksum"], "properties": {
            "sequence": {"type": "integer"}, "id": {"type": "string"}, "created_at": {"type": "string", "format": "date-time"}, "checksum": {"type": "string"}}}]},
        "TruthFact": {"allOf": [{"$ref": "#/components/schemas/TruthEvent"}, {"type": "object", "required": ["effective_from_chapter", "conflicted"], "properties": {
            "effective_from_chapter": {"type": "integer"}, "effective_to_chapter": {"type": ["integer", "null"]},
            "superseded_by_event_id": {"type": "string"}, "conflicted": {"type": "boolean"}}}]},
        "TruthConflict": {"type": "object", "required": ["id", "subject_type", "subject_id", "predicate", "left_event_id", "right_event_id", "from_chapter", "reason"], "properties": {
            "id": {"type": "string"}, "subject_type": {"type": "string"}, "subject_id": {"type": "string"},
            "predicate": {"type": "string"}, "left_event_id": {"type": "string"}, "right_event_id": {"type": "string"},
            "from_chapter": {"type": "integer"}, "to_chapter": {"type": ["integer", "null"]}, "reason": {"type": "string"}}},
        "TruthRebuildRequest": {"type": "object", "required": ["project_id", "from_chapter"], "properties": {
            "project_id": {"type": "string"}, "from_chapter": {"type": "integer", "minimum": 0}}},
    })
    error_response = {"description": "Structured error", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/ErrorEnvelope"}}}}
    idem = {"name": "Idempotency-Key", "in": "header", "required": True, "schema": {"type": "string", "maxLength": 200}}
    project = {"name": "project_id", "in": "query", "required": True, "schema": {"type": "string"}}
    chapter = {"name": "chapter", "in": "query", "required": True, "schema": {"type": "integer", "minimum": 0}}
    paths = document.setdefault("paths", {})
    paths["/api/truth/events"] = {
        "get": {"operationId": "listTruthEvents", "parameters": [project, {"name": "after_sequence", "in": "query", "schema": {"type": "integer", "minimum": 0}}, {"name": "limit", "in": "query", "schema": {"type": "integer", "maximum": 500}}], "responses": {"200": {"description": "Truth event page"}, "default": error_response}},
        "post": {"operationId": "appendTruthEvent", "parameters": [idem], "requestBody": {"required": True, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/TruthAppendRequest"}}}}, "responses": {"201": {"description": "Truth event appended"}, "200": {"description": "Idempotent replay"}, "default": error_response}},
    }
    paths["/api/truth/state"] = {"get": {"operationId": "queryTruthState", "parameters": [project, chapter,
        {"name": "subject_type", "in": "query", "schema": {"type": "string"}}, {"name": "subject_id", "in": "query", "schema": {"type": "string"}},
        {"name": "predicate", "in": "query", "schema": {"type": "string"}}, {"name": "limit", "in": "query", "schema": {"type": "integer", "maximum": 500}},
        {"name": "offset", "in": "query", "schema": {"type": "integer", "minimum": 0}}], "responses": {"200": {"description": "Chapter-bounded truth state"}, "default": error_response}}}
    paths["/api/truth/state:batch"] = {"post": {"operationId": "batchQueryTruthState", "parameters": [idem], "requestBody": {"required": True, "content": {"application/json": {"schema": {"type": "object"}}}}, "responses": {"200": {"description": "Batch truth states"}, "default": error_response}}}
    paths["/api/truth/conflicts"] = {"get": {"operationId": "listTruthConflicts", "parameters": [project, {"name": "chapter", "in": "query", "schema": {"type": "integer", "minimum": 0}}], "responses": {"200": {"description": "Truth conflicts"}, "default": error_response}}}
    paths["/api/truth/rebuild"] = {"post": {"operationId": "rebuildTruthProjection", "parameters": [idem], "requestBody": {"required": True, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/TruthRebuildRequest"}}}}, "responses": {"200": {"description": "Truth projection rebuilt"}, "default": error_response}}}
    paths["/api/truth/verify"] = {"get": {"operationId": "verifyTruthProjection", "parameters": [project], "responses": {"200": {"description": "Truth projection verification"}, "default": error_response}}}
    path.write_text(json.dumps(document, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def patch_openapi_test() -> None:
    path = ROOT / "internal/server/openapi_test.go"
    text = path.read_text(encoding="utf-8")
    if '"/api/truth/events"' in text:
        return
    start = text.find("expected := map[string][]string{")
    if start < 0:
        raise RuntimeError("cannot find OpenAPI expected map")
    closing = text.find("\n\t}", start)
    if closing < 0:
        raise RuntimeError("cannot find OpenAPI expected map end")
    entries = '''
		"/api/truth/events":      {"get", "post"},
		"/api/truth/state":       {"get"},
		"/api/truth/state:batch": {"post"},
		"/api/truth/conflicts":   {"get"},
		"/api/truth/rebuild":     {"post"},
		"/api/truth/verify":      {"get"},'''
    text = text[:closing] + entries + text[closing:]
    path.write_text(text, encoding="utf-8")


def patch_ci() -> None:
    path = ROOT / ".github/workflows/ci.yml"
    text = path.read_text(encoding="utf-8")
    text = text.replace("./internal/db/migrate ./internal/project ./internal/server/...", "./internal/db/migrate ./internal/project ./internal/truthstore ./internal/server/...")
    if "Truth Store 100k temporal index gate" not in text:
        needle = "      - name: Validate OpenAPI routes and schemas\n"
        step = "      - name: Truth Store 100k temporal index gate\n        env:\n          GOWORK: \"off\"\n          NOVELFORGE_SCALE_TEST: \"1\"\n        run: go test -buildvcs=false -count=1 ./internal/truthstore -run '^TestHundredThousandFactTemporalQueryUsesIndex$'\n"
        if needle not in text:
            raise RuntimeError("cannot find CI OpenAPI step")
        text = text.replace(needle, step + needle, 1)
    path.write_text(text, encoding="utf-8")


def patch_docs() -> None:
    status_path = ROOT / "docs/IMPLEMENTATION_STATUS.md"
    status = status_path.read_text(encoding="utf-8")
    status = status.replace("| 4–13 | not started | Start Phase 4 from the actual remote main after running the required baseline. |", "| 4 — structured Truth Store | implementation in PR #10 | Temporal event log, projection, authority, conflicts, Chapter-N queries, rebuild and APIs implemented; CI and merged-main acceptance remain. |\n| 5–13 | not started | Begin only after Phase 4 merged-main acceptance. |")
    if "## Phase 4 implementation" not in status:
        status += """

## Phase 4 implementation

- Active branch: `feature/phase-04-truth-store`.
- Active pull request: [#10 — feat: add structured truth store](https://github.com/feng123-new/NovelForge/pull/10).
- Adds project migration 2 with append-only temporal events, deterministic projections, explicit conflicts and indexes.
- Adds bitemporal Chapter-N and batch queries that require both story validity and knowledge availability.
- Adds provenance, ranked authority, idempotent replay, explicit supersede/retract and checksum verification.
- Adds bounded projection rebuild and verification digest endpoints.
- Adds Truth REST/OpenAPI operations, integration tests, race coverage, busy-lock behavior and a 100,000-row index gate.
- Phase 4 remains formally incomplete until PR CI, squash merge, and merge-triggered `main` CI all pass.

## Phase 4 exact resume point

Inspect PR #10 Go, Frontend, Windows and Docker jobs. Fix only evidence-based failures, then squash merge and require the merge-triggered `main` workflow to pass before marking Phase 4 complete or beginning Phase 5.
"""
    status_path.write_text(status, encoding="utf-8")
    architecture = ROOT / "docs/ARCHITECTURE.md"
    architecture_text = architecture.read_text(encoding="utf-8")
    if "## Temporal Truth Store" not in architecture_text:
        architecture_text += """

## Temporal Truth Store

The project database owns an append-only `truth_events` log and rebuildable `truth_facts` /
`truth_conflicts` projections. Every fact carries valid-time and knowledge-time chapter
ranges; Chapter-N queries use their intersection so later discoveries cannot leak into
earlier context. Conflicting values coexist until an authorized event explicitly
supersedes or retracts a predecessor. Human and canonical assertions outrank generated
or inferred assertions, but authority never causes a silent overwrite.
"""
    architecture.write_text(architecture_text, encoding="utf-8")


for relative, content in FILES.items():
    if relative == "internal/project/truth.go":
        content = content.replace("ENTRY_ROOT", discover_entry_root())
    write(relative, content)

patch_server()
patch_openapi()
patch_openapi_test()
patch_ci()
patch_docs()
