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
