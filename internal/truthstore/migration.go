package truthstore

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
			authority TEXT NOT NULL CHECK (authority IN ('llm_suggestion', 'story_compass', 'volume_plan', 'arc_plan', 'chapter_plan', 'generated_final', 'human_final')),
			confidence REAL NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
			source_type TEXT NOT NULL,
			source_id TEXT NOT NULL,
			source_chapter INTEGER NOT NULL CHECK (source_chapter >= 0),
			source_version TEXT NOT NULL,
			source_extractor TEXT NOT NULL DEFAULT '',
			source_confirmed_by TEXT NOT NULL DEFAULT '',
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
			status TEXT NOT NULL CHECK (status IN ('unresolved', 'resolved')),
			reason TEXT NOT NULL,
			UNIQUE(left_event_id, right_event_id, from_chapter)
		);
		CREATE INDEX idx_truth_conflicts_asof
			ON truth_conflicts(subject_type, subject_id, predicate, status, from_chapter, to_chapter);
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
