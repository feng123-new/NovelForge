package narrativeledger

import "github.com/voocel/ainovel-cli/internal/db/migrate"

// Migration adds the Phase 6 Narrative Ledger to the existing per-project
// SQLite database. It is intentionally independent from Truth projection tables
// while retaining the same authority and provenance vocabulary.
func Migration() migrate.Migration {
	return migrate.Migration{
		Version: 4,
		Name:    "narrative_ledger",
		SQL: `CREATE TABLE narrative_ledger_operations (
			idempotency_key TEXT PRIMARY KEY,
			request_hash TEXT NOT NULL,
			operation TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE TABLE narrative_ledger_commits (
			transaction_id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			chapter INTEGER NOT NULL CHECK (chapter >= 0),
			proposal_id TEXT NOT NULL,
			candidate_id TEXT NOT NULL,
			request_hash TEXT NOT NULL,
			commit_id TEXT NOT NULL UNIQUE,
			foreshadow_count INTEGER NOT NULL DEFAULT 0,
			secret_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		);
		CREATE INDEX idx_narrative_ledger_commits_project_chapter
			ON narrative_ledger_commits(project_id, chapter, transaction_id);
		CREATE TABLE foreshadows (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			importance TEXT NOT NULL CHECK (importance IN ('low','medium','high','critical')),
			planted_chapter INTEGER NOT NULL CHECK (planted_chapter >= 0),
			expected_payoff_min INTEGER NOT NULL CHECK (expected_payoff_min >= planted_chapter),
			expected_payoff_max INTEGER NOT NULL CHECK (expected_payoff_max >= expected_payoff_min),
			actual_payoff INTEGER,
			status TEXT NOT NULL CHECK (status IN ('planned','planted','progressing','resolved','abandoned','contradicted')),
			related_entities_json TEXT NOT NULL CHECK (json_valid(related_entities_json)),
			related_arcs_json TEXT NOT NULL CHECK (json_valid(related_arcs_json)),
			last_progress_chapter INTEGER NOT NULL CHECK (last_progress_chapter >= planted_chapter),
			urgency TEXT NOT NULL CHECK (urgency IN ('low','normal','high','critical')),
			source_version TEXT NOT NULL,
			authority TEXT NOT NULL CHECK (authority IN ('llm_suggestion','story_compass','volume_plan','arc_plan','chapter_plan','generated_final','human_final')),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE INDEX idx_foreshadows_project_status_payoff
			ON foreshadows(project_id, status, expected_payoff_max, importance, urgency, id);
		CREATE INDEX idx_foreshadows_project_progress
			ON foreshadows(project_id, last_progress_chapter, id);
		CREATE INDEX idx_foreshadows_project_importance
			ON foreshadows(project_id, importance, status, expected_payoff_max, id);
		CREATE TABLE foreshadow_entities (
			foreshadow_id TEXT NOT NULL REFERENCES foreshadows(id) ON DELETE CASCADE,
			entity_id TEXT NOT NULL,
			PRIMARY KEY(foreshadow_id, entity_id)
		);
		CREATE INDEX idx_foreshadow_entities_entity ON foreshadow_entities(entity_id, foreshadow_id);
		CREATE TABLE foreshadow_arcs (
			foreshadow_id TEXT NOT NULL REFERENCES foreshadows(id) ON DELETE CASCADE,
			arc_id TEXT NOT NULL,
			PRIMARY KEY(foreshadow_id, arc_id)
		);
		CREATE INDEX idx_foreshadow_arcs_arc ON foreshadow_arcs(arc_id, foreshadow_id);
		CREATE TABLE foreshadow_events (
			sequence INTEGER PRIMARY KEY AUTOINCREMENT,
			id TEXT NOT NULL UNIQUE,
			foreshadow_id TEXT NOT NULL REFERENCES foreshadows(id) ON DELETE RESTRICT,
			project_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			chapter INTEGER NOT NULL CHECK (chapter >= 0),
			payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
			source_version TEXT NOT NULL,
			authority TEXT NOT NULL,
			source_type TEXT NOT NULL,
			source_id TEXT NOT NULL,
			source_chapter INTEGER NOT NULL,
			source_extractor TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		);
		CREATE INDEX idx_foreshadow_events_resource ON foreshadow_events(foreshadow_id, sequence);
		CREATE TABLE secrets (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			description TEXT NOT NULL,
			truth TEXT NOT NULL,
			created_chapter INTEGER NOT NULL CHECK (created_chapter >= 0),
			revealed_chapter INTEGER,
			public_status TEXT NOT NULL CHECK (public_status IN ('private','public')),
			related_foreshadow TEXT NOT NULL DEFAULT '',
			source_version TEXT NOT NULL,
			authority TEXT NOT NULL CHECK (authority IN ('llm_suggestion','story_compass','volume_plan','arc_plan','chapter_plan','generated_final','human_final')),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			CHECK (revealed_chapter IS NULL OR revealed_chapter >= created_chapter)
		);
		CREATE INDEX idx_secrets_project_public ON secrets(project_id, public_status, revealed_chapter, created_chapter, id);
		CREATE TABLE secret_holders (
			secret_id TEXT NOT NULL REFERENCES secrets(id) ON DELETE CASCADE,
			entity_id TEXT NOT NULL,
			valid_from_chapter INTEGER NOT NULL CHECK (valid_from_chapter >= 0),
			valid_to_chapter INTEGER,
			source_version TEXT NOT NULL,
			authority TEXT NOT NULL,
			provenance_json TEXT NOT NULL CHECK (json_valid(provenance_json)),
			PRIMARY KEY(secret_id, entity_id, valid_from_chapter),
			CHECK (valid_to_chapter IS NULL OR valid_to_chapter >= valid_from_chapter)
		);
		CREATE INDEX idx_secret_holders_temporal ON secret_holders(secret_id, valid_from_chapter, valid_to_chapter, entity_id);
		CREATE INDEX idx_secret_holders_entity ON secret_holders(entity_id, valid_from_chapter, valid_to_chapter, secret_id);
		CREATE TABLE secret_events (
			sequence INTEGER PRIMARY KEY AUTOINCREMENT,
			id TEXT NOT NULL UNIQUE,
			secret_id TEXT NOT NULL REFERENCES secrets(id) ON DELETE RESTRICT,
			project_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			chapter INTEGER NOT NULL CHECK (chapter >= 0),
			payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
			source_version TEXT NOT NULL,
			authority TEXT NOT NULL,
			source_type TEXT NOT NULL,
			source_id TEXT NOT NULL,
			source_chapter INTEGER NOT NULL,
			source_extractor TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		);
		CREATE INDEX idx_secret_events_resource ON secret_events(secret_id, sequence);
		CREATE TABLE narrative_ledger_meta (
			id INTEGER PRIMARY KEY CHECK (id=1),
			last_commit_chapter INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL
		);
		INSERT INTO narrative_ledger_meta(id,last_commit_chapter,updated_at) VALUES(1,0,'1970-01-01T00:00:00Z');
		CREATE TRIGGER foreshadow_events_append_only_update BEFORE UPDATE ON foreshadow_events BEGIN SELECT RAISE(ABORT, 'foreshadow_events is append-only'); END;
		CREATE TRIGGER foreshadow_events_append_only_delete BEFORE DELETE ON foreshadow_events BEGIN SELECT RAISE(ABORT, 'foreshadow_events is append-only'); END;
		CREATE TRIGGER secret_events_append_only_update BEFORE UPDATE ON secret_events BEGIN SELECT RAISE(ABORT, 'secret_events is append-only'); END;
		CREATE TRIGGER secret_events_append_only_delete BEFORE DELETE ON secret_events BEGIN SELECT RAISE(ABORT, 'secret_events is append-only'); END;`,
	}
}
