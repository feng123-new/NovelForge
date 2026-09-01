# NovelForge Implementation Status

Last updated: 2026-09-01

## Repository state

- Actual remote base: `main@d7e9992d3bf21f58e0ca379eacc3851457c886fb`.
- Latest verified base CI: [run 33411404325](https://github.com/feng123-new/NovelForge/actions/runs/33411404325) — Linux, Windows, and Docker jobs passed.
- Current delivery branch: `feature/phase-02-project-api`.
- Validated Phase 2 implementation head: `8b0adfa33e286adefc6b5d6ea79e90e9601f8a89`.
- Pull request: [#7 — feat: complete project and API foundation](https://github.com/feng123-new/NovelForge/pull/7).
- Successful Phase 2 PR acceptance CI: [run 33462527697](https://github.com/feng123-new/NovelForge/actions/runs/33462527697).
- Main merge commit: not applicable at this document revision; Phase 2 has not been merged.
- Main CI result: not applicable at this document revision.
- Exact next phase after merged-main acceptance: Phase 3 — formal Web Workspace.

## Phase status

| Phase | Status | Notes |
| --- | --- | --- |
| 0 — upstream baseline | complete | Imported ainovel-cli baseline and retained provenance. |
| 1 — compatibility | complete | Dual-path runtime, setup/write routing, doctor, migration rollback, lifecycle smoke, branding and cross-platform gates are complete. |
| 2 — project/API foundation | PR acceptance passed; merge pending | All PR #7 Linux, Windows, and Docker jobs passed at implementation head `8b0adfa`; this status-only commit must pass the same gates before merge. Phase 2 is not complete until merged-main CI succeeds. |
| 3–13 | not started | Must begin only after Phase 2 is merged and the resulting `main` CI passes. |

## Completed

### Project lifecycle

- Added a workspace-bounded Project Repository with create, explicit skeleton import, detail, metadata update, archive, unarchive, duplicate, default trash deletion, and separately explicit permanent deletion.
- Preserved read compatibility for existing ainovel projects without implicit conversion.
- Added opaque random IDs for new projects and stable opaque IDs for legacy projects.
- Added the `.novelforge` project layout and per-project `project.db`.
- Added path containment, traversal rejection, Windows drive/UNC rejection, symlink escape rejection, Workspace/root/HOME/current-directory protection, Git repository root protection, exact delete confirmation, and Workspace-private trash.
- Added duplicate-time secret scrubbing and exclusion of runtime config, `.env*`, credential-like files, backups, trash, SQLite WAL/SHM files, and symlinks.

### API durability and safety

- Added required `Idempotency-Key` handling for every Phase 2 write route.
- Persisted operation, project, request hash, status, exact response status/body, creation time, and expiry.
- Replays an identical completed request and returns a conflict when the key is reused with a different request hash.
- Added a unified safe error envelope with stable code, message, details, retryability, and trace ID.
- Prevented error responses from exposing host absolute paths, SQL, stack traces, authorization headers, API keys, or provider secrets.
- Added stable project collection ordering, filtering, archived filtering, offset pagination, and `limit <= 100`.
- Added strict JSON decoding, bounded request bodies, and method validation.

### Durable events

- Added the Workspace `.novelforge/server.db` with durable event and idempotency storage.
- Added SQLite-backed event append and replay.
- Added `Last-Event-ID` replay, project filtering, heartbeats, restart recovery, and bounded slow-client fan-out.
- Events are committed before process-local delivery; a slow subscriber does not block state producers.

### Architecture and storage

- Added a versioned SQLite migration runner with immutable checksums, UTC timestamps, foreign keys, busy timeout, WAL, pre-migration backup, transactional application, rollback, unknown-version rejection, and applied-checksum validation.
- Added a CGo-free `modernc.org/sqlite` driver boundary; release builds can run with `CGO_ENABLED=0`.
- Added `EngineService` and a legacy `host.Host` adapter so HTTP handlers do not copy engine behavior.
- Kept Workspace control state separate from project story state: `.novelforge/server.db` owns API durability and each project owns `.novelforge/project.db`.
- Added a Windows-safe SQLite file URI encoder and Windows-only regression coverage.

### OpenAPI and contracts

- Expanded OpenAPI 3.1 to every Phase 2 route.
- Added unique operation ID checks, route drift checks, schema/reference checks, error envelope definitions, pagination definitions, and `Idempotency-Key` headers.
- Added route-level HTTP tests for project lifecycle, idempotency, errors, replay, request limits, strict JSON, and absolute-path redaction.

## Changed files

Major Phase 2 paths:

- `.github/workflows/ci.yml`
- `cmd/novelforge/server.go`
- `go.mod`
- `go.sum`
- `internal/db/migrate/*`
- `internal/project/*`
- `internal/server/api.go`
- `internal/server/engineadapter/*`
- `internal/server/eventstore/*`
- `internal/server/idempotency/*`
- `internal/server/repository/*`
- `internal/server/events.go`
- `internal/server/projects.go`
- `internal/server/server.go`
- `internal/server/server_test.go`
- `internal/server/openapi.json`
- `internal/server/openapi_test.go`
- `scripts/dependency_license_inventory.go`
- `scripts/dependency_license_inventory_test.go`
- `docs/PROJECT_API.md`
- `docs/DEPENDENCY_LICENSES.md`
- `docs/LICENSES.md`
- `docs/IMPLEMENTATION_STATUS.md`

## Architecture decisions

- Existing ainovel projects remain readable in place. Adding NovelForge metadata is an explicit import operation.
- Filesystem mutation is owned by `internal/project`; HTTP handlers pass opaque IDs and typed inputs rather than host paths.
- Default deletion is a reversible move to Workspace trash. Permanent deletion is separately explicit and uses the same confirmation and boundary checks.
- A completed idempotent request stores and replays the exact response; a reused key with a different request hash is a conflict.
- Events are committed before fan-out. A slow SSE subscriber is disconnected instead of blocking a state producer.
- The existing runtime remains authoritative behind an Engine adapter. Phase 2 does not copy or replace planning/writing logic.
- SQLite uses a pure-Go driver so official builds can set `CGO_ENABLED=0`.
- License reviews are exact to `module@version` and can replace only an `UNKNOWN` scanner component; already detected incompatible components remain blocking.

## Database / migration changes

- Added generic `schema_migrations(version, name, checksum, applied_at)` support.
- Added Workspace migrations for durable `events` and `idempotency_records`.
- Added the project migration for `project_metadata`.
- Existing databases with pending migrations are checkpointed and backed up before the transaction.
- A failed migration rolls back all pending schema and migration-record writes.
- An applied checksum mismatch or unknown applied version fails closed.
- Database timestamps are UTC and SQLite connections enable foreign keys, bounded busy timeout, and WAL.

## API changes

Implemented:

```text
GET    /api/health
GET    /api/openapi.json
GET    /api/events
GET    /api/projects
POST   /api/projects
GET    /api/projects/{id}
PATCH  /api/projects/{id}
POST   /api/projects/{id}/archive
POST   /api/projects/{id}/unarchive
POST   /api/projects/{id}/duplicate
DELETE /api/projects/{id}
```

All write routes require `Idempotency-Key`. Project collections have stable ordering, `limit <= 100`, offset pagination, query filtering, and archived filtering.

## UI changes

None in Phase 2. The existing embedded transitional Web asset remains intact. Formal Svelte/Vite/TypeScript source begins in Phase 3; no fake Web action was added.

## Tests executed

PR acceptance run [33462527697](https://github.com/feng123-new/NovelForge/actions/runs/33462527697) executed:

```text
go mod tidy: PASS
go mod download all: PASS
dependency license inventory generation: PASS
gofmt drift check: PASS
go vet ./...: PASS
go test -buildvcs=false -count=1 ./...: PASS
targeted race tests for migrations/project/server: PASS
OpenAPI route and schema tests: PASS
CGO_ENABLED=0 novelforge build: PASS
module lock drift check: PASS
dependency license policy and inventory check: PASS
embedded Web asset validation: PASS
lifecycle shell syntax: PASS
install -> upgrade -> dry-run uninstall -> uninstall smoke: PASS
NovelForge brand audit: PASS
Windows full repository tests with CGO_ENABLED=0: PASS
Windows novelforge build with CGO_ENABLED=0: PASS
Docker build: PASS
```

## Test results

All three PR #7 jobs completed successfully at `8b0adfa33e286adefc6b5d6ea79e90e9601f8a89`:

- Go lint, test and build: PASS.
- Windows test and build: PASS.
- Docker build: PASS.

Earlier evidence-based CI failures exposed and led to fixes for case-sensitive module checksums, an uncommitted tidy graph, deterministic fixture ID reuse, Windows SQLite URI authority parsing, and incomplete license-file classification. Assertions and production behavior were not weakened to obtain a pass.

## Performance

No Phase 2 performance claim is recorded. Project listing is a deterministic Workspace scan with bounded API pagination. Event replay is indexed by `(project_id, id)` and idempotency expiry is indexed by `expires_at`. Formal 100/500/1000-chapter and 100,000-event benchmarks belong to Phase 13.

## License review

- Added `modernc.org/sqlite v1.57.0` and committed the tidy-normalized module graph.
- Added a deterministic dependency license inventory and fail-closed CI policy gate.
- Added exact-version evidence for module snapshots whose cache license set contains an unresolved component; reviews cannot replace detected GPL/AGPL/LGPL/SSPL components.
- No source was copied from the MIT UI reference.
- No source, SQL, components, prompts, or tests were copied from GPL/AGPL clean-room references.

## Known issues

- Phase 2 is not complete until this status-only commit passes PR CI, PR #7 is merged, and the resulting `main` CI succeeds.
- The Web UI remains the transitional embedded dashboard; Phase 3 has not started.
- Structured Truth Store, quality gate, narrative ledger, context compiler, chapter versions, durable Autopilot, skills/reference libraries, lifecycle tooling, production diagnostics, benchmarks, and v0.1.0 release work remain Phase 4–13 deliverables.

## Next phase

After Phase 2 merged-main acceptance, start `feature/phase-03-web-workspace` from the verified `main` merge commit and implement the rebuildable Svelte/Vite/TypeScript Workspace with real backend-backed pages and frontend CI gates.

## Delivery evidence

- Functional implementation head: `8b0adfa33e286adefc6b5d6ea79e90e9601f8a89`.
- Pull request: [#7](https://github.com/feng123-new/NovelForge/pull/7).
- PR acceptance CI: [33462527697](https://github.com/feng123-new/NovelForge/actions/runs/33462527697) — success.
- Main merge commit: pending.
- Main CI: pending.

## Exact resume point

1. Verify the CI run triggered by this status-only commit completes successfully for Linux, Windows, and Docker.
2. Update PR #7 description with final acceptance evidence.
3. Squash-merge PR #7 into `main`; do not bypass the successful checks.
4. Verify the merge-triggered `main` CI completes successfully for every job.
5. Create `feature/phase-03-web-workspace` from that exact verified `main` commit.
6. Record the Phase 2 merge commit and main CI in this file on the Phase 3 branch, then begin the formal Web Workspace.
