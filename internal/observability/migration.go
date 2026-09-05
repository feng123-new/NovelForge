package observability

import "github.com/voocel/ainovel-cli/internal/db/migrate"

func Migration() migrate.Migration {
	return migrate.Migration{Version: 11, Name: "observation_attempts_and_policy", SQL: `
CREATE TABLE observation_policy(id INTEGER PRIMARY KEY CHECK(id=1),revision INTEGER NOT NULL,payload_json TEXT NOT NULL CHECK(json_valid(payload_json)));
INSERT INTO observation_policy VALUES(1,1,'{}');
CREATE TABLE observation_attempts(seq INTEGER PRIMARY KEY AUTOINCREMENT,id TEXT NOT NULL UNIQUE,project_id TEXT NOT NULL,task_id TEXT NOT NULL,logical_id TEXT NOT NULL,chapter INTEGER NOT NULL,agent TEXT NOT NULL,operation TEXT NOT NULL,provider TEXT NOT NULL,model TEXT NOT NULL,state TEXT NOT NULL,started_at TEXT NOT NULL,ended_at TEXT,cost_micros INTEGER,reserved_micros INTEGER NOT NULL DEFAULT 0,error_code TEXT NOT NULL DEFAULT '',payload_json TEXT NOT NULL CHECK(json_valid(payload_json)));
CREATE INDEX idx_observation_task ON observation_attempts(project_id,task_id,seq);
CREATE INDEX idx_observation_chapter ON observation_attempts(project_id,chapter,seq);
CREATE INDEX idx_observation_provider ON observation_attempts(project_id,provider,seq);
CREATE TABLE observation_links(call_key TEXT PRIMARY KEY,logical_id TEXT NOT NULL);
CREATE TABLE observation_replays(logical_id TEXT PRIMARY KEY,count INTEGER NOT NULL);
CREATE TABLE observation_changes(idempotency_key TEXT PRIMARY KEY,request_hash TEXT NOT NULL,result_json TEXT NOT NULL CHECK(json_valid(result_json)),created_at TEXT NOT NULL);
CREATE TRIGGER observation_changes_no_update BEFORE UPDATE ON observation_changes BEGIN SELECT RAISE(ABORT,'immutable observation change'); END;
CREATE TRIGGER observation_changes_no_delete BEFORE DELETE ON observation_changes BEGIN SELECT RAISE(ABORT,'immutable observation change'); END;
`}
}
