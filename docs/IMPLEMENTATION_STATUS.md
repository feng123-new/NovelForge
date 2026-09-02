# NovelForge Implementation Status

Last updated: 2026-09-02

## Repository state

- Verified base: `main@617fc20a96b307b0656a6267e3657ea9ff9f5147`.
- Phase 0 through Phase 3 are complete on `main`.
- Phase 4 production implementation is on `feature/phase-04-truth-store` and is being delivered by non-draft PR [#12 — feat: add structured truth store](https://github.com/feng123-new/NovelForge/pull/12).
- The implementation exists directly in normal Go, OpenAPI, test, and documentation paths. The current delivery tree contains no `.phase4` payloads, recovery generators, source-materialization workflow, or self-modifying workflow.
- PR #10 was closed without merge because it remained Draft. PR #12 replaces it with the same reviewed production implementation and a clean, synchronized head.

## Phase status

| Phase | Status | Evidence |
| --- | --- | --- |
| 0 — upstream baseline | complete | ainovel-cli provenance and compatibility retained. |
| 1 — compatibility | complete | Runtime isolation, migration, lifecycle, brand, Windows and Docker gates. |
| 2 — project/API foundation | complete | Project repository, SQLite migrations, idempotency, durable events, safe errors and OpenAPI. |
| 3 — formal Web Workspace | complete | Rebuildable Svelte workspace, real APIs, frontend tests/build/audit/license gates. |
| 4 — Structured Truth Store | acceptance in progress | Production code and tests are complete; PR #12 awaits the exact synchronized-head CI, squash merge, and merged-main CI. |
| 5–13 | not started | Phase 5 starts only after Phase 4 merged-main acceptance. |

## Completed

### Truth authority and temporal state

- Immutable, append-only `truth_events` with canonical request hashes, checksums, provenance, UTC timestamps and explicit authority.
- Rebuildable `truth_facts`, `truth_conflicts`, and `truth_projection_meta` projections.
- Explicit assert, supersede and retract semantics with deterministic authority ordering and conflict history.
- Separate story-valid and system-knowledge intervals, so Chapter-N queries exclude future state and future knowledge.
- Stable paginated event, Chapter-N state and conflict queries, plus a bounded one-statement batch query.

### Projection integrity and recovery

- Deterministic full and chapter-bounded replay from immutable events.
- Event checksum, supersession graph, row-count and projection-digest verification.
- Project migration 2 through the existing checksum, backup, transaction and rollback runner.
- No direct Writer or Librarian authority path was introduced; Final acceptance integration remains Phase 5.

### API and tests

Added production routes:

```text
GET  /api/truth/events
POST /api/truth/events
GET  /api/truth/state
POST /api/truth/state:batch
GET  /api/truth/conflicts
POST /api/truth/rebuild
GET  /api/truth/verify
```

Coverage includes temporal isolation, authority, conflict, supersede, rebuild, provenance, persistent idempotency, safe API envelopes, route/schema drift, targeted race tests, CGo-free build, and a 100,000-fact indexed Chapter-N query gate.

## Changed files

Production paths include:

- `.github/workflows/ci.yml`
- `internal/truthstore/`
- `internal/project/truth.go`
- `internal/project/truth_migration.go`
- `internal/project/truth_test.go`
- `internal/server/truth.go`
- `internal/server/truth_test.go`
- `internal/server/openapi.json`
- `internal/server/openapi_test.go`
- `docs/TRUTH_STORE.md`
- `docs/ARCHITECTURE.md`

## Architecture decisions

- Truth is immutable events plus deterministic projections, not mutable last-write-wins rows.
- Authority remains deterministic Go policy; RAG and unaccepted model output are never Truth.
- Valid time and knowledge time are independent.
- Batch Chapter-N queries use one bounded SQLite statement and explicit indexes.
- SQLite remains behind a repository interface and uses the existing CGo-free driver.
- HTTP idempotency and event-store idempotency are independent safeguards.

## Database / migration changes

Project migration 2 adds `truth_events`, `truth_facts`, `truth_conflicts`, `truth_projection_meta`, their temporal/key/conflict indexes, and append-only triggers. Existing project data is retained; pending migrations are backed up and applied transactionally.

## UI changes

None. Phase 4 intentionally adds no Truth mutation controls before Phase 5 provides the chapter quality transaction and Final-to-Truth coordinator.

## Tests executed and results

- Clean production head `d349ca6a3a49e631ffeec03600732458856f715f`: CI run [33545170779](https://github.com/feng123-new/NovelForge/actions/runs/33545170779) — success.
- PR #12 clean production head `d349ca6a3a49e631ffeec03600732458856f715f`: CI run [33585438545](https://github.com/feng123-new/NovelForge/actions/runs/33585438545) — success across Go, Frontend, Windows and Docker.
- Synchronized delivery head includes `main@617fc20a96b307b0656a6267e3657ea9ff9f5147` without changing the validated production tree.
- Exact synchronized-head CI: pending after this status commit.
- Squash merge: pending.
- Merge-triggered `main` CI: pending.

## Performance

The Phase 4 gate projects 100,000 facts and verifies the bounded Chapter-N query uses `idx_truth_facts_asof`. Formal release-scale p50/p95 and 100/500/1000-chapter benchmarks remain later work.

## License review

No new Go or npm dependency was added. Existing Apache-2.0 notices and fail-closed Go/frontend license inventories remain active. No GPL/AGPL reference implementation, SQL, test, prompt, or component was copied.

## Known issues

No known data-corruption or credential-exposure blocker exists in the clean Phase 4 tree. Formal completion still requires PR #12 squash merge and success of the merge-triggered `main` workflow.

## Delivery evidence

- Feature branch: `feature/phase-04-truth-store`.
- Pull request: [#12](https://github.com/feng123-new/NovelForge/pull/12), non-draft.
- Reviewed production head: `d349ca6a3a49e631ffeec03600732458856f715f`.
- Synchronized parent commit: `119bf31a21fce0988856ce800c9f40db7f38d4f4`.
- Final head after this status commit: determined by GitHub contents API.
- PR CI: pending for the final head.
- Main merge commit: pending.
- Main CI: pending.

## Next phase

After the Phase 4 feature PR and merged-main workflow are both successful, create `feature/phase-05-quality-gate` from the exact verified `main` and implement Librarian Fact Proposals, deterministic Continuity PASS/WARN/FAIL, Editor reviews, bounded rewrites, candidate selection, model-call idempotency, persistent chapter transactions, HOLD, and the recoverable Final-to-Truth commit boundary.

## Exact resume point

1. Verify the standard CI workflow for the final PR #12 head.
2. Squash-merge PR #12 only after every Go, Frontend, Windows and Docker job succeeds.
3. Verify the merge-triggered `main` workflow on the exact merge SHA.
4. Record the final PR head, PR CI run, merge SHA, merged-main CI run and completion status.
5. Create `feature/phase-05-quality-gate` from that verified `main` SHA.
