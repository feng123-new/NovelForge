# NovelForge Implementation Status

Last updated: 2026-09-02

## Repository state

- Verified Phase 3 base: `main@617fc20a96b307b0656a6267e3657ea9ff9f5147`.
- Phase 2 PR: [#7 — feat: complete project and API foundation](https://github.com/feng123-new/NovelForge/pull/7) — merged; merged-main CI passed.
- Phase 3 PR: [#8 — feat: add NovelForge web workspace](https://github.com/feng123-new/NovelForge/pull/8) — merged; merged-main CI passed.
- Active Phase 4 branch: `feature/phase-04-truth-store`.
- Active Phase 4 PR: [#10 — feat: add structured truth store](https://github.com/feng123-new/NovelForge/pull/10).
- Phase 4 implementation is present in normal production source paths. Temporary recovery scripts, payload fragments, source-snapshot steps, and Phase-specific self-modifying workflows have been removed from the delivery tree.
- Phase 4 remains formally in progress until the cleaned PR head passes every required job, is squash-merged, and the merge-triggered `main` workflow succeeds.

## Phase status

| Phase | Status | Notes |
| --- | --- | --- |
| 0 — upstream baseline | complete | Imported ainovel-cli baseline and retained provenance. |
| 1 — compatibility | complete | NovelForge runtime isolation, migration, lifecycle smoke, branding, Linux/Windows/Docker gates. |
| 2 — project/API foundation | complete | Project repository, migrations, idempotency, durable events, safe errors, Engine adapter, OpenAPI. |
| 3 — formal Web Workspace | complete | Rebuildable Svelte workspace, real API pages, frontend tests/build/license gates, embedded delivery. |
| 4 — Structured Truth Store | implementation complete; acceptance pending | Clean production source, tests, OpenAPI and documentation are ready for final PR and merged-main validation. |
| 5–13 | not started | Begin only after Phase 4 merged-main acceptance. |

## Completed

### Truth authority layer

- Added project-local append-only `truth_events` with canonical request hashes, event checksums, complete provenance, and deterministic UTC timestamps.
- Added rebuildable `truth_facts`, `truth_conflicts`, and `truth_projection_meta` projections.
- Added SQLite triggers that reject direct event update and delete.
- Added explicit `assert`, `supersede`, and `retract` event kinds.
- Added deterministic authority ordering: LLM Suggestion < Story Compass < Volume Plan < Arc Plan < Current Chapter Plan < Generated Final < Accepted Human Final.
- Enforced same-key explicit supersession, no authority downgrade, no double supersession, and no replacement before the target fact becomes effective.

### Temporal queries and conflict history

- Added separate story-valid and system-knowledge intervals.
- Chapter-N state requires both intervals to include N, preventing future-state and future-knowledge leakage.
- Added stable filtered event listing, paginated Chapter-N state, paginated conflict history, and conflict status filtering.
- Added one-statement batch querying for at most 100 selectors, avoiding a per-selector N+1 path.
- Added `unresolved` and `resolved` conflict history rather than silently deleting evidence after a correction.

### Projection rebuild and integrity

- Added deterministic replay from immutable events.
- Added full rebuild and chapter-bounded rebuild that replace only projections intersecting the boundary.
- Added event checksum verification, supersession graph validation, count verification, and deterministic projection digest verification.
- Projection digest covers temporal bounds, value hash, authority and rank, confidence, supersession, and conflict status.

### Project and API integration

- Added project migration 2 through the existing checksum/backup/rollback migration runner.
- Added `truthstore.Repository` and a project repository adapter using the existing safe project boundary.
- Added Truth REST handlers using the common request-size limit, strict JSON decoder, safe error envelope, and persistent idempotency ledger.
- Added OpenAPI schemas for event append, event pages, Chapter-N state, bounded batch query, conflicts, rebuild, and verification.

## Changed files

Major production paths:

- `.github/workflows/ci.yml`
- `internal/truthstore/model.go`
- `internal/truthstore/domain.go`
- `internal/truthstore/repository.go`
- `internal/truthstore/migration.go`
- `internal/truthstore/store.go`
- `internal/truthstore/batch.go`
- `internal/truthstore/rebuild.go`
- `internal/truthstore/store_test.go`
- `internal/truthstore/scale_test.go`
- `internal/project/truth.go`
- `internal/project/truth_migration.go`
- `internal/project/truth_test.go`
- `internal/server/api.go`
- `internal/server/server.go`
- `internal/server/truth.go`
- `internal/server/truth_test.go`
- `internal/server/openapi.json`
- `internal/server/openapi_test.go`
- `docs/TRUTH_STORE.md`
- `docs/ARCHITECTURE.md`
- `docs/IMPLEMENTATION_STATUS.md`

## Architecture decisions

- Truth is stored as immutable typed events plus rebuildable projections rather than mutable last-write-wins rows.
- Truth Store is not an authority rank; authority is explicit source policy owned by deterministic Go code.
- LLM output remains a proposal. Phase 4 exposes storage boundaries but does not bypass Final chapter acceptance.
- Valid time and knowledge time remain independent so planned future events and later revelations are representable without leakage.
- Batch queries use one SQLite statement; daily Chapter-N paths do not perform an O(n²) full-book scan.
- SQLite is the V1 implementation behind a repository interface; PostgreSQL is not added as a runtime dependency.
- HTTP idempotency reuses the existing workspace ledger, while the event table independently protects direct repository retries.

## Database / migration changes

Project schema migration 2 adds:

```text
truth_events
truth_facts
truth_conflicts
truth_projection_meta
```

It also adds temporal, key, supersession, and conflict indexes plus append-only triggers. Existing project data is retained. The existing runner records the migration checksum, creates a database backup before pending migrations, applies changes transactionally, and restores the original database on failure.

## API changes

Added:

```text
GET  /api/truth/events
POST /api/truth/events
GET  /api/truth/state
POST /api/truth/state:batch
GET  /api/truth/conflicts
POST /api/truth/rebuild
GET  /api/truth/verify
```

POST operations require `Idempotency-Key`. Collection operations use stable ordering and bounded limits. Every operation is represented in OpenAPI 3.1 and route-drift tests.

## UI changes

No Phase 4 Web controls were added. The Phase 3 UI intentionally continues to omit Truth mutation controls until the Phase 5 chapter transaction can safely connect proposals, Continuity, Final selection, and Truth commit. No fake button or browser-authoritative state was introduced.

## Tests executed

Local production-source validation completed with the vendored dependency graph:

```text
Truth Store unit and temporal tests: PASS
Project migration and persistence tests: PASS
Truth API integration and persistent replay tests: PASS
Targeted Truth Store race tests: PASS
100,000-fact temporal index gate: PASS
CGO_ENABLED=0 NovelForge build: PASS
OpenAPI JSON parse and Truth schema checks: PASS
```

The full repository suite passes except for one pre-existing permission assertion when the local container runs as root: a chmod-only read-only-file test expects a write failure, but root can write the file. The same test remains enabled and must pass on GitHub's non-root Linux runner; no assertion has been removed or weakened.

## Test results

- Cleaned Phase 4 PR CI: pending.
- Phase 4 squash merge: pending.
- Merge-triggered `main` CI: pending.
- Phase 4 is not yet marked complete.

## Performance

- Added a CI scale gate with 100,000 projected facts.
- The gate asserts SQLite uses `idx_truth_facts_asof` for the bounded Chapter-N key query.
- Batch selectors are compiled into one SQL statement with window functions.
- Formal p50/p95, allocation, memory, and 100/500/1000-chapter benchmarks remain Phase 13 work; no unmeasured latency claim is made here.

## License review

- No new Go or npm dependency was added.
- The implementation uses the existing Apache-2.0 project dependency graph and CGo-free SQLite driver.
- No source, SQL, tests, components, or prompts were copied from GPL/AGPL clean-room references.
- Existing dependency inventories and license gates remain unchanged and blocking.

## Known issues

- Phase 4 has no known data-corruption or credential-exposure blocker in the cleaned candidate tree.
- Phase 4 is still awaiting authoritative GitHub CI, squash merge, and merged-main CI.
- Librarian proposal extraction, Continuity gating, Editor review, bounded rewrite, and atomic Final-to-Truth commit are Phase 5 work.
- Narrative Ledger, Context Compiler, chapter versions, durable Autopilot, reference libraries, lifecycle tooling, observability, and release work remain Phase 6–13.

## Next phase

After Phase 4 merged-main acceptance, create `feature/phase-05-quality-gate` from that exact verified `main` commit and implement Librarian Fact Proposals, Continuity PASS/WARN/FAIL, Editor review, bounded rewrites, candidate selection, Final preservation, and the chapter commit boundary without removing the mature `commit_chapter` recovery saga.

## Delivery evidence

- Current base main: `617fc20a96b307b0656a6267e3657ea9ff9f5147`.
- Active branch: `feature/phase-04-truth-store`.
- Active PR: [#10](https://github.com/feng123-new/NovelForge/pull/10).
- Final cleaned PR head: pending.
- Final PR CI: pending.
- Main merge commit: pending.
- Main CI: pending.

## Exact resume point

1. Commit the cleaned production tree to PR #10.
2. Confirm the PR diff contains production code, tests, permanent CI, and documentation only.
3. Run and inspect all Go, Frontend, Windows, and Docker jobs for the exact cleaned head.
4. Fix only evidence-based failures without lowering assertions.
5. Mark PR #10 ready and squash-merge only after every required job succeeds.
6. Verify the merge-triggered `main` workflow on the exact merge commit.
7. Record exact PR, commit, CI, merge, and main-CI evidence in a follow-up status commit before starting Phase 5.
