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
- Phase 7 production is implemented on `delivery/phase-07-context-compiler-clean`; formal completion remains gated on exact-head PR CI, squash merge, and merge-triggered `main` CI.

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
| 7 — Context Compiler and Hybrid Retrieval | in progress | Normal production source is on the clean delivery branch; remote acceptance is pending. |
| 8–13 | not started | Begin Phase 8 only after Phase 7 is merged and its `main` CI succeeds. |

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

## Next phase

```text
Phase 7 — Context Compiler and Hybrid Retrieval
delivery/phase-07-context-compiler-clean
```

## Exact resume point

1. Fetch and prune the repository after this evidence record is merged and its merge-triggered `main` CI succeeds.
2. Verify the exact current `main` SHA and that no unrelated open PR requires reconciliation.
3. Review `delivery/phase-07-context-compiler-clean` against that exact accepted `main`.
4. Require the exact production head to pass Go, Frontend, Windows, and Docker CI.
5. Squash merge only after all required jobs succeed, then verify the merge-triggered `main` workflow before recording Phase 7 as complete.

## Phase 7 — Context Compiler and Hybrid Retrieval

### Delivery candidate

- Five deterministic context layers: Truth, Narrative, Recent, Historical Retrieval, and Style.
- Configurable default token allocation 20/15/25/20/10 plus a separate 10% System reservation.
- Fail-closed retention for Current Chapter Plan, POV Character State, Critical World Rules, Critical Foreshadows, explicit Knowledge Boundary, and required contract beats.
- Fixed Structured → Timeline → Foreshadow → Relation → Recent → FTS5 → optional Vector retrieval sequence.
- Project-local Migration 5 with bounded, project-scoped, Chapter-N FTS5 retrieval.
- Stable context ordering, stable SHA-256, per-layer token diagnostics, and explicit trim reasons.
- Incremental `novel_context` adapter; existing fields and regression behavior are retained.
- Deterministic unit, migration, temporal-boundary, overflow, ordering, retention, FTS5, race, and benchmark coverage.

### Local validation evidence

- Go toolchain: `go1.25.5 linux/amd64`.
- Targeted tests: success.
- Targeted Race Detector: success.
- Full existing Go suite: success when run as an unprivileged user; the root-only run correctly exposed the pre-existing readonly-file test assumption.
- `CGO_ENABLED=0 go build -trimpath ./cmd/novelforge`: success.
- Benchmark: `BenchmarkCompilerFiveLayers-4`, 1,626,468 ns/op, 1,089,548 B/op, 2,910 allocs/op on AMD EPYC 9V74.

### Remote acceptance

The normal production source is on `delivery/phase-07-context-compiler-clean`. Do not mark Phase 7 complete until its exact-head Pull Request workflow, squash merge, and merge-triggered `main` workflow all succeed and their immutable IDs are recorded here.
