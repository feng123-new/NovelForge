
# NovelForge Implementation Status

Last updated: 2026-09-02

## Repository state

- Phase 2 PR #7, Phase 3 PR #8, Phase 4 PR #13, and Phase 5 PR #19 are merged and their merge-triggered main workflows passed.
- Accepted Phase 5 evidence includes production merge `831cb2983ce851063ce9fb650eaebb14f6ad44c1`, PR CI `33591270069`, and merged-main CI `33591405462`.
- Current accepted base for Phase 6 is `main@77bff5dbbde74bf928129d614b168ac704ad3969`.
- Phase 6 delivery is PR #23 from `feature/phase-06-narrative-ledger`.
- Phase 6 final PR head, successful CI run, merge SHA, and merged-main CI must be recorded only after those events occur.

## Phase status

| Phase | Status | Notes |
| --- | --- | --- |
| 0 — upstream baseline | complete | ainovel-cli baseline retained with provenance. |
| 1 — compatibility | complete | Runtime isolation, migration, lifecycle, branding, Linux/Windows/Docker gates. |
| 2 — project/API foundation | complete | PR #7 merged and main CI passed. |
| 3 — formal Web Workspace | complete | PR #8 merged and main CI passed. |
| 4 — Structured Truth Store | complete | PR #13 merged and main CI passed; evidence correction #14 passed. |
| 5 — Librarian / Continuity / Quality Gate | complete | PR #19 merged; PR and merged-main CI passed. |
| 6 — Narrative Ledger | delivery candidate | Production code, Migration 4, API, OpenAPI, Web, tests and docs are in PR #23; completion still requires final exact-head CI, squash merge, and merged-main CI. |
| 7–13 | not started | Phase 7 starts only from an accepted Phase 6 main. |

## Phase 6 delivery candidate

### Completed

- Added project Migration 4 and indexed SQLite storage for Foreshadows, Secrets, temporal holders, immutable audit events, operation idempotency, accepted-Final commits, and projection metadata.
- Added deterministic Foreshadow lifecycle validation and Chapter-N `OVERDUE` calculation without storing overdue as a mutable state.
- Added independent Secret authority truth, public reveal boundaries, and Chapter-N holder ranges with Truth-compatible provenance.
- Integrated accepted Phase 5 Finalize with replay-safe Ledger commits; rejected drafts, Continuity failures, and HOLD states cannot mutate the Ledger.
- Added `NarrativeLedgerContextProvider` with mandatory overdue/critical Foreshadows, authorized Secret summaries, and truth-free unknown knowledge boundaries.
- Added real Dashboard and Diagnostics projections, stable pagination/filtering, explicit ordering, and indexed overdue queries.
- Added REST/OpenAPI routes and typed Web API methods for Foreshadows, Secrets, holders, Dashboard, Diagnostics, and Planner Context.
- Added real Foreshadows and Secrets pages with pending, success, empty, error, filter, lifecycle, holder, and reveal behavior.
- Removed the accidental `docs/.phase7-tree-probe` from the delivery diff.

### Changed Files

Major Phase 6 paths include:

- `internal/narrativeledger/*.go`
- `internal/project/ledger.go`
- `internal/project/ledger_migration.go`
- `internal/qualitygate/coordinator*.go`
- `internal/server/ledger.go`
- `internal/server/ledger_test.go`
- `internal/server/openapi_phase6.json`
- `web/src/pages/Foreshadows.svelte`
- `web/src/pages/Foreshadows.test.ts`
- `web/src/pages/Secrets.svelte`
- `web/src/pages/Secrets.test.ts`
- `web/src/lib/api.ts`
- `web/src/lib/types.ts`
- `web/dist/assets/app.js`
- `web/dist/assets/app.css`
- `docs/NARRATIVE_LEDGER.md`
- `docs/ARCHITECTURE.md`
- `docs/TRUTH_STORE.md`
- `docs/WEB.md`
- `docs/IMPLEMENTATION_STATUS.md`
- `.github/workflows/ci.yml`

### Architecture Decisions

- Narrative Ledger is a deterministic projection and audit subsystem, not a second Truth authority.
- Writer, Librarian, retrieval, and RAG do not receive a Ledger repository. Only a deterministic accepted-Final coordinator or explicit local human API can write.
- `OVERDUE` is derived from Chapter-N and persisted lifecycle state; it cannot be manually set.
- Secret authority truth and role knowledge are distinct. Unknown Planner boundaries never include truth text.
- Accepted-Final replay uses stable transaction/idempotency keys so recovery cannot duplicate Ledger events.
- Phase 6 supplies stable mandatory Planner items; token allocation and trimming remain Phase 7 work.

### Database / Migration Changes

Project migration 4 adds:

```text
narrative_ledger_operations
narrative_ledger_commits
foreshadows
foreshadow_entities
foreshadow_arcs
foreshadow_events
secrets
secret_holders
secret_events
narrative_ledger_meta
```

Existing migration checksums, pre-migration backup, transactional apply, rollback, WAL, foreign keys, and bounded busy timeout remain in force. Immutable event triggers reject update/delete of Ledger audit history.

### API Changes

Added read routes:

```text
GET /api/projects/{id}/foreshadows
GET /api/projects/{id}/foreshadows/{foreshadow}
GET /api/projects/{id}/secrets
GET /api/projects/{id}/secrets/{secret}
GET /api/projects/{id}/ledger/dashboard
GET /api/projects/{id}/ledger/diagnostics
GET /api/projects/{id}/ledger/planner-context
```

Added write routes:

```text
POST  /api/projects/{id}/foreshadows
PATCH /api/projects/{id}/foreshadows/{foreshadow}
POST  /api/projects/{id}/secrets
PATCH /api/projects/{id}/secrets/{secret}
POST  /api/projects/{id}/secrets/{secret}/holders
POST  /api/projects/{id}/secrets/{secret}/holders/{holder}/close
```

All writes require `Idempotency-Key`, strict single-object JSON, the common one-MiB request limit, opaque project IDs, trace IDs, and the safe error envelope. Collections have bounded limits and explicit stable ordering.

### UI Changes

- Foreshadows: Chapter-N metrics, computed OVERDUE, filters, stable list, create, progress, resolve, and abandon.
- Secrets: management-only authority truth, Chapter-N holders, private creation, holder addition, and public reveal.
- All writes are disabled while pending and reload authoritative server state on completion.

### Tests Executed

Phase 6 deterministic tests cover:

- Foreshadow lifecycle and illegal transitions
- computed OVERDUE and `overdue_by_chapters`
- indexed overdue query plans, stable pagination and filtering
- Scenario E in repository and HTTP integration layers
- Secret holder Chapter-N visibility and public reveal
- prevention of unknown Secret truth leakage
- accepted-Final-only Ledger writes and replay conflicts
- concurrent idempotent creation
- strict JSON, pagination and chapter bounds
- REST/OpenAPI route and schema integration
- Foreshadows/Secrets Web empty, structured-error, write, pending and refresh behavior
- targeted Race execution including `internal/narrativeledger`
- existing Truth 100,000-fact index gate, ainovel-cli regression, Windows and Docker gates

### Test Results

- Earlier PR #23 run `33631668743` passed Go, Windows, Docker, Svelte checks, Vitest, Vite build and npm audit, but correctly failed because rebuilt `web/dist` was not committed.
- The delivery candidate commits the exact rebuilt assets and adds the direct Narrative Ledger race target and component tests.
- Final exact-head PR CI: pending.
- Squash merge: pending.
- Merge-triggered main CI: pending.

### Performance

- OVERDUE queries use project/status/payoff indexes and are checked through `EXPLAIN QUERY PLAN`.
- Planner Context and collections are bounded and explicitly ordered; no whole-book scan is used.
- Formal Context Compiler benchmarks remain Phase 7; release-scale benchmarks remain Phase 13.

### License Review

- Phase 6 adds no Go or npm dependency.
- Existing Apache-2.0 and fail-closed Go/npm dependency license gates remain enabled.
- No GPL/AGPL clean-room reference source, SQL, tests, components, or prompts were copied.
- The final PR diff contains no Phase-specific recovery generator, payload fragment, source snapshot, or self-modifying production workflow.

### Known Issues

- No known Narrative Ledger data-integrity, authority, knowledge-boundary, credential-exposure, OpenAPI, Windows, Docker, dependency-license, or build-drift blocker remains in the delivery candidate.
- Phase 6 is not formally complete until PR #23 exact-head CI, squash merge, and merged-main CI all succeed.
- Context token budgeting, FTS5 hybrid retrieval, deterministic context hashes, and Context Diagnostics remain Phase 7.

### Next Phase

`feature/phase-07-context-compiler`, recreated from the exact accepted Phase 6 main.

### Feature Branch

`feature/phase-06-narrative-ledger`

### Pull Request

PR #23 — `feat: add Narrative Ledger`

### Exact Resume Point

1. Require the exact final PR #23 head to pass Go, direct Narrative Ledger Race, Frontend, Windows and Docker jobs.
2. Squash merge PR #23 without bypassing checks.
3. Require the merge commit's `main` workflow to pass all four jobs.
4. Record immutable Phase 6 acceptance evidence in a minimal docs-only PR.
5. Recreate `feature/phase-07-context-compiler` from the accepted main and begin Phase 7.
