package qualitygate

import "github.com/voocel/ainovel-cli/internal/db/migrate"

func Migration() migrate.Migration {
	return migrate.Migration{
		Version: 3,
		Name:    "chapter_quality_gate",
		SQL: `CREATE TABLE chapter_transactions (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			chapter INTEGER NOT NULL CHECK (chapter > 0),
			state TEXT NOT NULL,
			attempt INTEGER NOT NULL DEFAULT 0 CHECK (attempt >= 0),
			max_rewrites INTEGER NOT NULL DEFAULT 2 CHECK (max_rewrites >= 0 AND max_rewrites <= 10),
			quality_threshold REAL NOT NULL DEFAULT 7.0 CHECK (quality_threshold >= 0 AND quality_threshold <= 10),
			final_candidate_id TEXT NOT NULL DEFAULT '',
			hold_reason TEXT NOT NULL DEFAULT '',
			last_reason TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(project_id, chapter)
		);
		CREATE INDEX idx_chapter_transactions_project_state ON chapter_transactions(project_id, state, chapter);
		CREATE TABLE chapter_state_changes (
			sequence INTEGER PRIMARY KEY AUTOINCREMENT,
			transaction_id TEXT NOT NULL REFERENCES chapter_transactions(id) ON DELETE CASCADE,
			chapter INTEGER NOT NULL,
			from_state TEXT NOT NULL,
			to_state TEXT NOT NULL,
			reason TEXT NOT NULL,
			actor TEXT NOT NULL,
			attempt INTEGER NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE INDEX idx_chapter_state_changes_tx ON chapter_state_changes(transaction_id, sequence);
		CREATE TABLE chapter_candidates (
			id TEXT PRIMARY KEY,
			transaction_id TEXT NOT NULL REFERENCES chapter_transactions(id) ON DELETE CASCADE,
			chapter INTEGER NOT NULL,
			attempt INTEGER NOT NULL,
			text_content TEXT NOT NULL,
			text_sha TEXT NOT NULL,
			source_version TEXT NOT NULL,
			continuity_status TEXT NOT NULL DEFAULT '',
			editor_score REAL,
			selected INTEGER NOT NULL DEFAULT 0,
			selection_reason TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			UNIQUE(transaction_id, attempt)
		);
		CREATE INDEX idx_chapter_candidates_tx_score ON chapter_candidates(transaction_id, continuity_status, editor_score DESC, attempt ASC);
		CREATE TABLE fact_proposals (
			proposal_id TEXT PRIMARY KEY,
			transaction_id TEXT NOT NULL REFERENCES chapter_transactions(id) ON DELETE CASCADE,
			candidate_id TEXT NOT NULL REFERENCES chapter_candidates(id) ON DELETE CASCADE,
			payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
			created_at TEXT NOT NULL,
			UNIQUE(transaction_id, candidate_id)
		);
		CREATE TABLE continuity_results (
			transaction_id TEXT NOT NULL REFERENCES chapter_transactions(id) ON DELETE CASCADE,
			candidate_id TEXT NOT NULL REFERENCES chapter_candidates(id) ON DELETE CASCADE,
			status TEXT NOT NULL CHECK (status IN ('PASS', 'WARN', 'FAIL')),
			blocking INTEGER NOT NULL,
			payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
			created_at TEXT NOT NULL,
			PRIMARY KEY(transaction_id, candidate_id)
		);
		CREATE TABLE editor_reviews (
			transaction_id TEXT NOT NULL REFERENCES chapter_transactions(id) ON DELETE CASCADE,
			candidate_id TEXT NOT NULL REFERENCES chapter_candidates(id) ON DELETE CASCADE,
			score REAL NOT NULL CHECK (score >= 0 AND score <= 10),
			payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
			created_at TEXT NOT NULL,
			PRIMARY KEY(transaction_id, candidate_id)
		);
		CREATE TABLE model_calls (
			id TEXT PRIMARY KEY,
			idempotency_key TEXT NOT NULL UNIQUE,
			project_id TEXT NOT NULL,
			chapter INTEGER NOT NULL,
			transaction_id TEXT NOT NULL,
			agent TEXT NOT NULL,
			operation TEXT NOT NULL,
			provider TEXT NOT NULL,
			model TEXT NOT NULL,
			request_hash TEXT NOT NULL,
			response_hash TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			attempt INTEGER NOT NULL,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			started_at TEXT NOT NULL,
			ended_at TEXT NOT NULL,
			error_code TEXT NOT NULL DEFAULT '',
			response_json TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX idx_model_calls_tx_agent ON model_calls(transaction_id, agent, operation, attempt);
		CREATE TABLE chapter_truth_commits (
			transaction_id TEXT NOT NULL REFERENCES chapter_transactions(id) ON DELETE CASCADE,
			proposal_id TEXT NOT NULL,
			change_index INTEGER NOT NULL,
			truth_event_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY(transaction_id, proposal_id, change_index)
		);
		CREATE TABLE chapter_checkpoints (
			transaction_id TEXT PRIMARY KEY REFERENCES chapter_transactions(id) ON DELETE CASCADE,
			candidate_id TEXT NOT NULL,
			final_sha TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
	}
}
