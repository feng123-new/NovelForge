# Chapter Versions

Phase 8 makes chapter text an immutable, auditable production artifact. Chapter files remain the human-visible representation of the active final, while SQLite stores the complete version graph, workflow events, idempotency evidence, external-file state, finalization saga state, rebuild state and plan impact.

## Core model

`chapter_versions` is append-only for chapter content and source provenance. A row records an opaque ID, project, positive chapter, monotonically increasing `version_number`, version type, normalized content, SHA-256, optional parent version, author type, model/provider/prompt hash when applicable, review, continuity, provenance and UTC creation time.

Supported version types are `draft`, `continuity_fix`, `editor_revision`, `human_revision`, `final` and `rejected`. Workflow accept/reject/finalize state is projected from append-only events rather than rewriting immutable chapter content. Historical finals and rejected revisions are retained.

A parent must belong to the same project and chapter. `(project_id, chapter, version_number)` is unique. Database triggers reject mutation of immutable content/source columns and reject version deletion.

## Active Final

`chapter_active_finals` is the single authoritative pointer for a project's chapter. The primary key on `(project_id, chapter)` guarantees at most one Active Final. A trigger guarantees the pointer references a `final` ChapterVersion in the same project/chapter.

Switching the pointer appends `active_final_switched`. Old finals remain immutable history. Generated finals use `generated_final` authority. Accepted Human Final uses `human_final`, matching the Truth Store's highest accepted chapter authority. Once a Human Final is active, a generated final cannot downgrade it.

Only an immutable `final` ChapterVersion is allowed to submit Truth. Draft, editor, restore and human revisions must pass evaluation, acceptance and finalization first.

## Human Save

The Web Versions workspace and REST API save chapter edits by creating a new `human_revision`. Saving never updates a prior version and never moves Active Final. The current Active Final becomes the parent when present.

Every write requires `Idempotency-Key`. Replaying the same key and request returns the original version. Reusing a key with different content or request identity returns `IDEMPOTENCY_CONFLICT`.

Human revisions never claim provider/model provenance.

## Check, Accept and Reject

`Check` reruns the Phase 7 quality pipeline for the selected immutable content: Librarian Fact Proposal, Continuity, Editor review and Truth conflict resolution. The result is persisted as candidate evaluation evidence.

`Accept` marks a checked candidate eligible for Finalize; it does not submit Truth. Continuity `FAIL`/blocking prevents acceptance and finalization. `Reject` is append-only, requires a reason, keeps content/review/continuity/provenance, and keeps the version available for History and Diff. The current Active Final cannot be rejected in place.

## Finalize saga

Finalize is an idempotent recoverable saga. The production sequence is:

1. validate version, acceptance, rejection, continuity and authority;
2. create/reuse an immutable `final` version;
3. commit Truth using generated or Human Final authority;
4. commit Narrative Ledger accepted-final effects;
5. write the chapter file and verify its SHA boundary;
6. switch the unique Active Final;
7. refresh the chapter context document;
8. for Human Final, invalidate and rebuild derived state from Chapter N;
9. persist a chapter-version checkpoint;
10. clear external-sync state only after Human Final completion;
11. append finalization audit and complete the idempotent operation.

Saga evidence is stored in `chapter_finalize_sagas` and `chapter_version_checkpoints`. Truth event IDs are persisted so retries do not repeat Truth commits. Finalization supports deterministic test fault points after version creation, Truth, Ledger, chapter-file write, Active Final switch, before checkpoint and after checkpoint.

A successful replay returns the prior result. Concurrent finalization is constrained by persisted operation/saga state and the one-row Active Final projection.

## Diff

`GET /api/projects/{id}/chapters/{chapter}/diff` compares two explicit opaque version IDs from the same project/chapter. Modes are `inline` and `side_by_side`.

The response includes both version IDs and SHAs, structured hunks, old/new line coordinates, additions, deletions, unchanged count, truncation and an optional next cursor. Diff input is bounded by chapter byte limits, output limits and stable cursor pagination. The implementation operates only on stored chapter content; it does not accept filesystem paths and never reads outside the project workspace.

The Web Versions page exposes both modes and a Next Chunk action when the server returns `next_cursor`.

## Restore

Restore creates a new version with `author_type=restore`, content copied from the selected historical version, parent pointing to that source, and restore provenance. Restore never overwrites the source, never deletes the current final and never promotes itself to Active Final. It must be checked, accepted and finalized like any other candidate. Restore is idempotent.

## External SHA detection

For a finalized chapter, the server resolves the single regular chapter file inside the project's `chapters` directory, rejects symlink escape, bounds file size, requires valid UTF-8, normalizes content and computes the observed SHA.

If observed SHA differs from Active Final SHA, `chapter_external_state.sync_required` becomes true and `external_change_detected` is appended. The database final is not overwritten and the file is not automatically trusted as Truth. API/UI expose safe short SHA summaries without absolute paths.

## Explicit Sync

`POST /api/projects/{id}/chapters/{chapter}/sync` rechecks the observed SHA to prevent time-of-check/time-of-use drift. The external content is stored as a `human_revision` whose parent is the current Active Final and whose provenance records original/observed SHA.

Sync then reruns Librarian Fact Proposal, Continuity and Truth conflict checks. Failures preserve the human revision and leave the old Active Final and `sync_required` visible. No Truth is committed by Sync itself.

After explicit acceptance and Finalize, the resulting Human Final receives `human_final` authority. Conflicting lower-authority generated Truth is superseded rather than deleted. Sync state is cleared only after the full finalization/rebuild/checkpoint boundary succeeds.

Repeated Sync and Finalize are idempotent and reuse their persisted operation evidence.

## Derived state invalidation and boundary rebuild

Accepted Human Final invalidates context documents from Chapter N before replay. Truth Store boundary rebuild recomputes authoritative projection from N while retaining raw Truth events. Context FTS chapter-final documents are rebuilt from current Active Finals. Rebuild status is persisted in `derived_state_rebuilds` with boundary, source version, current step, affected-state summary, before/after digests and error code.

Chapter N-1 and earlier are not part of the rebuild boundary. The system never deletes historical ChapterVersions or original Truth events. Narrative Ledger impacts are measured at the same boundary, and Human Final ledger commit is idempotent so retries do not duplicate accepted-final events.

While rebuild is incomplete, API state reports `running`/`failed` rather than pretending stale projections are ready.

## Plan Impact

Human Final fact changes create structured entries in `chapter_plan_impacts` beginning with Chapter N+1. Each entry records plan ID, downstream chapter, severity, affected fact, previous assumption, new Truth, required action, reason and source version. Plans are marked for review/replan; Phase 8 does not silently rewrite later plans. Phase 9 Autopilot can consume this persisted impact boundary.

## REST API

Phase 8 production routes are included in the merged OpenAPI 3.1 document:

- `GET /api/projects/{id}/chapters/{chapter}`
- `GET|POST /api/projects/{id}/chapters/{chapter}/versions`
- `GET /api/projects/{id}/chapters/{chapter}/versions/{version}`
- `GET /api/projects/{id}/chapters/{chapter}/diff`
- `POST .../versions/{version}/check`
- `POST .../versions/{version}/restore`
- `POST .../versions/{version}/accept`
- `POST .../versions/{version}/reject`
- `POST .../versions/{version}/finalize`
- `GET /api/projects/{id}/chapters/{chapter}/sync-status`
- `POST /api/projects/{id}/chapters/{chapter}/sync`
- `GET /api/projects/{id}/chapters/{chapter}/rebuild`
- `GET /api/projects/{id}/chapters/{chapter}/plan-impact`

Version lists default to excluding content and use bounded stable pagination/filtering. All writes use the existing strict JSON decoder, request limits, project/chapter boundary checks, Safe Error Envelope, trace IDs and server-side idempotency handling.

## Web workspace

The `Versions` page is a real backend-backed chapter editor/reviewer. It provides History, Active/Rejected/Human Final badges, Markdown editor, Save Human Revision, Check, Accept, Reject, Restore as New Version, Finalize, external-edit warning and explicit Sync, rebuild state, plan impact, and Inline/Side-by-side Diff.

After every mutation it reloads server authority. Pending operations disable repeat clicks. Structured server errors retain code/trace ID and do not expose filesystem paths.

## Scenario B

The Phase 8 acceptance scenario changes Chapter 50 from “Character A died” to “Character A was severely injured, escaped and remains alive”. The automated path must prove:

- the external SHA mismatch is detected without overwriting the original final;
- explicit Sync creates one human revision parented to the original final;
- deterministic Librarian extraction proposes severe injury, alive and escaped;
- Continuity/Truth conflict checks run;
- acceptance/finalization creates the one Active Human Final;
- the old death Truth event remains audit history and is superseded by higher authority;
- boundary rebuild begins at Chapter 50;
- Chapter 49 digest/query state is unchanged;
- Chapter 50+ projections/context reflect the corrected state;
- Chapter 51+ plan impact is recorded;
- repeated Sync/Finalize/Rebuild do not duplicate versions or authoritative events;
- recovery fault points converge to consistent Active Final, chapter SHA, Truth, Ledger and checkpoint.

Scenario B uses deterministic fakes; no commercial LLM credential is required.

## Migration and recovery

Phase 8 uses project Migration 6 plus the chapter-version counter migration registered through the existing safe migration runner. Migration continues to inherit checksum validation, transactionality, pre-migration backup, rollback, unknown-version rejection, foreign keys, busy timeout and the repository's validated SQLite concurrency policy.

The schema adds indexes for project/chapter version number and creation time, parent lookup, content SHA, pending external sync and rebuild boundary/state.

## Security and limits

- chapter IDs/version IDs are values, not paths;
- chapter filesystem resolution rejects symlinks and escape;
- external chapter content is bounded to 2 MiB and must be valid UTF-8;
- Diff is bounded and paginated and never accepts an absolute path;
- API errors do not return raw SQL, stack traces, secrets or absolute paths;
- provider/model metadata is only recorded for model-authored versions;
- no API key/provider secret is copied during Restore or returned in provenance.

## Known limits

Phase 8 intentionally does not add the Phase 9 durable Autopilot job queue and does not automatically rewrite all impacted future plans. Rebuild is boundary-scoped and uses the existing Truth Store/Context/Narrative Ledger primitives. Import/export/backup product features remain later phases.
