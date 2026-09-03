# Engine

NovelForge preserves the existing ainovel-cli TUI, headless flow and legacy engine adapter. Phase 5 adds a separate chapter-quality transaction rather than replacing those entry points.

## Deterministic chapter transaction

The quality coordinator is responsible for state transitions, bounded retries/rewrites, artifact persistence and final commit. Model services only return creative/semantic results. The durable state machine and selection policy live in Go.

Default policy:

- maximum rewrites: 2
- Editor threshold: 7.0 / 10
- Continuity WARN may continue only because the deterministic policy allows it
- Continuity FAIL always blocks Finalize

## Persistence and recovery

Project migration 3 stores transactions, state changes, candidates, Fact Proposals, Continuity results, Editor reviews, model-call metadata, Truth-commit bookkeeping and checkpoints. A successful Writer call is persisted as a Draft before any later agent runs. Re-entry resumes from durable artifacts and does not repeat completed model calls.

Finalize is a recoverable saga across SQLite Truth state and the chapter filesystem. Truth appends are idempotent, chapter replacement is hash-checked and recoverable on Windows, and completion is recorded only after checkpoint persistence.

## Model boundary

`ModelInvoker` is injected into the server or agent services. Provider/network retries are bounded and only retry errors explicitly marked retryable. The model-call ledger stores hashes/tokens/status but not full sensitive prompt text in normal logs.

See [QUALITY_GATE.md](QUALITY_GATE.md) and [TRUTH_STORE.md](TRUTH_STORE.md).

## Phase 8 immutable chapter-version boundary

Phase 8 inserts `internal/chapterversion` between candidate prose and the authoritative chapter file. Migration 6 stores immutable `ChapterVersion` rows, append-only review/audit events, one Active Final projection per project/chapter, idempotent operation records, external-SHA state, Finalize sagas, rebuild checkpoints and downstream plan-impact records.

A browser or external editor save never overwrites the Active Final. It appends a `human_revision`, normally parented to the currently active Final. Restore likewise creates a new immutable Draft with restore provenance. Reject records an append-only review event and keeps the rejected version addressable in History.

The explicit human path is:

```text
HUMAN SAVE / EXTERNAL SYNC
  -> immutable human_revision
  -> Librarian proposal
  -> Continuity check
  -> Editor review when configured
  -> Truth conflict evaluation
  -> explicit Accept
  -> recoverable Finalize saga
  -> immutable final + Active Final switch
  -> Human Final Truth/Ledger authority
  -> Chapter-N derived-state rebuild
  -> downstream plan-impact records
```

`Check` and `SyncExternal` may invoke semantic services, but they cannot commit authoritative Truth. `Accept` records approval but still does not commit Truth. Only `Finalize` may create/switch a Final and commit accepted proposal changes.

Generated Phase 5 candidates are bridged into the same ChapterVersion-first finalization path without repeating model calls. This keeps generated and human Finals under one authority, recovery and idempotency model.

External file edits are detected by comparing the normalized chapter file SHA with the immutable Active Final SHA. A mismatch produces `sync_required`; it never silently mutates database state. Explicit sync re-reads and re-hashes the file, rejects time-of-check/time-of-use changes, creates or reuses a matching `human_revision`, then re-runs evaluation.

Finalize is replay-safe across Final creation, Truth supersede/assert events, Narrative Ledger promotion, file replacement, Active Final switch, Chapter-N rebuild and checkpoint persistence. An Accepted Human Final cannot be downgraded by a later generated Final. See [CHAPTER_VERSIONS.md](CHAPTER_VERSIONS.md) for the complete Phase 8 state and failure contract.
