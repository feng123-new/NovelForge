package authoring

import "github.com/voocel/ainovel-cli/internal/db/migrate"

// Append-only migration definition. Do not put evolving embedded skill text here.
func Migration() migrate.Migration {
	return migrate.Migration{Version: 9, Name: "authoring_skills_style_references", SQL: `
CREATE TABLE authoring_state (id INTEGER PRIMARY KEY CHECK(id=1),revision INTEGER NOT NULL,rules_json TEXT NOT NULL CHECK(json_valid(rules_json)));
INSERT INTO authoring_state VALUES(1,1,'{"enabled":true,"phrases":["不由得","嘴角勾起一抹","命运的齿轮"],"max_phrase_occurrences":1,"max_sentence_repeats":1,"min_sentence_runes":12,"previous_chapters":3}');
CREATE TABLE authoring_entries (id TEXT PRIMARY KEY,kind TEXT NOT NULL CHECK(kind IN ('skill','style','knowledge')),role TEXT NOT NULL,enabled INTEGER NOT NULL CHECK(enabled IN (0,1)),pinned INTEGER NOT NULL CHECK(pinned IN (0,1)),priority INTEGER NOT NULL,from_chapter INTEGER NOT NULL,pov TEXT NOT NULL,payload_json TEXT NOT NULL CHECK(json_valid(payload_json)));
CREATE INDEX authoring_selection ON authoring_entries(kind,enabled,role,from_chapter,priority,id);
CREATE VIRTUAL TABLE authoring_search USING fts5(id UNINDEXED,kind UNINDEXED,text,characters,tokenize='unicode61');
CREATE TABLE authoring_operations (idempotency_key TEXT PRIMARY KEY,request_hash TEXT NOT NULL,result_json TEXT NOT NULL CHECK(json_valid(result_json)),mutation_json TEXT NOT NULL CHECK(json_valid(mutation_json)));
CREATE TRIGGER authoring_operations_immutable_update BEFORE UPDATE ON authoring_operations BEGIN SELECT RAISE(ABORT,'authoring operations are immutable'); END;
CREATE TRIGGER authoring_operations_immutable_delete BEFORE DELETE ON authoring_operations BEGIN SELECT RAISE(ABORT,'authoring operations are immutable'); END;
CREATE TABLE authoring_selections (id TEXT PRIMARY KEY,request_hash TEXT NOT NULL,payload_json TEXT NOT NULL CHECK(json_valid(payload_json)));
CREATE TRIGGER authoring_selections_immutable_update BEFORE UPDATE ON authoring_selections BEGIN SELECT RAISE(ABORT,'authoring selections are immutable'); END;
CREATE TRIGGER authoring_selections_immutable_delete BEFORE DELETE ON authoring_selections BEGIN SELECT RAISE(ABORT,'authoring selections are immutable'); END;
`}
}
