# NovelForge Architecture

## 1. Purpose

NovelForge is an engineering evolution of ainovel-cli for novels in the 1–3 million Chinese-character range, 300–1000+ chapters, 50–300 characters, and 100,000+ facts/events. The system must support Human Copilot and resumable Autopilot without sending the whole book to an LLM on every chapter.

The repository deliberately keeps the upstream module path and core package layout through v0.1.0. NovelForge capabilities are introduced through repositories, interfaces, adapters, and additive entry points so upstream fixes can still be compared and integrated.

## 2. Executable surfaces

```text
novelforge              -> existing interactive TUI
novelforge --headless   -> existing unattended engine entry
novelforge server       -> embedded REST/SSE/Web entry
cmd/ainovel-cli         -> retained legacy/upstream-compatible command
```

The Web server is additive. It does not replace the TUI, Headless runner, Engine, Store, Checkpoint, Import, Export, Sync, Diagnose, Simulation Profile, or model providers.

## 3. Component model

```text
┌──────────────────────────────────────────────────────────────┐
│ Web / CLI / TUI / Headless                                   │
└──────────────────────────────┬───────────────────────────────┘
                               │
┌──────────────────────────────▼───────────────────────────────┐
│ NovelForge API                                                │
│ REST commands · idempotency · SSE replay · OpenAPI           │
└──────────────────────────────┬───────────────────────────────┘
                               │ EngineService
┌──────────────────────────────▼───────────────────────────────┐
│ Deterministic Engine                                          │
│ state machine · retries · checkpoints · jobs · recovery       │
└──────┬──────────────────────┬──────────────────────┬──────────┘
       │                      │                      │
┌──────▼─────────┐   ┌────────▼─────────┐   ┌────────▼────────┐
│ Agent Runtime  │   │ Context Compiler │   │ Quality Gate    │
│ role workers   │   │ budgeted layers  │   │ continuity/edit │
└──────┬─────────┘   └────────┬─────────┘   └────────┬────────┘
       │                      │                      │
┌──────▼──────────────────────▼──────────────────────▼─────────┐
│ LLM Router                    Truth Store / Event Projection  │
│ providers · fallback          SQLite · FTS5 · provenance      │
└──────────────────────────────┬───────────────────────────────┘
                               │
                    ┌──────────▼──────────┐
                    │ Checkpoint / Backup │
                    └─────────────────────┘
```

## 4. Deterministic versus semantic responsibility

**Go owns:** lifecycle, state transitions, routing, idempotency, retry limits, atomic commit, schema validation, chapter versions, job durability, API, permissions, files, migrations, provenance, event projection, checkpoint, and recovery.

**LLMs own:** premise/world/character ideation, rolling planning, literary drafting, ambiguous continuity judgment, literary review, fact proposals, and style analysis.

An LLM never writes authoritative Truth Store rows directly. It returns schema-validated proposals; the Engine validates and commits them only after the corresponding chapter version becomes Final.

## 5. Authority order

When sources conflict, NovelForge applies the project authority policy:

```text
Accepted Human Final
> Generated Final Chapter
> Current Chapter Plan
> Arc Plan
> Volume Plan
> Story Compass
> LLM Suggestion
```

Truth Store is the persistence mechanism for events, provenance, conflicts, and projections; it is not itself a source rank. Lower-authority proposals cannot silently replace higher-authority facts.

## 6. Agent boundaries

NovelForge keeps the number of agents intentionally small:

- **Architect**: premise, world frame, character frame, theme, ending direction, compass, volume, and arc; never writes chapter prose.
- **Planner**: rolling 3–8 chapter horizon and structured chapter plans.
- **Writer**: prose only; produces a Chapter Draft and cannot mutate Truth.
- **Librarian**: extracts structured Fact Proposals from a candidate chapter and cannot commit Truth.
- **Continuity**: checks state, timeline, relations, inventory, knowledge boundaries, rules, and foreshadow consistency; emits structured PASS/WARN/FAIL results.
- **Editor**: evaluates literary quality, pacing, conflict, emotion, dialogue distinction, hooks, repetition, and configurable forbidden patterns.

The upstream Architect/Writer/Editor/Arbiter behavior remains available while these boundaries are introduced through adapters rather than a destructive rewrite.

## 7. Project and workspace boundary

A configured workspace may contain multiple projects. NovelForge-created projects own:

```text
project-root/
  .novelforge/
    project.json
    project.db
    config.json
    rules/
    skills/
    style/
    output/
    backups/
    trash/
  chapters/
  references/
```

Existing ainovel projects remain readable from their legacy artifacts. Conversion is never implicit: an explicit skeleton import initializes `.novelforge` metadata and the project database in place.

The project repository is the only component allowed to translate an opaque project ID into a host filesystem path. HTTP handlers exchange typed inputs and opaque IDs, never absolute paths. Destructive operations enforce workspace containment, reject traversal and symlink escape, require an exact ID/title confirmation, and refuse protected roots or Git repositories. Default deletion moves data into workspace-private trash; permanent deletion requires an explicit flag.

## 8. SQLite topology and migrations

Phase 2 establishes two durability scopes:

- `<workspace>/.novelforge/server.db` stores API control data such as durable SSE events and idempotency records.
- `<project>/.novelforge/project.db` stores project-local metadata now and later project Truth, chapter versions, jobs, and projections.

Both use the CGo-free `modernc.org/sqlite` driver, foreign keys, a bounded busy timeout, WAL mode, and versioned migrations. `schema_migrations` records version, name, SHA-256 checksum, and UTC application time. Existing databases are checkpointed and backed up before pending migrations; all pending migrations and their records are applied in one transaction. Checksum drift and unknown applied versions fail closed.

V1 does not require PostgreSQL, Redis, Kafka, Elasticsearch, Qdrant, or Kubernetes. Repository interfaces preserve future storage choices without adding those operational dependencies to v0.1.0.

## 9. API durability and transport

`internal/server` owns HTTP and SSE transport. It is separated from filesystem lifecycle (`internal/project`), control persistence (`internal/server/repository`), idempotency, event storage, and the existing engine (`internal/server/engineadapter`).

Every write route requires an `Idempotency-Key`. The workspace database binds a key to the operation, project, exact request hash, state, response status/body, creation time, and expiry. The same request replays the exact response; a different request using the key returns a conflict.

Events are persisted before live fan-out. SSE supports heartbeat, project filtering, `Last-Event-ID`, restart replay, and bounded subscribers. A slow subscriber is disconnected rather than blocking a producer.

REST failures use one safe envelope with code, message, details, retryability, and trace ID. Responses do not serialize raw SQL, stack traces, credentials, authorization headers, provider secrets, or absolute paths. Project collections use stable sorting and bounded pagination.

The current route and storage contract is documented in `docs/PROJECT_API.md` and in the embedded OpenAPI 3.1 document.

## 10. Engine adapter

`EngineService` is the Web-facing runtime boundary. The Phase 2 `LegacyAdapter` delegates start, resume, event, stream, completion, and close operations to the existing `host.Host`. The API therefore does not copy planning, writing, checkpoint, or recovery logic. Durable jobs introduced later can use the same interface.

## 11. Truth Store target

V1 uses project-local SQLite with versioned migrations and Repository interfaces. Core records include Novel, Entity, Character, time-bounded CharacterState, Relation, KnowledgeFact/Holder, Inventory Event, Timeline Event, Fact/Provenance, Foreshadow, Secret, Conflict, StateEvent, and ChapterVersion.

Longitudinal state is event-sourced where it materially improves recomputation. A location change, item acquisition, injury, or death is an immutable event; projections answer “what was true at Chapter N.” Editing Chapter 50 invalidates and rebuilds derived projections after Chapter 50 without contaminating earlier state.

Phase 2 establishes the migration and per-project database foundation only; the full Truth schema belongs to Phase 4.

## 12. Context Compiler target

Context is assembled under an explicit token budget:

1. **Truth** — current character state, relations, knowledge, inventory, timeline, rules, secrets, and active foreshadows.
2. **Narrative** — compass, volume, arc, and current chapter plan.
3. **Recent** — 2–5 recent chapters or summaries.
4. **Historical Retrieval** — structured queries, timeline, foreshadow/relation lookup, FTS5, and optional vector retrieval.
5. **Style** — simulation profile, user rules, skills, voice, and forbidden patterns.

Critical items such as the current plan, POV state, hard world rules, knowledge boundary, required contract beats, and urgent foreshadows are pinned before optional historical text. The daily path may not perform O(n²) whole-book scans.

## 13. Chapter transaction target

```text
PLAN
  -> DRAFT
  -> LIBRARIAN FACT PROPOSAL
  -> CONTINUITY CHECK
  -> EDITOR REVIEW
  -> bounded REWRITE (default max 2)
  -> choose candidate / HOLD severe errors
  -> FINAL VERSION
  -> atomic TRUTH COMMIT
  -> CHECKPOINT
  -> NEXT
```

Writer success followed by Librarian failure leaves a Draft. Continuity failure cannot Finalize. Truth commit failure cannot discard the chosen Final candidate. Checkpoints follow every expensive, independently recoverable step so restart does not repeat completed paid calls.

## 14. Compatibility strategy

- Keep `cmd/ainovel-cli` and the upstream module path through v0.1.0.
- `cmd/novelforge` calls the shared TUI/headless runtime.
- Continue reading `.ainovel` configuration and project artifacts unchanged.
- Keep configuration migration explicit and backed up.
- Introduce new capabilities in separate packages and minimize churn in upstream core files.
- Preserve the existing commit/revision recovery protocols rather than replacing them with Web-only logic.
- Record the exact upstream base and sync procedure in `UPSTREAM_BASE.md` and `docs/UPSTREAM_SYNC.md`.

## 15. License architecture

ainovel-cli code is Apache-2.0 and directly reused. show-me-the-story is MIT and is currently a design/UI reference only; any future direct reuse must retain its copyright and full license text. NovelWriter (AGPL-3.0) and AI-Novel-Writer (GPL-3.0) are clean-room design references only. No source, SQL, tests, prompts, or translated implementation from either copyleft project may enter NovelForge's Apache-2.0 tree.

The resolved Go module graph is recorded in `docs/DEPENDENCY_LICENSES.md` and checked by CI together with the CGo-disabled build.

## 16. Temporal Truth Store

The project database owns an append-only `truth_events` log and rebuildable `truth_facts` / `truth_conflicts` projections. Every event carries separate story-valid and system-knowledge chapter ranges; Chapter-N queries use their intersection so later state and discoveries cannot leak into earlier context. Conflicting values coexist until an authorized event explicitly supersedes or retracts a predecessor. Authority follows Accepted Human Final > Generated Final Chapter > Current Chapter Plan > Arc Plan > Volume Plan > Story Compass > LLM Suggestion, but ranking never causes a silent overwrite. Every source includes an explicit source chapter and source version. See `docs/TRUTH_STORE.md` for the storage, rebuild, verification, idempotency, and API contracts.

## 17. Phase 5 chapter quality transaction

Phase 5 makes the Chapter transaction target concrete in `internal/qualitygate` and project migration 3. The coordinator persists the Draft before invoking Librarian, persists Fact Proposal before Continuity, persists Continuity before Editor, and persists Editor scoring before deterministic candidate selection. Model-facing services receive only typed requests and model-call infrastructure; Writer and Librarian never receive a Truth repository.

The model-call repository gives every paid/semantic operation a request hash and idempotency key. Completed same-hash calls replay without another provider invocation, while key/content conflicts fail closed. Provider retries and structured-output repairs have explicit caps.

Continuity uses the Phase 4 Chapter-N Truth query for inventory, knowledge, location, relations, timeline, world rules and other blocking predicates. RAG is not consulted as authority. A blocking FAIL cannot be superseded by a high Editor score.

Finalization is a recoverable saga: accepted candidate → idempotent generated-final Truth events → hash-checked chapter-file switch → durable checkpoint → `completed`. The chapter file switch preserves a same-directory backup across the Windows replace boundary, and retries do not repeat Truth events or model stages. See `docs/QUALITY_GATE.md`.
