# NovelForge Implementation Status

Last updated: 2026-09-02

## Repository state

- Phase 2 PR #7 merged; merged-main CI passed.
- Phase 3 PR #8 merged; merged-main CI passed.
- Phase 4 production and evidence correction are on accepted main `5cd80f9efe35e16e3db3302fca5cdd178e28c7b0`.
- Phase 4 production PR #13 CI `33585504193` succeeded; merge-triggered main CI `33585658124` succeeded.
- Phase 4 evidence PR #14 CI `33586026981` succeeded; resulting main CI `33586415570` succeeded.
- Phase 5 production was squash-merged from PR #19 as `831cb2983ce851063ce9fb650eaebb14f6ad44c1`.
- Phase 5 exact-head PR CI `33591270069` and merge-triggered main CI `33591405462` both succeeded.
- Draft PR #18 was closed without merge and superseded by the normal non-draft delivery PR #19 on the same final production head.
- Phase 5 acceptance record PR #20 CI `33591592779` succeeded; its squash merge `0934fc98bcedd7aa7a33baa84984e5cdef0196ed` passed main CI `33591712938`.

## Phase status

| Phase | Status | Notes |
| --- | --- | --- |
| 0 — upstream baseline | complete | ainovel-cli baseline retained with provenance. |
| 1 — compatibility | complete | Runtime isolation, migration, lifecycle, branding, Linux/Windows/Docker gates. |
| 2 — project/API foundation | complete | PR #7 merged and main CI passed. |
| 3 — formal Web Workspace | complete | PR #8 merged and main CI passed. |
| 4 — Structured Truth Store | complete | PR #13 plus merge-triggered main CI passed; evidence correction #14 also passed. |
| 5 — Librarian / Continuity / Quality Gate | complete | Production code, API, OpenAPI, Web, tests and docs merged in PR #19; PR and merged-main CI passed. |
| 6–13 | not started | Phase 6 begins from the latest accepted main; no Phase 6 production work is included in Phase 5. |

## Phase 4 evidence

- Final feature head: `078c18aa69929469d219dfd00f5e47aa2c348d86`
- PR: #13
- PR CI: `33585504193` — success
- Main merge: `903a163cb84385155783e52161e70233a15e8dc7`
- Main CI: `33585658124` — success
- Evidence correction main: `5cd80f9efe35e16e3db3302fca5cdd178e28c7b0`
- Evidence correction main CI: `33586415570` — success

## Phase 5

### Completed

- Added stable Architect, Planner, Writer, Librarian, Continuity, Editor, structured-output, model-call, transaction-repository, commit-coordinator and quality-policy interfaces.
- Added strict structured-output decoding: unknown fields, multiple JSON values and trailing data fail; optional repair is bounded and every repaired response is revalidated.
- Added provenance-bound Fact Proposal groups for entity, character, relationship, location, inventory, knowledge, timeline, world, foreshadow, secret, injury and cultivation changes.
- Kept Librarian proposals separate from authoritative Truth until a deterministic coordinator accepts a Final candidate.
- Added deterministic Chapter-N continuity checks against the temporal Truth Store, including inventory and knowledge boundaries.
- Added a persistent chapter-quality state machine, candidate history, proposal history, continuity results, Editor reviews, model-call ledger and transition audit.
- Added default `max_rewrites=2`, configurable quality threshold 7.0 and deterministic WARN policy.
- Added bounded candidate selection; Continuity FAIL can never become Final and an Editor score cannot override a blocking continuity result.
- Added model-call idempotency: same key and request hash replay; same key with different content conflicts; transient provider retry is bounded.
- Added a recoverable Final → Truth → chapter file → checkpoint saga with per-chapter Finalize serialization and replay-safe Truth keys.
- Added Windows-safe recoverable chapter replacement with a same-directory backup.
- Added project migration 3, real REST/OpenAPI integration and real Chapters Web quality controls.
- Preserved the existing TUI, headless, host, Store, checkpoint, revision and mature `commit_chapter` recovery paths.

### Changed Files

Major production and acceptance paths:

- `.github/workflows/ci.yml`
- `internal/qualitygate/*.go`
- `internal/project/quality.go`
- `internal/project/quality_migration.go`
- `internal/server/quality.go`
- `internal/server/quality_test.go`
- `internal/server/server.go`
- `internal/server/workspace.go`
- `internal/server/openapi.go`
- `internal/server/openapi_phase5.json`
- `internal/server/openapi_test.go`
- `web/src/lib/api.ts`
- `web/src/lib/api.test.ts`
- `web/src/lib/types.ts`
- `web/src/pages/Chapters.svelte`
- `web/dist/assets/app.css`
- `web/dist/assets/app.js`
- `docs/AGENTS.md`
- `docs/ENGINE.md`
- `docs/API.md`
- `docs/QUALITY_GATE.md`
- `docs/ARCHITECTURE.md`
- `docs/IMPLEMENTATION_STATUS.md`

### Architecture Decisions

- Writer and Librarian do not receive Truth repositories; only the deterministic commit coordinator converts the accepted proposal into Truth events.
- Continuity uses authoritative Chapter-N Truth; retrieval or RAG is not an authority input.
- Full Draft text is durable but omitted from quality diagnostic API responses.
- Semantic model outputs are behind injected services and `ModelInvoker`; no API key is stored in the project database, Web state or normal logs.
- JSON repair defaults to one attempt when a repairer is configured; provider retry defaults to two and is capped at five.
- The quality loop is finite. Default `max_rewrites` is two and no Agent can create an unbounded self-loop.
- A transaction becomes `completed` only after Truth events, the final chapter file and checkpoint are durable.
- The cross-database/filesystem boundary is a recoverable saga with persisted step state and idempotent re-entry rather than an unsafe best-effort sequence.

### Database / Migration Changes

Project migration 3 `chapter_quality_gate` adds:

```text
chapter_transactions
chapter_state_changes
chapter_candidates
fact_proposals
continuity_results
editor_reviews
model_calls
chapter_truth_commits
chapter_checkpoints
```

The existing migration runner continues to provide checksums, pre-migration backup, transactional application, unknown-version rejection and restore on failure.

### API Changes

Added:

```text
POST /api/projects/{id}/chapters/{chapter}/generate
POST /api/projects/{id}/chapters/{chapter}/check
POST /api/projects/{id}/chapters/{chapter}/rewrite
POST /api/projects/{id}/chapters/{chapter}/finalize
GET  /api/projects/{id}/chapters/{chapter}/quality
GET  /api/projects/{id}/chapters/{chapter}/candidates
```

All writes require `Idempotency-Key` and use the existing one-MiB request bound, strict JSON decoding, opaque project IDs, trace IDs and safe error envelope. All routes and schemas are included in the composed OpenAPI 3.1 document and route-drift tests.

### UI Changes

The Chapters workspace now shows authoritative transaction state, Draft count, Librarian proposal status, Continuity PASS/WARN/FAIL, Editor score, rewrite attempt/max, Final candidate, HOLD reason and structured trace errors. Generate, Check, Rewrite and Finalize invoke real API routes, disable during pending work and reload authoritative server state after every action. Phase 8 history, diff, restore and human-revision controls were not added prematurely.

### Tests Executed

Deterministic Phase 5 tests cover:

- Writer and Librarian separation from Truth through interface boundaries
- Fact Proposal provenance and schema validation
- malformed JSON repair, unknown/trailing JSON rejection and repair cap
- Draft retention after Librarian failure and post-Draft crash injection
- Continuity FAIL blocking Editor override and Finalize
- deterministic WARN handling
- Editor score persistence
- default and maximum rewrite bounds with no model calls beyond the cap
- highest safe candidate selection, recorded reason and HOLD when no safe candidate exists
- model-call idempotent replay and content conflict
- deterministic fake timeout, 429 and 5xx bounded retry with call counts
- Truth-commit failure retaining the Final candidate
- Finalize replay and concurrent Finalize without duplicate Truth
- illegal state-transition rejection
- Chapter-N inventory and knowledge temporal checks
- HTTP safe errors, strict JSON, idempotency and full Generate → Check → Finalize integration
- OpenAPI route and schema drift
- Web API idempotency, structured errors and authoritative refresh behavior

The exact final PR and merged-main workflows also executed:

```text
gofmt drift check
GOWORK=off go vet ./...
GOWORK=off go test -buildvcs=false -count=1 ./...
targeted race tests including internal/qualitygate
Truth Store 100,000-fact temporal index gate
OpenAPI route/schema validation
CGO_ENABLED=0 NovelForge build
Go dependency and license inventory
module-lock drift check
shell syntax validation
install/upgrade/uninstall lifecycle smoke
brand audit
npm ci
Svelte/TypeScript checks
Vitest
production Web build
npm audit --audit-level=high
frontend dependency and license inventory
committed Web build-drift check
Windows full tests and build
Docker build
```

### Test Results

- Final feature head: `9fe20781383c04ae39c5816a43fbe037a81dd82e`.
- Final non-draft delivery PR: #19 — `feat: add fact extraction and continuity gate`.
- PR CI run `33591270069`: **success** on the exact final head; Go, Frontend, Windows and Docker all succeeded.
- Squash merge: **success**, main commit `831cb2983ce851063ce9fb650eaebb14f6ad44c1`.
- Merge-triggered main CI run `33591405462`: **success** on the exact merge commit; Go, Frontend, Windows and Docker all succeeded.
- Draft PR #18 was closed without merge after the connector could not transition its draft flag; it is not used as acceptance evidence.
- Acceptance record PR #20 CI run `33591592779`: **success**; squash merge `0934fc98bcedd7aa7a33baa84984e5cdef0196ed` and resulting main CI run `33591712938`: **success**.

### Performance

- Phase 5 continuity checks issue bounded, indexed Chapter-N Truth queries per proposed fact and do not scan the complete novel.
- The existing 100,000-fact Truth Store index gate remains blocking and passed on the exact PR head and merged main.
- Model retry, JSON repair and rewrite loops have explicit caps.
- Formal Context Compiler benchmarks belong to Phase 7; release-scale benchmarks remain Phase 13 work.

### License Review

- Phase 5 adds no Go or npm dependency.
- Existing Apache-2.0 dependency inventories and fail-closed Go/npm license gates remained enabled and passed.
- No GPL/AGPL clean-room reference source, SQL, components, tests or prompts were copied.
- The final production diff contains no Phase-specific recovery generator, payload fragment or self-modifying workflow.

### Known Issues

- No Phase 5 data-integrity, continuity-gate, credential-exposure, OpenAPI, Windows, Docker, dependency-license or build-drift blocker remains.
- Narrative Ledger and Context Compiler are intentionally outside Phase 5 and remain Phase 6 and Phase 7 work.
- The connected GitHub GraphQL wrapper could not mark draft PR #18 ready because of an upstream response-schema incompatibility; the exact validated head was therefore delivered through normal non-draft PR #19 without changing production content.

### Next Phase

`feature/phase-06-narrative-ledger`

### Feature Branch

`feature/phase-05-quality-gate`

### Final Head Commit

`9fe20781383c04ae39c5816a43fbe037a81dd82e`

### Pull Request

[#19 — feat: add fact extraction and continuity gate](https://github.com/feng123-new/NovelForge/pull/19)

### PR CI Result

`33591270069` — success

### Main Merge Commit

`831cb2983ce851063ce9fb650eaebb14f6ad44c1`

### Main CI Result

`33591405462` — success

### Exact Resume Point

1. Fetch `origin/main` and verify the latest workflow on that exact commit is successful.
2. Create `feature/phase-06-narrative-ledger` from the exact fetched main SHA.
3. Implement Narrative Ledger without reopening or expanding the accepted Phase 5 scope.
