# NovelForge Implementation Status

Last updated: 2026-09-01

## Repository state

- Actual remote base: `main@d7e9992d3bf21f58e0ca379eacc3851457c886fb`
- Latest verified base CI: [run 33411404325](https://github.com/feng123-new/NovelForge/actions/runs/33411404325) — Linux, Windows, and Docker jobs passed.
- Current delivery branch: `feature/phase-02-project-api`
- Phase 2 implementation commit: not created yet at this document revision.
- Pull request: not created yet at this document revision.
- Phase 2 CI: not run yet at this document revision.
- Main merge commit: not applicable; Phase 2 is not merged.
- Exact next phase after Phase 2 acceptance: Phase 3 — formal Web Workspace.

## Phase status

| Phase | Status | Notes |
| --- | --- | --- |
| 0 — upstream baseline | complete | Imported ainovel-cli baseline and retained provenance. |
| 1 — compatibility | complete | Dual-path runtime, setup/write routing, doctor, migration rollback, lifecycle smoke, branding and cross-platform gates are complete. |
| 2 — project/API foundation | implementation in progress | Project lifecycle, API, SQLite migration runner, idempotency, durable SSE, engine adapter, tests, OpenAPI, and docs are implemented on the feature branch; real dependency resolution and GitHub Actions acceptance remain. |
| 3–13 | not started | Must begin only after Phase 2 PR and merged-main CI pass. |

## Phase 2 completed in the working implementation

- Added a workspace-bounded Project Repository with create, explicit skeleton import, detail, metadata update, archive, unarchive, duplicate, default trash deletion, and explicit permanent deletion.
- Preserved read compatibility for existing ainovel projects without implicit conversion.
- Added opaque random IDs for new projects and stable opaque IDs for legacy projects.
- Added the `.novelforge` project layout and per-project `project.db`.
- Added path containment, Windows/UNC/traversal rejection, symlink escape rejection, workspace/root/home/current-directory protection, Git repository root protection, explicit delete confirmation, and workspace-private trash.
- Added duplicate-time secret scrubbing and exclusion of runtime config, `.env*`, credential-like files, backups, trash, WAL/SHM files, and symlinks.
- Added the workspace `server.db` with durable events and idempotency records.
- Added versioned SQLite migrations with checksums, UTC timestamps, foreign keys, busy timeout, WAL, pre-migration backup, and transaction rollback.
- Added the CGo-free `modernc.org/sqlite` driver boundary.
- Added the unified error envelope and request trace IDs without raw paths, SQL, stack traces, or credentials.
- Added required `Idempotency-Key` handling and exact status/body replay for all write routes.
- Added durable SSE replay by `Last-Event-ID`, project filtering, heartbeats, restart recovery, and bounded slow-client handling.
- Added `EngineService` and a legacy host adapter so HTTP code does not duplicate engine logic.
- Expanded OpenAPI 3.1 to every Phase 2 route and added drift/operation-ID tests.
- Added API, repository, migration, idempotency, event replay, path security, legacy compatibility, and Windows-path tests.
- Expanded CI with targeted race tests, OpenAPI validation, CGo-disabled builds, dependency lock drift, and generated license inventory checks.

## Changed files

Planned Phase 2 commit paths:

- `.github/workflows/ci.yml`
- `cmd/novelforge/server.go`
- `go.mod`
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
- `docs/LICENSES.md`
- `docs/IMPLEMENTATION_STATUS.md`
- `docs/DEPENDENCY_LICENSES.md` after CI resolves the exact module graph
- `go.sum` after CI resolves the exact module graph

## Architecture decisions

- Workspace control state is separated from per-project story state: `.novelforge/server.db` holds API durability while each project owns `.novelforge/project.db`.
- Existing ainovel projects remain readable in place. Adding NovelForge metadata is an explicit import operation.
- Filesystem mutation is owned by `internal/project`; HTTP handlers pass opaque IDs and typed inputs rather than host paths.
- Default deletion is a reversible move to workspace trash. Permanent deletion is separately explicit and uses the same confirmation and boundary checks.
- A completed idempotent request stores and replays the exact response; a reused key with a different request hash is a conflict.
- Events are committed before fan-out. A slow SSE subscriber is disconnected instead of blocking a state producer.
- The existing runtime remains authoritative behind an Engine adapter. Phase 2 does not copy or replace its planning/writing logic.
- SQLite uses a pure-Go driver so official builds can set `CGO_ENABLED=0`.

## Database / migration changes

- Added generic `schema_migrations(version, name, checksum, applied_at)` support.
- Added workspace migrations for durable `events` and `idempotency_records`.
- Added project migration for `project_metadata`.
- Existing databases with pending migrations are checkpointed and backed up before the transaction.
- A failed migration rolls back all pending schema and migration-record writes.
- An applied checksum mismatch or unknown applied version fails closed.

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

None in Phase 2. The existing embedded transitional Web asset remains intact. Formal Svelte/Vite/TypeScript source begins in Phase 3; no fake Web actions were added.

## Tests executed before the first remote commit

The connected execution environment cannot resolve external GitHub/module hosts, so it cannot download the new SQLite module locally. The following checks were nevertheless run against the implementation with a temporary, uncommitted compile-only SQLite/bootstrap/host/Web stub boundary:

```text
gofmt over all changed Go files: PASS
compile-only go test for internal/db/migrate: PASS
targeted compile-only go test for internal/project: PASS
targeted compile-only go test for internal/server and subpackages: PASS
scripts package unit tests: PASS
OpenAPI JSON parse: PASS
engine adapter signatures checked against the real host.Host source: PASS
```

The temporary stubs are outside the planned commit. Real module resolution, SQLite runtime tests, full repository tests, race tests, Windows, Docker, and CGo-disabled builds must pass in GitHub Actions before merge.

## Performance

No Phase 2 performance claim is recorded before the real CI run. Project listing is a deterministic single workspace scan with bounded API pagination. Event replay is indexed by `(project_id, id)` and idempotency expiry is indexed by `expires_at`.

## License review

- Added `modernc.org/sqlite`; the resolved version and complete transitive graph must be locked and inventoried before merge.
- Added a standard-library-only license inventory generator and CI policy gate.
- No source was copied from the MIT UI reference.
- No source, SQL, components, prompts, or tests were copied from GPL/AGPL clean-room references.

## Known issues / remaining acceptance work

- The exact `go.sum`, tidy-normalized `go.mod`, and generated dependency inventory require a real networked Go toolchain. The first PR CI run will publish these generated files as a workflow artifact; they must be committed and revalidated before merge.
- Phase 2 is not complete until every PR job passes, the PR is merged, and the merged `main` CI passes.
- Phase 3–13 are untouched in this branch.

## Exact resume point

1. Create `feat: complete project and API foundation` on `feature/phase-02-project-api` from `main@d7e9992d3bf21f58e0ca379eacc3851457c886fb`.
2. Open the Phase 2 pull request.
3. Inspect every GitHub Actions job and download the dependency metadata artifact.
4. Commit the exact tidy-normalized `go.mod`, `go.sum`, and `docs/DEPENDENCY_LICENSES.md`.
5. Fix evidence-based test, race, Windows, Docker, OpenAPI, or license failures without lowering assertions.
6. Merge only after all PR jobs pass; then verify the merged-main CI before marking Phase 2 complete or starting Phase 3.
