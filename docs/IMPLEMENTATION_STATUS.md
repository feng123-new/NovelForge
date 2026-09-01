# NovelForge Implementation Status

Last updated: 2026-09-01

## Repository state

- Verified delivery base: `main@87db4c899187e23b0d08212e50a333d73f20e12a`.
- Phase 2 pull request: [#7 — feat: complete project and API foundation](https://github.com/feng123-new/NovelForge/pull/7) — merged.
- Phase 2 merged-main CI: [run 33462929571](https://github.com/feng123-new/NovelForge/actions/runs/33462929571) — Go, Windows and Docker jobs passed.
- Current delivery branch: `feature/phase-03-web-workspace`.
- Phase 3 implementation head before this status update: `839ebc88d6645602b6252a2ce13b897181261266`.
- Phase 3 pull request: not created at this document revision.
- Phase 3 CI: not run at this document revision.
- Exact next phase after Phase 3 acceptance: Phase 4 — Structured Truth Store.

## Phase status

| Phase | Status | Notes |
| --- | --- | --- |
| 0 — upstream baseline | complete | Imported ainovel-cli baseline and retained provenance. |
| 1 — compatibility | complete | Dual-path runtime, doctor, safe migration, lifecycle smoke, branding and cross-platform gates passed. |
| 2 — project/API foundation | complete | PR #7 merged at `87db4c8`; the merge-triggered main CI passed. |
| 3 — formal Web Workspace | implementation in progress | Backend-backed pages, Svelte source, tests, OpenAPI, CI and documentation are implemented; the exact npm lock, generated build and frontend license inventory still require PR CI acceptance. |
| 4–13 | not started | Must begin only after Phase 3 PR and merged-main CI pass. |

## Completed

### Phase 2 acceptance

- Confirmed the writable Project Repository, SQLite migration runner, idempotency store, durable events, safe error envelope, Engine adapter and OpenAPI contracts were merged.
- Confirmed the merge-triggered `main` workflow passed every Go, Windows and Docker job before starting Phase 3.

### Reproducible frontend source

- Added Svelte 5, Vite 6, strict TypeScript, Tailwind CSS, DaisyUI, Vitest, jsdom and Testing Library source under `web/`.
- Added deterministic production asset names and a self-hosted build compatible with the existing restrictive CSP.
- Added light and dark themes, responsive navigation, central workspace, AI Inspector and Task / Log regions.
- Kept the single-binary `go:embed` deployment model.
- Made browser theme preference the only browser-persisted setting; project and task authority remains on the server.

### Real Web pages

- Added a Dashboard backed by health and project APIs.
- Added Projects search, archive, unarchive, duplicate and reversible workspace-trash deletion.
- Added a six-step New Novel wizard for Basic, Idea, Style, Model Profile, Automation and Foundation confirmation.
- Added a real chapter collection page with project selection.
- Added Models backed by the Go runtime registry.
- Added Logs backed by durable/replayed SSE events.
- Added secret-free Settings backed by server capability data.
- Omitted fake chapter editing, Truth, version and Autopilot controls until their production APIs exist.

### Backend workspace support

- Added stable, bounded chapter metadata listing with `limit <= 100`.
- Skips chapter symlinks and reads at most 1 MiB per chapter during metadata inspection.
- Added validated Foundation request persistence at `.novelforge/foundation-request.json`.
- Rejects credential-looking request fields and values before persistence.
- Records `worker_available=false` so a stored request cannot be mistaken for a running worker.
- Added `GET /api/models` and safe `GET /api/settings`.
- Added GET/POST Foundation routes; the write reuses Phase 2 idempotency and publishes `foundation.requested` durably.

### Frontend state and tests

- Centralized all HTTP access in a typed API Client; components do not contain direct `fetch` calls.
- Added automatic `Idempotency-Key` generation for writes and preservation of structured error envelopes.
- Added hash routing with unknown-route fallback.
- Added one shared EventSource with connecting, connected, reconnecting and unavailable states.
- Added unit tests for the API client, route parsing, theme selection, SSE reconnect, wizard validation and request construction.
- Added component tests for Dashboard empty/error states and Projects archive/reversible-delete interactions.

### Build and license gates

- Added a dedicated Frontend GitHub Actions job for lock resolution, exact install, Svelte/TypeScript checks, tests and production build.
- CI rejects uncommitted lock/build drift.
- Added an npm dependency inventory generator that traverses top-level, scoped and nested installed packages.
- The frontend license policy fails closed for unknown metadata and rejects GPL, AGPL, LGPL, SSPL, BUSL and Commons Clause identifiers.
- CI uploads the exact lockfile, production build and generated inventory so the first networked run can be committed verbatim.

## Changed files

Major Phase 3 paths:

- `.github/workflows/ci.yml`
- `.gitignore`
- `README.md`
- `internal/project/chapters.go`
- `internal/project/foundation.go`
- `internal/project/workspace_test.go`
- `internal/server/workspace.go`
- `internal/server/workspace_test.go`
- `internal/server/server.go`
- `internal/server/openapi.json`
- `internal/server/openapi_test.go`
- `scripts/frontend_dependency_license_inventory.mjs`
- `web/package.json`
- `web/package-lock.json` after CI resolution
- `web/vite.config.ts`
- `web/svelte.config.js`
- `web/tsconfig.json`
- `web/postcss.config.cjs`
- `web/tailwind.config.cjs`
- `web/index.html`
- `web/src/**/*`
- `web/dist/**/*` after CI build
- `docs/WEB.md`
- `docs/LICENSES.md`
- `docs/FRONTEND_DEPENDENCY_LICENSES.md` after CI generation
- `docs/IMPLEMENTATION_STATUS.md`

## Architecture decisions

- The Web UI is a client of the same REST/SSE service and does not duplicate Engine logic.
- Components use one API Client so idempotency, error handling and same-origin behavior cannot drift per page.
- A Foundation request is persisted as a request snapshot, not represented as a completed generation job.
- The Phase 9 durable worker will consume the same project/engine/event boundaries rather than replacing Phase 3 routes.
- Chapter collection metadata is bounded and deterministic; full content and version/diff behavior are deferred to Phase 8.
- Models expose public runtime metadata only. Provider configuration and credentials remain outside the Web contract.
- `web/package-lock.json`, `web/dist` and both dependency inventories are release inputs and are checked for drift.

## Database / migration changes

- No new database migration is added in Phase 3.
- Foundation request persistence uses an atomic owner-only project file.
- Existing Phase 2 `server.db`, event and idempotency tables remain authoritative and unchanged.

## API changes

Added:

```text
GET  /api/models
GET  /api/settings
GET  /api/projects/{id}/chapters
GET  /api/projects/{id}/foundation
POST /api/projects/{id}/foundation
```

The Foundation write requires `Idempotency-Key`, stores a validated request, emits a durable event and returns HTTP 202. Every new route is represented in OpenAPI 3.1 and route-drift tests.

## UI changes

Added real pages for Dashboard, Projects, New Novel, Chapters, Models, Logs and Settings. Each asynchronous page includes loading, empty and/or error states appropriate to its data. Project writes show pending and success/error feedback and refresh authoritative server state after completion.

## Tests executed

Pre-PR source validation completed for the implementation sources:

```text
Go formatting over changed Go files: PASS
OpenAPI JSON parse: PASS
OpenAPI operation uniqueness and write-header inspection: PASS
TypeScript source syntax check with the available local compiler: PASS
Direct-fetch audit outside the API client: PASS
Browser-storage audit: theme preference only
```

The exact dependency install, Svelte check, frontend tests, production build, full Go suite, race tests, Windows build, Docker build and license inventories must pass in GitHub Actions before merge.

## Test results

No Phase 3 GitHub Actions result is recorded yet. Phase 3 remains in progress until the PR workflow and the merge-triggered `main` workflow both pass.

## Performance

No Phase 3 performance claim is recorded. Page collections use `limit <= 100`; chapter metadata reads are capped at 1 MiB per file. Formal browser, API and long-book benchmarks remain a Phase 13 deliverable.

## License review

- No source was copied from the MIT product reference.
- No source, component, SQL, test or prompt was copied from GPL/AGPL clean-room references.
- All frontend dependencies are build/test tooling or bundled permissive runtime code and must appear in the exact generated inventory before merge.
- Any unknown or prohibited license result remains blocking; the scanner has no broad package-name override.

## Known issues

- `web/package-lock.json` has not yet been resolved by a networked npm environment.
- The exact Phase 3 `web/dist` build and frontend dependency inventory have not yet been committed.
- The first Frontend CI run is expected to upload those generated artifacts and fail the drift gate until they are committed exactly.
- Phase 3 is not complete until all PR jobs pass, the PR is merged, and merged-main CI succeeds.
- Truth Store, Quality Gate, Narrative Ledger, Context Compiler, Chapter Versions, Durable Autopilot, Skills/Reference, Lifecycle, Observability and Release work remain Phase 4–13.

## Next phase

After Phase 3 merged-main acceptance, create `feature/phase-04-truth-store` from that exact verified `main` commit and implement the versioned temporal Truth Store, event projections, Chapter-N queries, conflicts, provenance and rebuild boundaries.

## Delivery evidence

- Base main: `87db4c899187e23b0d08212e50a333d73f20e12a`.
- Phase 2 PR: [#7](https://github.com/feng123-new/NovelForge/pull/7) — merged.
- Phase 2 main CI: [33462929571](https://github.com/feng123-new/NovelForge/actions/runs/33462929571) — success.
- Phase 3 branch: `feature/phase-03-web-workspace`.
- Phase 3 PR: pending.
- Phase 3 CI: pending.
- Phase 3 main merge and CI: pending.

## Exact resume point

1. Create the Phase 3 pull request from `feature/phase-03-web-workspace` to `main`.
2. Inspect every Go, Frontend, Windows and Docker job.
3. Download the first `frontend-metadata-*` workflow artifact.
4. Commit the exact `web/package-lock.json`, rebuilt `web/dist` and generated `docs/FRONTEND_DEPENDENCY_LICENSES.md`.
5. Fix evidence-based Svelte, TypeScript, test, Go, Windows, Docker or license failures without weakening assertions.
6. Merge only after all PR jobs pass.
7. Verify the merge-triggered `main` CI before marking Phase 3 complete or starting Phase 4.
