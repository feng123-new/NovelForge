package narrativeledger

import "github.com/voocel/ainovel-cli/internal/db/migrate"

// Migration installs the Phase 6 Narrative Ledger after the Phase 5 quality gate.
func Migration() migrate.Migration {
	return migrate.Migration{
		Version: 4,
		Name:    "narrative_ledger",
		SQL: `CREATE TABLE narrative_ledger_commits (
			source_transaction_id TEXT PRIMARY KEY,
			source_candidate_id TEXT NOT NULL DEFAULT '',
			chapter INTEGER NOT NULL CHECK (chapter >= 0),
			authority TEXT NOT NULL CHECK (authority IN ('accepted_final', 'human')),
			content_hash TEXT NOT NULL,
			provenance_json TEXT NOT NULL DEFAULT '{}',
			committed_at TEXT NOT NULL
		);

		CREATE TABLE foreshadows (
			id TEXT PRIMARY KEY,
			key TEXT NOT NULL UNIQUE,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			priority TEXT NOT NULL CHECK (priority IN ('critical', 'high', 'normal', 'low')),
			status TEXT NOT NULL CHECK (status IN ('planned', 'planted', 'reinforced', 'revealed', 'abandoned')),
			planted_chapter INTEGER CHECK (planted_chapter IS NULL OR planted_chapter >= 0),
			due_chapter INTEGER CHECK (due_chapter IS NULL OR due_chapter >= 0),
			reveal_chapter INTEGER CHECK (reveal_chapter IS NULL OR reveal_chapter >= 0),
			source_transaction_id TEXT NOT NULL REFERENCES narrative_ledger_commits(source_transaction_id),
			updated_chapter INTEGER NOT NULL CHECK (updated_chapter >= 0),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE INDEX idx_foreshadows_status_due_priority
			ON foreshadows(status, due_chapter, priority, key);
		CREATE INDEX idx_foreshadows_priority_status
			ON foreshadows(priority, status, due_chapter, key);

		CREATE TABLE foreshadow_events (
			event_id TEXT PRIMARY KEY,
			foreshadow_id TEXT NOT NULL REFERENCES foreshadows(id) ON DELETE CASCADE,
			source_transaction_id TEXT NOT NULL REFERENCES narrative_ledger_commits(source_transaction_id),
			chapter INTEGER NOT NULL CHECK (chapter >= 0),
			action TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			provenance_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			UNIQUE(source_transaction_id, foreshadow_id, action)
		);
		CREATE INDEX idx_foreshadow_events_source
			ON foreshadow_events(source_transaction_id, foreshadow_id);

		CREATE TABLE secrets (
			id TEXT PRIMARY KEY,
			key TEXT NOT NULL UNIQUE,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL CHECK (status IN ('hidden', 'hinted', 'revealed', 'retired')),
			public_from_chapter INTEGER CHECK (public_from_chapter IS NULL OR public_from_chapter >= 0),
			source_transaction_id TEXT NOT NULL REFERENCES narrative_ledger_commits(source_transaction_id),
			updated_chapter INTEGER NOT NULL CHECK (updated_chapter >= 0),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE INDEX idx_secrets_status_public
			ON secrets(status, public_from_chapter, key);

		CREATE TABLE secret_knowledge (
			knowledge_id TEXT PRIMARY KEY,
			secret_id TEXT NOT NULL REFERENCES secrets(id) ON DELETE CASCADE,
			holder TEXT NOT NULL,
			known_from_chapter INTEGER NOT NULL CHECK (known_from_chapter >= 0),
			known_until_chapter INTEGER,
			source_transaction_id TEXT NOT NULL REFERENCES narrative_ledger_commits(source_transaction_id),
			created_at TEXT NOT NULL,
			CHECK (known_until_chapter IS NULL OR known_until_chapter >= known_from_chapter),
			UNIQUE(secret_id, holder, known_from_chapter)
		);
		CREATE INDEX idx_secret_knowledge_temporal
			ON secret_knowledge(secret_id, known_from_chapter, known_until_chapter, holder);
		CREATE INDEX idx_secret_knowledge_holder
			ON secret_knowledge(holder, known_from_chapter, known_until_chapter, secret_id);

		CREATE TABLE secret_events (
			event_id TEXT PRIMARY KEY,
			secret_id TEXT NOT NULL REFERENCES secrets(id) ON DELETE CASCADE,
			source_transaction_id TEXT NOT NULL REFERENCES narrative_ledger_commits(source_transaction_id),
			chapter INTEGER NOT NULL CHECK (chapter >= 0),
			action TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			provenance_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			UNIQUE(source_transaction_id, secret_id, action)
		);
		CREATE INDEX idx_secret_events_source
			ON secret_events(source_transaction_id, secret_id);

		CREATE VIEW narrative_ledger_current_chapter AS
			SELECT COALESCE(MAX(chapter), 0) AS current_chapter
			FROM narrative_ledger_commits;

		CREATE VIEW foreshadow_status_view AS
			SELECT f.*,
				CASE
					WHEN f.status IN ('planned', 'planted', 'reinforced')
						AND f.due_chapter IS NOT NULL
						AND f.due_chapter < c.current_chapter
					THEN 'overdue'
					ELSE f.status
				END AS effective_status
			FROM foreshadows AS f
			CROSS JOIN narrative_ledger_current_chapter AS c;

		CREATE VIEW secret_status_view AS
			SELECT s.*,
				CASE
					WHEN s.public_from_chapter IS NOT NULL
						AND s.public_from_chapter <= c.current_chapter
					THEN 1
					ELSE 0
				END AS is_public
			FROM secrets AS s
			CROSS JOIN narrative_ledger_current_chapter AS c;`,
	}
}
