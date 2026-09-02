# NovelForge Implementation Status

Last updated: 2026-09-02

## Repository state

- Phase 2 PR [#7](https://github.com/feng123-new/NovelForge/pull/7) merged; merged-main CI passed.
- Phase 3 PR [#8](https://github.com/feng123-new/NovelForge/pull/8) merged; merged-main CI passed.
- Phase 4 was recovered from a previously validated clean implementation and landed from the actual pre-Phase-4 main `617fc20a96b307b0656a6267e3657ea9ff9f5147` without recovery payloads, generators, source fragments, self-modifying workflows, or direct pushes to main.
- Phase 4 production main: `903a163cb84385155783e52161e70233a15e8dc7`.

## Phase status

| Phase | Status | Notes |
| --- | --- | --- |
| 0 — upstream baseline | complete | ainovel-cli baseline retained with provenance. |
| 1 — compatibility | complete | Runtime isolation, migration, lifecycle, branding, Linux/Windows/Docker gates. |
| 2 — project/API foundation | complete | PR #7 merged and main CI passed. |
| 3 — formal Web Workspace | complete | PR #8 merged and main CI passed. |
| 4 — Structured Truth Store | complete | PR #13 and merge-triggered main CI passed. |
| 5–13 | not started | Phase 5 begins from the accepted Phase 4 main. |

## Phase 4

### Completed

- Project-local immutable `truth_events` with deterministic request hashes, checksums, provenance, UTC timestamps, authority, valid-time and knowledge-time bounds.
- Rebuildable `truth_facts`, `truth_conflicts`, and `truth_projection_meta` projections.
- Explicit `assert`, `supersede`, and `retract` event kinds with deterministic authority ordering.
- Chapter-N temporal state queries, paginated event/conflict history, bounded batch queries, conflict history, boundary rebuild and integrity verification.
- Truth API, OpenAPI 3.1 schemas, route drift tests, project adapter and safe migration integration.
- Append-only SQLite triggers and indexed 100,000-fact temporal query gate.

### Changed Files

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

### Architecture Decisions

- Truth is immutable typed events plus rebuildable projections, not mutable last-write-wins rows.
- LLM output is never authoritative merely because a model emitted it.
- Authority and Chapter-N visibility are deterministic Go policy.
- Valid time and knowledge time are independent to prevent future-state and future-knowledge leakage.
- Batch queries use one SQLite statement and normal chapter paths avoid O(n²) full-book scans.
- SQLite remains behind a repository boundary and uses the existing CGo-free driver.

### Database / Migration Changes

Migration 2 `structured_truth_store` adds:

```text
truth_events
truth_facts
truth_conflicts
truth_projection_meta
```

The existing migration runner provides checksum verification, pre-migration backup, transactional application and restore on failure.

### API Changes

```text
GET  /api/truth/events
POST /api/truth/events
GET  /api/truth/state
POST /api/truth/state:batch
GET  /api/truth/conflicts
POST /api/truth/rebuild
GET  /api/truth/verify
```

Write APIs require `Idempotency-Key`; all requests use bounded input, stable collection semantics and the common safe error envelope.

### UI Changes

No Phase 4 mutation UI was added. Browser-authoritative Truth and fake controls remain prohibited; the Phase 5 quality transaction will connect accepted Final candidates to Truth.

### Tests Executed

The exact PR head and merged main executed the repository CI gates, including:

```text
gofmt drift check
GOWORK=off go vet ./...
GOWORK=off go test -buildvcs=false -count=1 ./...
targeted race tests
Truth Store 100k temporal index gate
OpenAPI route/schema validation
CGO_ENABLED=0 NovelForge build
Go dependency/license inventory and policy gate
project migration tests
shell syntax and lifecycle smoke
install/upgrade/uninstall smoke
brand audit
npm ci/check/test/build/audit/license inventory
frontend build drift
Windows full tests/build
Docker build
```

### Test Results

- PR #13 CI run `33585504193`: **success** on exact head `078c18aa69929469d219dfd00f5e47aa2c348d86`.
- Squash merge: **success**, main commit `903a163cb84385155783e52161e70233a15e8dc7`.
- Merge-triggered main CI run `33585658124`: **success** on exact main commit `903a163cb84385155783e52161e70233a15e8dc7`.

### Performance

- Permanent CI exercises 100,000 projected facts.
- The gate verifies the Chapter-N bounded key query uses `idx_truth_facts_asof`.
- Bounded batch selectors are compiled into one SQL statement.

### License Review

- No new Go or npm dependency was introduced by Phase 4.
- Apache-2.0 distribution policy and fail-closed dependency license gates remain intact.
- No GPL/AGPL clean-room reference source, SQL, test or UI code was copied.

### Known Issues

- No Phase 4 data-integrity, credential-exposure, Windows, Docker, OpenAPI or license blocker remains.
- Librarian proposals, Continuity, Editor, bounded rewrite and Final-to-Truth commit belong to Phase 5.

### Next Phase

`feature/phase-05-quality-gate`

### Feature Branch

`fix/phase-04-truth-store-landing`

### Final Head Commit

`078c18aa69929469d219dfd00f5e47aa2c348d86`

### Pull Request

[#13 — feat: add structured truth store](https://github.com/feng123-new/NovelForge/pull/13)

### PR CI Result

`33585504193` — success

### Main Merge Commit

`903a163cb84385155783e52161e70233a15e8dc7`

### Main CI Result

`33585658124` — success

### Exact Resume Point

Create `feature/phase-05-quality-gate` from the exact accepted main after this evidence-only status correction is merged and its own main CI is green.
