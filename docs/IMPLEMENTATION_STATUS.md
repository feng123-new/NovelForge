# NovelForge Implementation Status

Last updated: 2026-09-03

## Verified repository state

- Phase 2 project/API foundation was delivered by PR #7 and passed merge-triggered `main` CI.
- Phase 3 formal Web Workspace was delivered by PR #8 and passed merge-triggered `main` CI.
- Phase 4 Structured Truth Store was delivered by PR #13; PR CI `33585504193` and merged-main CI `33585658124` succeeded. Evidence correction PR #14 also passed.
- Phase 5 Librarian / Continuity / Quality Gate was delivered by PR #19; PR CI `33591270069` and merged-main CI `33591405462` succeeded. Acceptance record PR #20 also passed.
- Phase 6 Narrative Ledger production was delivered by PR #24 from exact head `6e3a7613ebf65ad90ac5bc7952c05c0251d5c934`.
- Phase 6 production PR CI `33636642108` succeeded; squash merge `ef3d144567caadcdf16af809c636b3858d9eb8a2` passed merged-main CI `33637529694`.
- Phase 6 acceptance record PR #25 exact-head CI `33638270944` succeeded; squash merge `dff822c14ef9a5eae7f37695fa755d00b875f047` was followed by successful main CI `33638643303`.
- Phase 6 acceptance hardening PR #26 exact head `b2944cd8c3d7977753c1ff20ca3cf48f41ceb76c` passed PR CI `33639657423`.
- PR #26 squash merge `12bc259e39df20664c1bf950cf95f8978572ff64` passed merge-triggered main CI `33639951628` across Go, Frontend, Windows and Docker.
- Phase 7 Context Compiler and Hybrid Retrieval was delivered by PR #28 from exact head `b5600f21cbb91180aab8faed75865734dc515739`; PR CI `33716621992` succeeded, squash merge `30a91deac2dbe1add0ba8353c05f89401a53b108` passed merge-triggered main CI `33716865357`.

## Phase status

| Phase | Status | Evidence |
| --- | --- | --- |
| 0 — upstream baseline | complete | ainovel-cli baseline retained with provenance. |
| 1 — compatibility | complete | Runtime isolation, migration, lifecycle, branding and cross-platform gates passed. |
| 2 — project/API foundation | complete | PR #7 merged; merged-main CI passed. |
| 3 — formal Web Workspace | complete | PR #8 merged; merged-main CI passed. |
| 4 — Structured Truth Store | complete | PR #13 merged; exact-head and merged-main CI passed. |
| 5 — Librarian / Continuity / Quality Gate | complete | PR #19 merged; exact-head and merged-main CI passed. |
| 6 — Narrative Ledger | complete | PR #24 production, PR #25 acceptance evidence and PR #26 hardening all merged and validated. |
| 7 — Context Compiler and Hybrid Retrieval | complete | PR #28 exact-head CI and merge-triggered `main` CI passed. |
| 8–13 | not started | Phase 8 begins from the accepted Phase 7 `main`. |

## Phase 6 — Narrative Ledger

### Completed

- Added project Migration 4 and the `internal/narrativeledger` production package.
- Added deterministic Foreshadow lifecycle validation with `planned`, `planted`, `progressing`, `resolved`, `abandoned` and `contradicted` states.
- Implemented `OVERDUE` as a Chapter-N computed view rather than a mutable stored state.
- Added stable bounded listing, project/status/payoff indexes, append-only audit events and an `EXPLAIN QUERY PLAN` regression gate.
- Added an independent Secret model with temporal holders, Chapter-N public state, source version, authority and Truth-compatible provenance.
- Prevented unknown-role views from receiving Secret truth; they receive metadata-only knowledge-boundary records.
- Integrated accepted Phase 5 Final candidates through a replay-safe `AcceptedFinalCommitter` boundary.
- Kept Writer, Librarian and retrieval components unable to mutate the Ledger directly.
- Added real Dashboard metrics, Diagnostics, Planner Context, REST routes, OpenAPI contracts, Foreshadows Web page and Secrets Web page.
- Preserved the existing Truth Store as world-fact authority; Narrative Ledger is an indexed temporal projection, not a second authority system.

### Database / migration changes

Project Migration 4 adds:

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

The existing migration runner still supplies checksums, pre-migration backup, transactionality, unknown-version rejection and rollback. Immutable Ledger audit tables are protected by update/delete triggers.

### API changes

Read routes:

```text
GET /api/projects/{id}/foreshadows
GET /api/projects/{id}/foreshadows/{foreshadow}
GET /api/projects/{id}/secrets
GET /api/projects/{id}/secrets/{secret}
GET /api/projects/{id}/ledger/dashboard
GET /api/projects/{id}/ledger/diagnostics
GET /api/projects/{id}/ledger/planner-context
```

Write routes:

```text
POST  /api/projects/{id}/foreshadows
PATCH /api/projects/{id}/foreshadows/{foreshadow}
POST  /api/projects/{id}/secrets
PATCH /api/projects/{id}/secrets/{secret}
POST  /api/projects/{id}/secrets/{secret}/holders
POST  /api/projects/{id}/secrets/{secret}/holders/{holder}/close
```

Writes require `Idempotency-Key`, strict single-object JSON, bounded request bodies, project ownership checks, trace IDs and the common safe error envelope. Collections use stable ordering and bounded pagination.

### UI changes

- Added real Foreshadows and Secrets routes.
- Added Chapter-N selection, computed OVERDUE badges, Dashboard metrics and Diagnostics.
- Added real create, progress, resolve, abandon, holder-add and public-reveal actions.
- Every write disables duplicate submission, shows pending/success/structured-error state and reloads authoritative server state.
- Secret management views distinguish authority truth from role knowledge.

### Tests and CI

Deterministic repository and HTTP coverage includes:

- Foreshadow lifecycle and illegal-transition rejection;
- computed OVERDUE and `overdue_by_chapters`;
- stable pagination and indexed overdue query planning;
- Secret Holder Chapter-N ranges and public reveal boundaries;
- safe truth omission for unauthorized role-bound context;
- accepted-Final-only Ledger mutation;
- Finalize replay without duplicate Ledger events;
- concurrent idempotent creation;
- strict JSON, pagination and chapter bounds;
- Scenario E at repository and HTTP boundaries;
- OpenAPI route/schema drift.

Acceptance hardening adds:

- direct `internal/narrativeledger` execution in the targeted Go race gate;
- `Foreshadows.test.ts` page tests for empty/error states, idempotent create, pending lifecycle actions and authoritative refresh;
- `Secrets.test.ts` page tests for empty/error states, private Secret creation, temporal holder writes and public reveal;
- a deterministic committed Web build including the Tailwind output induced by the new component tests;
- cross-layer architecture, Truth Store and Web documentation.

The final hardening workflow executed:

```text
gofmt drift check
go vet ./...
go test -buildvcs=false -count=1 ./...
go test -race including internal/narrativeledger
Truth Store 100,000-fact temporal index gate
OpenAPI route/schema validation
CGO_ENABLED=0 NovelForge build
module and dependency-license gates
lifecycle smoke and brand audit
npm ci
Svelte/TypeScript check
Vitest: 9 files, 25 tests
Vite production build
npm high-severity audit
frontend dependency/license inventory
committed Web build-drift check
Windows full tests and build
Docker build
```

### Acceptance evidence

| Delivery | Exact head | PR CI | Merge | Main CI |
| --- | --- | --- | --- | --- |
| Phase 6 production PR #24 | `6e3a7613ebf65ad90ac5bc7952c05c0251d5c934` | `33636642108` success | `ef3d144567caadcdf16af809c636b3858d9eb8a2` | `33637529694` success |
| Acceptance record PR #25 | `f8c0e695b5b37869c2830bf38f8f2874d1ec7000` | `33638270944` success | `dff822c14ef9a5eae7f37695fa755d00b875f047` | `33638643303` success |
| Acceptance hardening PR #26 | `b2944cd8c3d7977753c1ff20ca3cf48f41ceb76c` | `33639657423` success | `12bc259e39df20664c1bf950cf95f8978572ff64` | `33639951628` success |

Superseded PR #23 was closed without merge and is not acceptance evidence.

### Architecture decisions

- `OVERDUE` is always derived from the requested Chapter N and payoff boundary.
- Truth Store remains authoritative for world facts; Ledger records reuse provenance and authority vocabulary without overriding Truth.
- Unknown Secret boundaries contain no authority truth.
- Only the selected, continuity-safe Final candidate can produce Ledger mutations.
- Ledger finalization is idempotent and participates in the existing recoverable chapter saga.
- Phase 7 consumes the stable `NarrativeLedgerContextProvider`; Phase 6 itself does not perform token trimming.

### Performance

Foreshadow schedule and Secret holder queries are bounded and index-backed. Scenario E verifies that a Chapter 20 critical Foreshadow with payoff range 100–130 is computed as overdue by five chapters at Chapter 135 and remains mandatory in Planner Context. Context-build and hybrid-retrieval benchmarks remain Phase 7 scope.

### License review

Phase 6 and its hardening add no Go or npm dependency. Existing Apache-2.0 dependency inventories and fail-closed Go/npm license gates remained enabled and passed. No GPL/AGPL clean-room reference source, SQL, component, test or prompt was copied.

### Repository hygiene

The accepted `main` contains only the normal `.github/workflows/ci.yml`; it contains no Phase-specific recovery workflow, source generator, payload fragment, probe file or finalizer script. Temporary delivery helpers were not merged into production.

### Known issues

No Phase 6 data-integrity, temporal-boundary, OpenAPI, generated-asset, Windows, Docker, dependency-license or race-gate blocker remains. Context compilation, FTS5 hybrid retrieval and token budgeting are intentionally Phase 7 work.


## Phase 7 — Context Compiler and Hybrid Retrieval

### Completed

- Added `internal/contextcompiler` as the deterministic, read-only prompt-input boundary.
- Added five stable layers: Truth, Narrative, Recent, Historical Retrieval and Style, plus a separate System reservation.
- Added configurable default allocation `20/15/25/20/10` plus `10%` System and fail-closed validation that the complete budget totals 100%.
- Added mandatory retention for Current Chapter Plan, POV Character State, Critical World Rules, Critical Foreshadows, explicit Knowledge Boundary and required contract beats.
- Added deterministic overflow behavior: missing, future or unfit mandatory records return structured errors instead of being silently dropped.
- Added fixed Structured → Timeline → Foreshadow → Relation → Recent → FTS5 → optional Vector retrieval ordering.
- Added a `VectorRetriever` interface without an external vector-database dependency.
- Added project-scoped, Chapter-N bounded FTS5 retrieval through project Migration 5.
- Added read-only Truth Store and Narrative Ledger adapters, including authorized Secret knowledge and truth-free unknown boundaries.
- Added deterministic ordering, stable rendering, SHA-256 context identity, per-layer token diagnostics and explicit trim reasons.
- Integrated incrementally with the established `novel_context` tool while retaining its existing fields and regression behavior.

### Changed files

Production, test and documentation paths delivered by PR #28:

```text
.github/workflows/ci.yml
docs/ARCHITECTURE.md
docs/CONTEXT_COMPILER.md
docs/IMPLEMENTATION_STATUS.md
internal/contextcompiler/*.go
internal/project/context_migration.go
internal/tools/novel_context.go
internal/tools/novel_context_test.go
```

### Architecture decisions

- Truth Store remains authoritative; FTS5 and optional vector results are retrieval evidence and cannot override Truth.
- The compiler receives read-only providers and has no world-state, Ledger or Chapter Final write capability.
- All provider items are evaluated against the requested Chapter N before allocation.
- Mandatory safety constraints are selected before optional context and fail closed when they cannot fit.
- Stable layer, stage, priority, source-chapter, kind and ID tie-breaks prevent map, SQL-default or filesystem-order nondeterminism.
- The existing `novel_context` path is migrated through an adapter rather than being destructively replaced.

### Database / migration changes

Project Migration 5 adds:

```text
context_documents
context_documents_fts
idx_context_documents_project_chapter
context_documents_ai
context_documents_ad
context_documents_au
```

The FTS index is external-content based, project scoped and maintained through insert, update and delete synchronization triggers. Queries are bounded by project and `source_chapter <= Chapter N`. The existing migration runner continues to provide checksums, backup, transactionality and rollback behavior.

### API and UI changes

Phase 7 adds no state-changing HTTP route and no speculative UI control. Context diagnostics are exposed through the existing authoritative `novel_context` result as `_context_compiler`; no browser-held state is promoted to authority.

### Tests executed

The exact PR and merged-main workflows passed:

```text
gofmt drift check
go vet ./...
go test -buildvcs=false -count=1 ./...
targeted race including internal/contextcompiler
Truth Store 100,000-fact temporal index gate
OpenAPI route/schema validation
CGO_ENABLED=0 NovelForge build
module-lock and Go dependency-license gates
lifecycle smoke and brand audit
npm ci
Svelte/TypeScript check
Vitest
Vite production build
npm high-severity audit
frontend dependency-license inventory
committed Web build-drift check
Windows full tests and build
Docker build
```

Deterministic Phase 7 tests cover default and custom budget allocation, invalid-budget rejection, mandatory overflow, missing requirements, future mandatory failure, future optional trimming, stable order and context hash, exact retrieval stage order, duplicate removal, per-layer diagnostics, Truth and Ledger adapters, Secret knowledge boundaries, FTS5 project/chapter isolation, upsert/delete synchronization, query-plan index use, migration and legacy `novel_context` integration.

### Performance

`BenchmarkCompilerFiveLayers-4` on the delivery candidate recorded:

```text
1,626,468 ns/op
1,089,548 B/op
2,910 allocs/op
```

This is a Phase 7 diagnostic benchmark, not the final Phase 13 100/500/1000-chapter release benchmark.

### License review

Phase 7 adds no Go or npm dependency. The existing Apache-2.0 fail-closed Go and frontend dependency-license gates remained enabled and passed. No GPL/AGPL reference implementation, SQL, test, component or prompt was copied.

### Acceptance evidence

| Delivery | Exact head | PR CI | Merge | Main CI |
| --- | --- | --- | --- | --- |
| Phase 7 production PR #28 | `b5600f21cbb91180aab8faed75865734dc515739` | `33716621992` — success | `30a91deac2dbe1add0ba8353c05f89401a53b108` | `33716865357` — success |

### Repository hygiene

The production PR contains only normal source, tests, Migration 5, permanent CI hardening and documentation. No payload fragment, source generator, temporary workflow, self-modifying finalizer or Phase 8 implementation was merged into `main`.

### Known issues

No Phase 7 data-integrity, future-state leakage, deterministic-ordering, FTS5, race, Windows, Docker, generated-asset, OpenAPI or dependency-license blocker remains. The vector provider is intentionally optional and V1 has no external vector-database dependency.

## Next phase

```text
Phase 8 — Chapter Version and Human Edit Sync
feature/phase-08-chapter-versions
```

## Exact resume point

1. Fetch and prune after this acceptance record is merged and its merge-triggered `main` CI succeeds.
2. Verify the exact accepted `main` SHA and that no unrelated Pull Request requires reconciliation.
3. Create or fast-forward `feature/phase-08-chapter-versions` from that exact accepted `main`.
4. Run the full Go, Frontend, Windows and Docker baseline.
5. Implement immutable ChapterVersion history, diff, restore, active-final uniqueness, Human Revision synchronization and Chapter-N derived-state rebuild without reopening accepted Phase 7 scope.
