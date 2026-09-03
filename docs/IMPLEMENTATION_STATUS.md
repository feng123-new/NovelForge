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
- Phase 8 Chapter Version and Human Edit Sync was delivered by PR #30 from exact head `dfc0c44ede7bfc59b1b56bd23e88643006f3ac62`.
- Phase 8 production PR CI `33745966937` succeeded; squash merge `502cbf06611515484cb4bad2b2418b717e92f6b3` passed merge-triggered `main` CI `33746266244` across Go, Frontend, Windows and Docker.
- Phase 8 acceptance record PR #31 exact head `300e0e944069b3e902c19514503cdea5505c852c` passed CI `33747489810`; squash merge `8cc2759bd44d554b9c259531192fba120f38e5da` passed `main` CI `33747741718`.
- Phase 8 final closure PR #32 exact head `bb7a532bf00fad371b6eedb010ef388c8c324bfc` passed CI `33748179603`; squash merge `2718a0728321328e51cfc6773c5f8fd40178f908` passed final closure `main` CI `33748407175`.
- `feature/phase-09-autopilot` was created only after Phase 8 closure CI succeeded and contains no Phase 9 product implementation at handoff.

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
| 8 — Chapter Version and Human Edit Sync | complete | PR #30 exact-head CI `33745966937`, squash merge `502cbf06611515484cb4bad2b2418b717e92f6b3`, and merged-main CI `33746266244` passed; acceptance evidence is recorded separately. |
| 9–13 | not started | Phase 8 closure is complete; `feature/phase-09-autopilot` exists at the accepted handoff with no Phase 9 implementation yet. |

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

## Phase 8 — Chapter Version and Human Edit Sync

### Completed

- Added project Migration 6 and `internal/chapterversion` as the immutable chapter-revision authority boundary.
- Added monotonic per-chapter version numbers, parent lineage, append-only review/audit events, one Active Final projection, idempotent operations, external-SHA state, recoverable Finalize sagas, rebuild records, checkpoints and downstream plan impacts.
- Added Human Revision save, Check, Accept, Reject, Restore-as-new-version, Finalize and explicit external-file Sync without silent overwrite of Active Final.
- Added Human Final authority above generated final, explicit Truth supersede/assert history, Narrative Ledger Human Final promotion and Chapter-N derived-state rebuild.
- Added bounded deterministic inline/side-by-side Diff with cursor pagination and hard byte/line/work ceilings.
- Added the real Versions Web workspace and a Phase 8 OpenAPI extension merged into `/api/openapi.json`.
- Fixed SQLite single-connection cursor/write deadlocks by materializing rows before nested decoration or FTS writes.
- Hardened Finalize recovery so an Active Final alone cannot skip unfinished rebuild/checkpoint work; completed replay requires persisted saga/checkpoint evidence.

### Migration 6

Migration `chapter_versions_human_sync` adds immutable `chapter_versions`, `chapter_version_events`, `chapter_active_finals`, `chapter_revision_operations`, `chapter_external_state`, `derived_state_rebuilds`, `chapter_plan_impacts`, `chapter_finalize_sagas`, `chapter_version_checkpoints`, and the per-chapter version counter migration. Immutable history is protected by database triggers; the Active Final is a separate mutable projection.

### Authority and synchronization decisions

- Browser/editor save creates `human_revision`; it is not a Truth write.
- External files are compared by normalized SHA against the immutable Active Final and require explicit sync on mismatch.
- Librarian, Continuity and Editor may evaluate a revision but cannot promote authority.
- Accept records approval without committing Truth.
- Finalize alone may create/switch a Final and commit the accepted proposal.
- Accepted Human Final is the highest authority and cannot be silently downgraded by generated output.
- Human corrections append explicit supersede/assert Truth events; previous generated events remain auditable.
- Rebuild begins at edited Chapter N; Chapter N-1 and earlier remain unchanged.
- Downstream changed assumptions are recorded as plan impacts rather than silently rewriting historical plans.

### Web and API

Phase 8 adds authoritative chapter-version state, list/get/create, Diff, Check, Restore, Accept, Reject, Finalize, Sync Status, Sync, Rebuild and Plan Impact routes. All writes use `Idempotency-Key` and the common safe error envelope. Web History, Markdown editor/reader, Inspector, external edit warning, plan impact and Diff controls reload server authority after writes. No new frontend dependency was added.

### Scenario B and recovery tests

Permanent Scenario B verifies a Chapter 50 correction from generated death to severe injury, escape and survival: SHA mismatch is detected, sync creates a human revision without replacing the old Final, the generated death remains in Truth history, Human Final explicitly supersedes it, Chapter 49 remains unchanged, Chapter 50+ projects the correction, Chapter 51 plan impacts are emitted, file SHA converges to the Active Human Final and repeated operations remain idempotent.

The Finalize recovery matrix injects one failure after version creation, Truth commit, Ledger commit, chapter-file write, Active Final switch, before checkpoint and after checkpoint. Same-key retry must converge to one completed Human Final, one Active Final and one checkpoint without duplicate authoritative Truth.

### Acceptance evidence

| Delivery | Exact head | PR CI | Merge | Main CI |
| --- | --- | --- | --- | --- |
| Phase 8 production PR #30 | `dfc0c44ede7bfc59b1b56bd23e88643006f3ac62` | `33745966937` — success | `502cbf06611515484cb4bad2b2418b717e92f6b3` | `33746266244` — success |
| Phase 8 acceptance record PR #31 | `300e0e944069b3e902c19514503cdea5505c852c` | `33747489810` — success | `8cc2759bd44d554b9c259531192fba120f38e5da` | `33747741718` — success |
| Phase 8 final closure PR #32 | `bb7a532bf00fad371b6eedb010ef388c8c324bfc` | `33748179603` — success | `2718a0728321328e51cfc6773c5f8fd40178f908` | `33748407175` — success |

Full evidence and the acceptance rule are in `docs/PHASE_08_ACCEPTANCE.md`.

### Repository hygiene

PR #30 was the single Phase 8 production PR. The temporary workflow used to recover exact CI-generated Web assets was deleted before the final green production head and was not merged. The accepted production tree contains no Phase-specific recovery workflow, payload fragment, probe, source generator or self-modifying finalizer.

### Known issues

No Phase 8 data-integrity, idempotency, crash-recovery, Scenario B, OpenAPI, Web build-drift, race, Windows, Docker, dependency-license, lifecycle or generated-asset blocker remains in the production merge. Phase 9 durable Autopilot work has not started.

## Next phase

```text
Phase 9 — Durable Autopilot
feature/phase-09-autopilot
```

## Exact resume point

1. Use the existing `feature/phase-09-autopilot` branch for Phase 9 product work.
2. Before the first Phase 9 change, verify that branch matches the current accepted `main`; fast-forward it across any later documentation-only closure commit.
3. Preserve Phase 8 Scenario B, crash-recovery, race, OpenAPI, Windows, Docker, dependency-license and generated-asset gates throughout Phase 9.
4. Treat Phase 8 as closed except for an independently reviewed regression fix with its own exact-head and merged-main CI evidence.
5. Do not mix Phase 8 acceptance-history edits with Phase 9 product implementation.
