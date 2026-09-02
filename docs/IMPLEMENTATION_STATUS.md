# NovelForge Implementation Status

Last updated: 2026-09-02

## Repository state

- Phase 2 PR #7 merged; merged-main CI passed.
- Phase 3 PR #8 merged; merged-main CI passed.
- Phase 4 production and evidence correction are on accepted main `5cd80f9efe35e16e3db3302fca5cdd178e28c7b0`.
- Phase 4 production PR #13 CI `33585504193` succeeded; merge-triggered main CI `33585658124` succeeded.
- Phase 4 evidence PR #14 CI `33586026981` succeeded; resulting main CI `33586415570` succeeded.
- Phase 5 branch is `feature/phase-05-quality-gate`; PR #18 is still Draft while exact final-head acceptance is pending.

## Phase status

| Phase | Status | Notes |
| --- | --- | --- |
| 0 — upstream baseline | complete | ainovel-cli baseline retained with provenance. |
| 1 — compatibility | complete | Runtime isolation, migration, lifecycle, branding, Linux/Windows/Docker gates. |
| 2 — project/API foundation | complete | PR #7 merged and main CI passed. |
| 3 — formal Web Workspace | complete | PR #8 merged and main CI passed. |
| 4 — Structured Truth Store | complete | PR #13 plus merge-triggered main CI passed; evidence correction #14 also passed. |
| 5 — Librarian / Continuity / Quality Gate | implementation complete; acceptance pending | Production code, API, OpenAPI, Web, tests and docs are on PR #18; final CI/merge/main CI remain pending. |
| 6–13 | not started | Phase 6 begins only after Phase 5 merged-main acceptance. |

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

- Added stable Architect/Planner/Writer/Librarian/Continuity/Editor, structured-output, model-call, transaction repository, commit coordinator and quality-policy interfaces.
- Added strict structured-output decoding: unknown fields, multiple JSON values and trailing data fail; optional repair is bounded and revalidated.
- Added provenance-bound Fact Proposal groups for entity/character/relation/location/inventory/knowledge/timeline/world/foreshadow/secret/injury/cultivation changes.
- Added deterministic Chapter-N continuity checks against the Truth Store, including temporal inventory and knowledge boundaries.
- Added persistent quality state machine, candidates, reviews, model-call ledger and transition audit.
- Added default `max_rewrites=2`, threshold 7.0 and deterministic WARN policy.
- Added bounded candidate selection; Continuity FAIL can never Finalize and Editor score cannot override it.
- Added model-call idempotency: same key/hash replays; same key/different hash conflicts; transient retries are bounded.
- Added recoverable Final→Truth→chapter file→checkpoint saga with per-chapter Finalize serialization and replay-safe Truth keys.
- Added Windows-safe recoverable chapter replacement with same-directory backup.
- Added project migration 3, real REST/OpenAPI integration and real Chapters Web quality controls.

### Changed Files

- `internal/qualitygate/*.go`
- `internal/project/quality*.go`
- `internal/server/quality*.go`
- `internal/server/server.go`
- `internal/server/workspace.go`
- `internal/server/openapi.go`
- `internal/server/openapi_phase5.json`
- `internal/server/openapi_test.go`
- `.github/workflows/ci.yml`
- `web/src/lib/api.ts`
- `web/src/lib/api.test.ts`
- `web/src/lib/types.ts`
- `web/src/pages/Chapters.svelte`
- `docs/AGENTS.md`
- `docs/ENGINE.md`
- `docs/API.md`
- `docs/QUALITY_GATE.md`
- `docs/ARCHITECTURE.md`
- `docs/IMPLEMENTATION_STATUS.md`

### Architecture Decisions

- Writer and Librarian do not receive Truth repositories; only the deterministic commit coordinator converts the accepted proposal to Truth.
- Continuity uses authoritative Chapter-N Truth; retrieval/RAG is not an authority input.
- Full Draft text is durable but omitted from quality diagnostic API responses.
- Semantic model outputs are behind injected services/`ModelInvoker`; no API key is stored in project DB, Web state or normal logs.
- Default JSON repair is one attempt when a repairer is configured; provider retry defaults to two and is capped at five.
- A transaction becomes `completed` only after Truth, final chapter file and checkpoint are durable.

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

### API Changes

```text
POST /api/projects/{id}/chapters/{chapter}/generate
POST /api/projects/{id}/chapters/{chapter}/check
POST /api/projects/{id}/chapters/{chapter}/rewrite
POST /api/projects/{id}/chapters/{chapter}/finalize
GET  /api/projects/{id}/chapters/{chapter}/quality
GET  /api/projects/{id}/chapters/{chapter}/candidates
```

All writes use `Idempotency-Key`, the existing request-size bound, strict JSON, opaque project IDs, trace IDs and safe error envelopes. All routes are in the merged OpenAPI 3.1 document.

### UI Changes

The Chapters workspace now shows authoritative transaction state, Draft count, Librarian proposal status, Continuity PASS/WARN/FAIL, Editor score, rewrite attempt/max, Final candidate, HOLD reason and structured trace errors. Generate/Check/Rewrite/Finalize invoke real API routes, disable during pending work and reload server state after every action. No Phase 8 history/diff/restore UI is present.

### Tests Executed

The branch contains deterministic tests for:

- Writer/Librarian separation from Truth through interface boundaries
- Fact Proposal provenance/schema validation
- malformed JSON repair, unknown/trailing JSON rejection, repair cap
- Draft retention after Librarian failure and post-Draft crash injection
- Continuity FAIL blocking Finalize and Editor
- deterministic WARN handling
- Editor score persistence
- default/max rewrite bounds and no calls beyond the limit
- best safe candidate selection and HOLD when none exists
- model-call idempotent replay/content conflict
- deterministic fake timeout/429/5xx bounded retry and call counts
- Truth-commit failure retaining Final candidate
- Finalize replay and concurrent Finalize without duplicate Truth
- illegal state transition rejection
- Chapter-N Inventory and Knowledge temporal checks
- HTTP safe error envelope, strict JSON, idempotency and full Generate→Check→Finalize
- OpenAPI route/schema drift
- Web API idempotency, structured errors and authoritative refresh path

The repository CI additionally gates full Go tests, targeted race including `internal/qualitygate`, CGO-free build, Truth 100k index gate, dependency licenses, lifecycle/brand, frontend check/test/build/audit/license drift, Windows and Docker.

### Test Results

- Intermediate PR #18 runs proved earlier backend core and existing full CI gates, but they are not acceptance evidence because later commits superseded those heads.
- Exact final Phase 5 PR CI: pending.
- Squash merge: pending.
- Merge-triggered main CI: pending.

### Performance

- Phase 5 chapter continuity checks issue bounded indexed Chapter-N Truth queries per proposed fact; they do not scan the complete novel.
- The existing 100,000-fact Truth Store index gate remains blocking in CI.
- Formal Context Compiler benchmarks belong to Phase 7; release-scale benchmarks remain Phase 13 work.

### License Review

- Phase 5 adds no Go or npm dependency.
- Existing Apache-2.0 dependency inventories and fail-closed Go/npm license gates remain blocking.
- No GPL/AGPL clean-room reference source, SQL, components, tests or prompts were copied.

### Known Issues

- Acceptance evidence is pending the exact final PR head, squash merge and merge-triggered main CI.
- Narrative Ledger and Context Compiler are intentionally not included in this Phase 5 PR.

### Next Phase

`feature/phase-06-narrative-ledger` after Phase 5 merged-main acceptance.

### Feature Branch

`feature/phase-05-quality-gate`

### Final Head Commit

Pending exact final-head CI.

### Pull Request

#18 — `feat: add fact extraction and continuity gate`

### PR CI Result

Pending exact final-head full workflow.

### Main Merge Commit

Pending.

### Main CI Result

Pending.

### Exact Resume Point

1. Wait for the exact final PR #18 head to complete all four CI jobs.
2. Fix only evidence-based failures without weakening gates.
3. Mark PR ready, squash merge, and verify merge-triggered main CI.
4. Record immutable Phase 5 evidence and create `feature/phase-06-narrative-ledger` from accepted main.
