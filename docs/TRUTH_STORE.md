# Structured Truth Store

Phase 4 adds a project-local, append-only Truth authority layer backed by SQLite. It is the durable source for narrative facts, provenance, temporal projections, and conflicts. It is deliberately separate from model memory, browser state, retrieval results, and LLM proposals.

## Ownership and storage

Each project owns its Truth data in:

```text
<project>/.novelforge/project.db
```

Project migration 2 creates the Truth schema through the existing migration runner. The runner enables foreign keys, a bounded busy timeout, WAL mode, migration checksums, pre-migration backup, transactional application, and rollback on failure. The V1 implementation uses `modernc.org/sqlite`, so official builds remain compatible with `CGO_ENABLED=0`.

The `truthstore.Repository` interface is the storage boundary. `truthstore.Store` is the SQLite implementation; the interface preserves a future PostgreSQL option without adding PostgreSQL to v0.1.0.

## Core domain model

The Go domain package defines stable records for:

- `Novel`
- `Entity` and `EntityType`
- `Character` and `CharacterState`
- `Relation`
- `KnowledgeFact` and `KnowledgeHolder`
- `InventoryEvent`
- `TimelineEvent`
- `WorldRule`
- `Provenance`
- `StateEvent`
- `Conflict`

Entity types include `character`, `location`, `organization`, `item`, `ability`, `species`, `concept`, and `event`. The authoritative persistence path is a typed event log plus rebuildable projections rather than mutable per-domain truth rows.

## Temporal model

Every Truth event carries two chapter intervals:

- **valid time** — when the statement is true in the story;
- **knowledge time** — when the statement is available to the authoring system.

The effective interval is the intersection of both ranges:

```text
effective_from = max(valid_from_chapter, known_from_chapter)
effective_to   = min(valid_to_chapter, known_to_chapter)
```

Open-ended upper bounds remain `NULL`. A Chapter-N query includes a fact only when:

```sql
effective_from_chapter <= N
AND (effective_to_chapter IS NULL OR effective_to_chapter >= N)
```

This prevents later character state, inventory acquisition, and knowledge revelation from leaking into earlier chapters. Knowledge may legitimately begin before story validity for a current plan; the two boundaries are never silently collapsed.

## Authority order

Truth Store is a persistence mechanism, not a source rank. The deterministic authority order is:

```text
Accepted Human Final
> Generated Final Chapter
> Current Chapter Plan
> Arc Plan
> Volume Plan
> Story Compass
> LLM Suggestion
```

The API values are:

```text
human_final
generated_final
chapter_plan
arc_plan
volume_plan
story_compass
llm_suggestion
```

Authority ranking is Go-owned logic. An LLM cannot promote its own proposal. A lower-authority event may be recorded for diagnosis, but it cannot explicitly supersede a higher-authority fact.

## Event, projection, and conflict rules

`truth_events` is immutable and append-only. SQLite triggers reject direct updates and deletes. Supported event kinds are:

- `assert`
- `supersede`
- `retract`

A correction must append a new event. `supersede` and `retract` require an explicit `supersedes_event_id` with the same subject and predicate. The replacement cannot begin before the target fact becomes effective, and a fact cannot be superseded twice.

`truth_facts` and `truth_conflicts` are deterministic projections. Distinct values with overlapping effective intervals coexist and produce a conflict instead of using last-write-wins. Conflict history is retained with one of two states:

- `unresolved` — the conflicting interval is still open;
- `resolved` — the conflicting interval has a finite end after an explicit correction or temporal close.

Unresolved conflicts are exposed in Chapter-N state and can block later Finalize logic in Phase 5.

## Provenance

Every event requires:

- `source.type`
- `source.id`
- `source.chapter`
- `source.version`

Optional bounded provenance fields are:

- `source.extractor`
- `source.confirmed_by`
- `source.excerpt`

The event checksum covers the canonical JSON value, temporal bounds, authority, confidence, complete source metadata, supersession target, and UTC creation time. Verification fails closed when an event checksum or projection digest does not match.

## Idempotency

Every write route requires a safe 1–128 character `Idempotency-Key`. The HTTP layer uses the workspace-wide persistent idempotency ledger, while the project Truth Store also protects the event append itself with a unique key and canonical request hash.

- Same key and same canonical request: replay the exact stored HTTP response.
- Same key and different request: return a structured conflict.
- Concurrent duplicate event appends: commit at most one event.

This dual boundary protects both transport retries and direct repository callers.

## Queries and pagination

The store provides:

- stable event listing by sequence;
- Chapter-N state queries;
- conflict history queries;
- batch Chapter-N queries;
- bounded projection rebuild;
- integrity verification.

Filters include subject type, subject ID, predicate, chapter boundary, event sequence, and conflict status. Collection limits are bounded to 500. Batch state queries accept at most 100 selectors and execute as one SQLite statement with window functions, avoiding a per-selector N+1 path.

## Rebuild and verification

`Rebuild(from_chapter)` replays the complete immutable event history in memory, then replaces only projected rows that intersect the requested boundary. Earlier unaffected projection rows remain untouched. Rebuild never mutates source events.

`Verify()` checks:

1. every event checksum;
2. supersession graph validity;
3. authority ordering;
4. expected fact and conflict counts;
5. the deterministic projection digest, including authority rank and conflict status.

## REST API

```text
GET  /api/truth/events
POST /api/truth/events
GET  /api/truth/state
POST /api/truth/state:batch
GET  /api/truth/conflicts
POST /api/truth/rebuild
GET  /api/truth/verify
```

All operations require `project_id`. POST operations require `Idempotency-Key`. Request bodies reject unknown fields, use the common 1 MiB transport limit, and return the common secret-free error envelope with `trace_id`. No response exposes an absolute path, raw SQL, credential, or stack trace.

## Index and scale gate

The temporal projection index is designed for the common query key:

```text
subject_type, subject_id, predicate,
effective_from_chapter, effective_to_chapter,
authority_rank, sequence
```

CI runs a 100,000-fact fixture, asserts that SQLite uses `idx_truth_facts_asof`, and executes a bounded Chapter-N query against that dataset. Formal 100/500/1000-chapter latency benchmarks remain a Phase 13 release requirement.

## Phase boundaries

Phase 4 does not allow an LLM proposal to become authoritative by itself. Phase 5 connects Librarian proposals, Continuity results, Final chapter selection, and atomic Truth commit. Phase 8 uses `Rebuild(from_chapter)` after an accepted human revision. Phase 9 invokes the same repository through durable Autopilot jobs.

## Narrative Ledger integration

Narrative Ledger migration 4 shares the project database and the existing migration, backup, checksum, WAL, busy-timeout, and rollback guarantees. Ledger rows are optimized projections and audit history for Foreshadows and Secrets; the Structured Truth Store remains the authority for general world facts.

Model-originated Ledger writes are accepted only after the Phase 5 coordinator selects and finalizes a continuity-safe candidate. Each accepted change carries source chapter, source version, authority, confidence, and provenance compatible with Truth events. A replayed Finalize operation returns the existing Ledger commit, while a changed payload under the same transaction/idempotency identity fails closed.

Chapter-N Secret visibility is determined independently from authority truth. Holder ranges and public reveal chapters prevent later knowledge from leaking into earlier Planner or Writer context. Rebuild and Human Edit synchronization preserve this boundary together with Truth projections.

## Phase 8 Human Final correction semantics

A Phase 8 `human_revision` is not authoritative merely because a person typed or saved it. The revision remains an immutable candidate until it has passed the explicit review path and is finalized. External chapter-file edits are likewise only evidence until explicit synchronization creates a `human_revision` and evaluates it.

When a Human Final is finalized, the ChapterVersion coordinator converts its persisted proposal into Truth events with `human_final` authority. Existing contradictory generated-final facts are not mutated in place. The coordinator appends explicit supersede/assert events with provenance tied to the accepted Human Final version. The old events remain in `truth_events` for audit and verification.

The rebuild boundary is the edited chapter N. `truthstore.Rebuild(N)` may change projections that intersect Chapter N and later, but it must not contaminate Chapter N-1 or earlier. Phase 8 acceptance Scenario B verifies this with a Chapter 50 correction: the original generated death event remains auditable, the Human Final supersedes it with `alive=true`, adds severe injury and escape facts, Chapter 49 still projects the original pre-edit state, and Chapter 50+ reflects the accepted correction.

Human Final is the highest defined authority. A generated Final cannot later supersede or downgrade an Accepted Human Final fact. Repeated Finalize with the same idempotency identity replays the same Truth result rather than appending duplicate events.

See [CHAPTER_VERSIONS.md](CHAPTER_VERSIONS.md) for the full save/check/accept/finalize/sync transaction and [CONTEXT_COMPILER.md](CONTEXT_COMPILER.md) for how rebuilt Chapter-N state reaches later prompts.
