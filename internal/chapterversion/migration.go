package chapterversion

import "github.com/voocel/ainovel-cli/internal/db/migrate"

// Migration adds the immutable ChapterVersion production model and the durable
// coordination records used by Phase 8. Chapter bodies and provenance are
// append-only; mutable tables below are projections/pointers or resumable saga
// state and never rewrite historical version content.
func Migration() migrate.Migration {
	return migrate.Migration{
		Version: 6,
		Name:    "chapter_versions_human_sync",
		SQL: `
CREATE TABLE chapter_versions (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    chapter INTEGER NOT NULL CHECK(chapter >= 1),
    version_number INTEGER NOT NULL CHECK(version_number >= 1),
    version_type TEXT NOT NULL CHECK(version_type IN ('draft','continuity_fix','editor_revision','human_revision','final','rejected')),
    content TEXT NOT NULL,
    content_sha TEXT NOT NULL CHECK(length(content_sha) = 64),
    parent_version_id TEXT REFERENCES chapter_versions(id) ON DELETE RESTRICT,
    author_type TEXT NOT NULL CHECK(author_type IN ('writer','librarian','editor','human','restore','system')),
    provider TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    prompt_hash TEXT NOT NULL DEFAULT '',
    review_json TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(review_json)),
    continuity_json TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(continuity_json)),
    provenance_json TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(provenance_json)),
    created_at TEXT NOT NULL,
    UNIQUE(project_id, chapter, version_number)
);
CREATE INDEX idx_chapter_versions_project_chapter_created
    ON chapter_versions(project_id, chapter, created_at, version_number);
CREATE INDEX idx_chapter_versions_parent
    ON chapter_versions(parent_version_id);
CREATE INDEX idx_chapter_versions_content_sha
    ON chapter_versions(project_id, chapter, content_sha);
CREATE TRIGGER chapter_versions_parent_same_chapter_insert
BEFORE INSERT ON chapter_versions
WHEN NEW.parent_version_id IS NOT NULL
BEGIN
    SELECT CASE WHEN NOT EXISTS(
        SELECT 1 FROM chapter_versions p
        WHERE p.id=NEW.parent_version_id AND p.project_id=NEW.project_id AND p.chapter=NEW.chapter
    ) THEN RAISE(ABORT, 'chapter version parent must belong to same project and chapter') END;
END;
CREATE TRIGGER chapter_versions_immutable_update
BEFORE UPDATE ON chapter_versions BEGIN
    SELECT RAISE(ABORT, 'chapter_versions is immutable');
END;
CREATE TRIGGER chapter_versions_immutable_delete
BEFORE DELETE ON chapter_versions BEGIN
    SELECT RAISE(ABORT, 'chapter_versions is immutable');
END;

CREATE TABLE chapter_version_events (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE,
    project_id TEXT NOT NULL,
    chapter INTEGER NOT NULL CHECK(chapter >= 1),
    version_id TEXT NOT NULL REFERENCES chapter_versions(id) ON DELETE RESTRICT,
    event_type TEXT NOT NULL CHECK(event_type IN (
        'created','accept','reject','restore','finalize','active_final_switched',
        'external_change_detected','sync_started','sync_completed',
        'rebuild_started','rebuild_completed','rebuild_failed'
    )),
    reason TEXT NOT NULL DEFAULT '',
    payload_json TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(payload_json)),
    created_at TEXT NOT NULL
);
CREATE INDEX idx_chapter_version_events_project_chapter
    ON chapter_version_events(project_id, chapter, sequence);
CREATE INDEX idx_chapter_version_events_version
    ON chapter_version_events(version_id, sequence);
CREATE TRIGGER chapter_version_events_append_only_update
BEFORE UPDATE ON chapter_version_events BEGIN
    SELECT RAISE(ABORT, 'chapter_version_events is append-only');
END;
CREATE TRIGGER chapter_version_events_append_only_delete
BEFORE DELETE ON chapter_version_events BEGIN
    SELECT RAISE(ABORT, 'chapter_version_events is append-only');
END;

CREATE TABLE chapter_active_finals (
    project_id TEXT NOT NULL,
    chapter INTEGER NOT NULL CHECK(chapter >= 1),
    version_id TEXT NOT NULL REFERENCES chapter_versions(id) ON DELETE RESTRICT,
    authority TEXT NOT NULL CHECK(authority IN ('generated_final','human_final')),
    activated_at TEXT NOT NULL,
    PRIMARY KEY(project_id, chapter),
    UNIQUE(version_id)
);
CREATE INDEX idx_chapter_active_finals_version
    ON chapter_active_finals(version_id);
CREATE TRIGGER chapter_active_final_validate_insert
BEFORE INSERT ON chapter_active_finals BEGIN
    SELECT CASE WHEN NOT EXISTS(
        SELECT 1 FROM chapter_versions v
        WHERE v.id=NEW.version_id AND v.project_id=NEW.project_id AND v.chapter=NEW.chapter
          AND v.version_type='final'
    ) THEN RAISE(ABORT, 'active final must reference a final version in the same chapter') END;
END;
CREATE TRIGGER chapter_active_final_validate_update
BEFORE UPDATE ON chapter_active_finals BEGIN
    SELECT CASE WHEN NOT EXISTS(
        SELECT 1 FROM chapter_versions v
        WHERE v.id=NEW.version_id AND v.project_id=NEW.project_id AND v.chapter=NEW.chapter
          AND v.version_type='final'
    ) THEN RAISE(ABORT, 'active final must reference a final version in the same chapter') END;
END;

CREATE TABLE chapter_revision_operations (
    idempotency_key TEXT PRIMARY KEY,
    operation TEXT NOT NULL,
    project_id TEXT NOT NULL,
    chapter INTEGER NOT NULL CHECK(chapter >= 1),
    version_id TEXT NOT NULL DEFAULT '',
    request_hash TEXT NOT NULL CHECK(length(request_hash)=64),
    status TEXT NOT NULL CHECK(status IN ('pending','completed','failed')),
    result_json TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(result_json)),
    created_at TEXT NOT NULL,
    completed_at TEXT
);
CREATE INDEX idx_chapter_revision_operations_project_chapter
    ON chapter_revision_operations(project_id, chapter, operation, created_at);

CREATE TABLE chapter_external_state (
    project_id TEXT NOT NULL,
    chapter INTEGER NOT NULL CHECK(chapter >= 1),
    active_version_id TEXT NOT NULL DEFAULT '',
    expected_sha TEXT NOT NULL DEFAULT '',
    observed_sha TEXT NOT NULL DEFAULT '',
    observed_at TEXT NOT NULL,
    sync_required INTEGER NOT NULL DEFAULT 0 CHECK(sync_required IN (0,1)),
    PRIMARY KEY(project_id, chapter)
);
CREATE INDEX idx_chapter_external_pending
    ON chapter_external_state(project_id, sync_required, chapter);

CREATE TABLE derived_state_rebuilds (
    operation_id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    boundary_chapter INTEGER NOT NULL CHECK(boundary_chapter >= 1),
    source_version TEXT NOT NULL,
    state TEXT NOT NULL CHECK(state IN ('pending','running','completed','failed')),
    current_step TEXT NOT NULL,
    affected_json TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(affected_json)),
    before_digest TEXT NOT NULL DEFAULT '',
    after_digest TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL,
    completed_at TEXT,
    error_code TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_derived_state_rebuilds_boundary_state
    ON derived_state_rebuilds(project_id, boundary_chapter, state, started_at);

CREATE TABLE chapter_plan_impacts (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    source_version TEXT NOT NULL,
    boundary_chapter INTEGER NOT NULL CHECK(boundary_chapter >= 1),
    plan_id TEXT NOT NULL,
    chapter INTEGER NOT NULL CHECK(chapter >= 1),
    severity TEXT NOT NULL CHECK(severity IN ('info','warning','blocking')),
    affected_fact TEXT NOT NULL,
    previous_assumption TEXT NOT NULL,
    new_truth TEXT NOT NULL,
    action_required TEXT NOT NULL,
    reason TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(project_id, source_version, plan_id, affected_fact)
);
CREATE INDEX idx_chapter_plan_impacts_project_chapter
    ON chapter_plan_impacts(project_id, chapter, severity, id);

CREATE TABLE chapter_finalize_sagas (
    operation_id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    chapter INTEGER NOT NULL CHECK(chapter >= 1),
    candidate_version_id TEXT NOT NULL REFERENCES chapter_versions(id) ON DELETE RESTRICT,
    final_version_id TEXT NOT NULL DEFAULT '',
    authority TEXT NOT NULL CHECK(authority IN ('generated_final','human_final')),
    state TEXT NOT NULL CHECK(state IN ('pending','running','completed','failed')),
    current_step TEXT NOT NULL,
    proposal_json TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(proposal_json)),
    truth_event_ids_json TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(truth_event_ids_json)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    error_code TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_chapter_finalize_sagas_project_chapter_state
    ON chapter_finalize_sagas(project_id, chapter, state, created_at);

CREATE TABLE chapter_version_checkpoints (
    operation_id TEXT PRIMARY KEY REFERENCES chapter_finalize_sagas(operation_id) ON DELETE RESTRICT,
    project_id TEXT NOT NULL,
    chapter INTEGER NOT NULL CHECK(chapter >= 1),
    version_id TEXT NOT NULL REFERENCES chapter_versions(id) ON DELETE RESTRICT,
    final_sha TEXT NOT NULL CHECK(length(final_sha)=64),
    created_at TEXT NOT NULL
);
CREATE INDEX idx_chapter_version_checkpoints_chapter
    ON chapter_version_checkpoints(project_id, chapter, created_at);
`,
	}
}
