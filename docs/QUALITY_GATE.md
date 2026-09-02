# Chapter Quality Gate

Phase 5 turns chapter generation into a persisted, bounded quality transaction. Creative model output is never the authoritative state machine and no model-facing service owns a database connection.

## Responsibilities

- **Architect** owns premise, world, character frame, theme, ending, Story Compass, volume and arc structure.
- **Planner** emits a strict `ChapterPlan` for a rolling horizon. The current chapter plan carries POV, location, objective, conflict, required beats, forbidden outcomes, knowledge and inventory boundaries, foreshadow obligations, and ending hook.
- **Writer** emits chapter prose only. `WriterService` has no Truth repository and cannot submit authoritative facts.
- **Librarian** emits a `FactProposal` tied to one durable candidate by chapter, source version, SHA and extractor. Proposals are diagnostic/pending data, not Truth events.
- **Continuity** performs deterministic Chapter-N queries against the Truth Store. Free-text retrieval is not an authority source. Result is `PASS`, `WARN` or blocking `FAIL` with structured issues.
- **Editor** scores literary quality from 0–10 and records strengths, weaknesses and revision guidance. Editor score never overrides a Continuity `FAIL`.

## Structured output

`ChapterPlan`, `FactProposal`, `ContinuityResult` and `EditorReview` are stable Go contracts. Model-backed Librarian and Editor adapters use `StrictDecoder`: unknown fields, multiple JSON values and trailing data are rejected. When a repairer is configured, repair is bounded; the default server cap is one repair. Validation failure never becomes an implicit PASS.

Fact proposals keep these independent groups:

- entity changes
- character changes
- relationship changes
- location changes
- inventory changes
- knowledge changes
- timeline events
- world facts
- foreshadow updates
- secrets
- injuries
- cultivation changes

Every proposed fact includes source chapter/version/SHA, extractor, confidence, proposed authority, subject/predicate/object, valid-time and knowledge-time boundaries, and a reason.

## Persistent state machine

The project migration `3 chapter_quality_gate` creates durable quality tables. Transactions use these states:

```text
planned
→ drafting
→ draft_ready
→ librarian_pending
→ facts_proposed
→ continuity_pending
→ continuity_pass | continuity_warn | continuity_fail
→ editor_pending
→ reviewed
→ rewrite_pending (bounded)
→ final_candidate
→ truth_commit_pending
→ checkpoint_pending
→ completed
```

`hold` preserves the last safe artifacts and is resumable only through explicit deterministic transitions. `failed` is terminal. Every transition records transaction ID, chapter, actor, attempt, reason and UTC timestamp. Illegal jumps and backwards attempt counters are rejected in Go.

## Rewrite policy and candidate selection

Defaults:

- `max_rewrites = 2`
- literary threshold `7.0 / 10`
- `WARN` is non-blocking but only because the deterministic default policy explicitly allows it

A `FAIL` candidate cannot be Final. A PASS candidate is preferred. If rewrite budget is exhausted before the literary threshold is reached, the highest Editor score among continuity-safe candidates wins, with the selection reason persisted. If no safe candidate exists, the transaction enters HOLD.

## Model call idempotency and bounded retry

Every model call stores metadata only: ID, idempotency key, project/chapter/transaction, agent, operation, provider/model, request/response hashes, status, attempt, token counts, UTC start/end, and an error code. Full sensitive prompts and raw provider responses are not written to normal logs.

Same key + same request hash replays the stored completed result without another provider call. Same key + different request hash is a structured conflict. Network/provider retry is explicit and bounded to retryable failures; server configuration caps retries at five and defaults to two.

## Chapter-N continuity

`TruthContinuityService` queries authoritative facts at the requested chapter boundary. Blocking predicates include life state, location, inventory, knowledge holder, timeline, world rules, relationships, injury, cultivation and plot sequence. Inventory and knowledge therefore do not use only the latest projection: future state and future knowledge are excluded by the Truth Store valid/known intervals.

## Final commit saga

Only an accepted Final candidate reaches the commit coordinator. The durable path is:

```text
Final candidate
→ idempotent Truth events
→ recoverable chapter-file switch
→ checkpoint
→ completed
```

The coordinator serializes Finalize per project/chapter. Truth append keys derive from the Finalize idempotency key, proposal ID and change index, so replay does not duplicate events. The project file writer uses a same-directory temporary file and content hash. Existing chapter replacement uses a recoverable `.quality-backup` switch so Windows does not depend on POSIX overwrite rename semantics.

Failure boundaries:

- Writer succeeds, Librarian fails: Draft remains.
- Librarian succeeds, Continuity fails: Draft and proposal remain; Truth is unchanged.
- Editor fails: Draft, proposal and continuity result remain.
- Truth commit fails: Final candidate remains in `truth_commit_pending`.
- Crash after Truth append: retry reuses idempotent Truth events.
- Crash after chapter-file switch: retry sees the same final SHA and advances to checkpoint.
- Checkpoint failure: no model stage is repeated.
- Only after checkpoint is durable does state become `completed`.

## HTTP and Web

Routes are project- and chapter-scoped:

```text
POST /api/projects/{id}/chapters/{chapter}/generate
POST /api/projects/{id}/chapters/{chapter}/check
POST /api/projects/{id}/chapters/{chapter}/rewrite
POST /api/projects/{id}/chapters/{chapter}/finalize
GET  /api/projects/{id}/chapters/{chapter}/quality
GET  /api/projects/{id}/chapters/{chapter}/candidates
```

All writes reuse the server-wide 1 MiB request bound, strict JSON decoder, `Idempotency-Key`, opaque project boundary, trace ID and safe error envelope. Candidate prose is intentionally omitted from quality metadata responses. The Chapters Web workspace calls only these real APIs, disables duplicate actions while pending, displays structured errors and reloads authoritative server state after every operation and browser refresh.

Phase 5 deliberately does not add version history, diff, restore or human revision controls; those belong to Phase 8.
