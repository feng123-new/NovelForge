# NovelForge Implementation Status

Last updated: 2026-09-02

## Repository state

- Verified Phase 3/main base before Phase 4 landing: `617fc20a96b307b0656a6267e3657ea9ff9f5147`.
- Phase 2 PR: [#7 — feat: complete project and API foundation](https://github.com/feng123-new/NovelForge/pull/7) — merged; merged-main CI passed.
- Phase 3 PR: [#8 — feat: add NovelForge web workspace](https://github.com/feng123-new/NovelForge/pull/8) — merged; merged-main CI passed.
- Phase 4 prerequisite landing branch: `fix/phase-04-truth-store-landing`.
- Phase 4 prerequisite landing PR: [#13 — feat: add structured truth store](https://github.com/feng123-new/NovelForge/pull/13).
- Phase 4 production implementation is in normal source paths. The clean landing diff contains no recovery payloads, source generators, or Phase-specific self-modifying workflows.
- Phase 4 remains formally in progress until PR #13 passes the complete workflow, is squash-merged, and the merge-triggered `main` workflow succeeds.

## Phase status

| Phase | Status | Notes |
| --- | --- | --- |
| 0 — upstream baseline | complete | Imported ainovel-cli baseline and retained provenance. |
| 1 — compatibility | complete | NovelForge runtime isolation, migration, lifecycle smoke, branding, Linux/Windows/Docker gates. |
| 2 — project/API foundation | complete | PR #7 merged; merged-main CI passed. |
| 3 — formal Web Workspace | complete | PR #8 merged; final PR CI and merged-main CI passed. |
| 4 — Structured Truth Store | implementation complete; acceptance pending | PR #13 contains clean production code, tests, OpenAPI and documentation. |
| 5–13 | not started | Phase 5 starts only after Phase 4 merged-main acceptance. |

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
- Added unresolved/resolved conflict history instead of silently deleting evidence.

### Projection rebuild and integrity

- Added deterministic replay from immutable events.
- Added full rebuild and chapter-bounded rebuild that replace only projections intersecting the boundary.
- Added event checksum verification, supersession graph validation, count verification, and deterministic projection digest verification.
- Projection digest covers temporal bounds, value hash, authority/rank, confidence, supersession, and conflict status.

### Project and API integration

- Added project migration 2 through the existing checksum/backup/rollback migration runner.
- Added `truthstore.Repository` and a project repository adapter using the existing safe project boundary.
- Added Truth REST handlers using the common request-size limit, strict JSON decoder, safe error envelope, and persistent idempotency ledger.
- Added OpenAPI schemas for event append, event pages, Chapter-N state, bounded batch query, conflicts, rebuild, and verification.

## Changed Files

- `.github/workflows/ci.yml`
- `internal/truthstore/*.go`
- `internal/project/truth*.go`
- `internal/server/truth*.go`
- `internal/server/api.go`
- `internal/server/server.go`
- `internal/server/openapi.json`
- `internal/server/openapi_test.go`
- `docs/TRUTH_STORE.md`
- `docs/ARCHITECTURE.md`
- `docs/IMPLEMENTATION_STATUS.md`

## Architecture Decisions

- Truth is immutable typed events plus rebuildable projections, not mutable last-write-wins rows.
- Authority is explicit deterministic policy; an LLM does not decide authoritative state.
- LLM output remains a proposal. Phase 4 does not bypass Final chapter acceptance.
- Valid time and knowledge time are independent so planned future state and later revelation can be represented without leakage.
- Batch queries use one SQLite statement; Chapter-N paths do not perform an O(n²) full-book scan.
- SQLite remains the V1 implementation behind a repository boundary.
- HTTP idempotency reuses the existing workspace ledger while Truth events independently protect direct repository retries.

## Database / Migration Changes

Project schema migration 2 adds:

```text
truth_events
truth_facts
truth_conflicts
truth_projection_meta
```

It also adds temporal/key/supersession/conflict indexes plus append-only triggers. Existing project data is retained. The existing runner records migration checksums, creates a backup before pending migrations, applies changes transactionally, and restores the original database on failure.

## API Changes

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

## UI Changes

No Phase 4 Web controls are added. The Phase 3 UI continues to omit Truth mutation controls until the Phase 5 quality transaction can safely connect proposals, continuity, Final selection and Truth commit. No fake button or browser-authoritative state is introduced.

## Tests Executed

The clean candidate carries and/or CI executes:

```text
gofmt drift check
GOWORK=off go vet ./...
GOWORK=off go test -buildvcs=false -count=1 ./...
Truth Store targeted race tests
100,000-fact Chapter-N index gate
OpenAPI route/schema validation
CGO_ENABLED=0 NovelForge build
Go dependency/license inventory drift and policy gate
project migration append/idempotency/rollback/race tests
shell syntax, lifecycle smoke and brand audit
npm ci/check/test/build/audit/license inventory
frontend build drift
Windows full tests/build
Docker build
```

## Test Results

- PR #13 CI: pending on the exact final head.
- Phase 4 squash merge: pending.
- Merge-triggered `main` CI: pending.
- Phase 4 is not complete until both PR CI and merged-main CI succeed.

## Performance

- The permanent CI gate exercises 100,000 projected facts.
- It asserts SQLite uses `idx_truth_facts_asof` for the bounded Chapter-N key query.
- Batch selectors are compiled into one SQL statement with window functions.
- Formal p50/p95 and 100/500/1000-chapter release benchmarks remain later-phase work.

## License Review

- No new Go or npm dependency is introduced by Phase 4.
- The implementation uses the existing Apache-2.0 dependency graph and CGo-free SQLite driver.
- No source, SQL, tests, components, or prompts are copied from GPL/AGPL clean-room references.
- Existing dependency inventories and fail-closed license gates remain blocking.

## Known Issues

- No known Phase 4 data-corruption or credential-exposure blocker is recorded in the clean candidate.
- Acceptance is still pending authoritative GitHub CI, squash merge, and merged-main CI.
- Librarian proposals, Continuity, Editor, bounded rewrites, and Final-to-Truth commit are Phase 5 work.

## Next Phase

After Phase 4 merged-main acceptance, create `feature/phase-05-quality-gate` from the exact accepted `main` and implement the persisted bounded chapter quality transaction.

## Feature Branch

`fix/phase-04-truth-store-landing`

## Final Head Commit

Pending final PR-head CI.

## Pull Request

[#13 — feat: add structured truth store](https://github.com/feng123-new/NovelForge/pull/13)

## PR CI Result

Pending.

## Main Merge Commit

Pending.

## Main CI Result

Pending.

## Exact Resume Point

1. Run the complete PR #13 workflow on its exact final head and inspect every job.
2. Fix evidence-based failures without weakening a gate.
3. Squash-merge only after the entire PR workflow succeeds.
4. Verify the merge-triggered workflow on the exact main merge commit.
5. Record exact Phase 4 evidence before creating `feature/phase-05-quality-gate`.
