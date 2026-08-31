# NovelForge Implementation Status

Last updated: 2026-08-31

## Repository state

- Base branch: `main`
- Base commit: `fff850da34fe490ffbfb4eb595a5f302711269a5`
- Active branch: `feature/phase-1-compatibility-completion`
- Active phase: Phase 1 — brand, configuration and compatibility
- Migration manifest version: `1`
- Pull request: pending
- CI: pending

## Phase status

| Phase | Status | Notes |
| --- | --- | --- |
| 0 — upstream baseline | complete | Imported ainovel-cli baseline and retained provenance. |
| 1 — compatibility | implementation complete; merge gate pending | Dual-path runtime, setup/write routing, doctor, migration rollback, lifecycle smoke and branding are implemented on the active branch. |
| 2 — project/API foundation | foundation only | Existing server remains read-only. |
| 3–13 | not started | Must follow test-gated delivery. |

## Completed in Phase 1

- Kept `cmd/ainovel-cli` on its original `.ainovel` configuration and rules behavior.
- Activated a separate NovelForge runtime profile before shared packages read setup state, configuration, rules or startup logs.
- Implemented configuration precedence:
  1. `--config`
  2. `NOVELFORGE_CONFIG`
  3. project `.novelforge`
  4. project `.ainovel`
  5. global `.novelforge`
  6. global `.ainovel`
- Selected one complete file per scope; new and legacy credential files at the same scope are never merged.
- Kept project-over-global overlay after each scope selects one generation.
- Made first-run NovelForge setup target `.novelforge` while legacy-only installations continue in place without implicit copy or move.
- Made TUI `/config` and `/model`, startup error logs and rules directories follow the active NovelForge layer.
- Added `--config` to TUI/headless startup.
- Updated `novelforge doctor` to report selected, fallback and shadowed layers, explicit configuration and runtime validation without exposing secrets.
- Retained `novelforge migrate` dry-run, scope selection, timestamped backup, SHA-256 manifest, symlink rejection, atomic destination commit and idempotence.
- Added deterministic failure-after-backup coverage proving source/backup retention, staging cleanup and absence of a partial destination.
- Added Linux/Windows dual-path regression coverage through the existing cross-platform test matrix.
- Added offline initial-install, upgrade, dry-run-uninstall and uninstall smoke coverage.
- Added safe `scripts/uninstall.sh`, which removes only the executable and preserves `.novelforge`, `.ainovel` and projects.
- Added an active packaging brand audit and switched Docker Compose to `/root/.novelforge`.
- Updated README, MIGRATION, DEVELOPMENT and ROADMAP documentation.

## Validation

Locally available checks in the restricted execution environment:

```text
gofmt on all changed Go files
sh -n scripts/uninstall.sh scripts/install_lifecycle_smoke.sh scripts/brand_audit.sh
install -> upgrade -> dry-run uninstall -> uninstall smoke: PASS
```

Authoritative repository gates required before merge:

- formatting
- `go vet ./...`
- `go test -buildvcs=false -count=1 ./...`
- Linux build
- Windows test/build
- install/upgrade/uninstall smoke
- brand audit
- Docker build

## Architecture decisions

- The command profile is process-local and activated only by `cmd/novelforge`; the legacy command does not inherit the new precedence.
- An explicit configuration is authoritative rather than an overlay, preventing accidental credential combination.
- `.novelforge` shadows `.ainovel` at the same scope even when the selected new file is invalid; invalid preferred configuration fails loudly instead of silently reverting.
- Global parse failure remains recoverable when a complete project layer exists, preserving established behavior.
- Normal startup performs no data migration. Migration remains an explicit copy-only command.

## License review

- No new Go or npm dependency was added.
- No source was copied from AGPL or GPL reference projects.
- Existing Apache-2.0 and third-party notice boundaries are unchanged.

## Known issues after Phase 1

These belong to later phases, not compatibility completion:

- Server project APIs are still read-only.
- SSE events are process-local and not replayable.
- The Web UI is still the embedded transitional dashboard.
- SQLite Truth Store, durable jobs, chapter versions and Web Autopilot are not implemented.

## Exact resume point

After this branch passes CI and merges, start Phase 2 from the resulting `main` HEAD:

1. add the Project Repository and safe project lifecycle writes;
2. add the unified API error envelope and idempotency store;
3. introduce the SQLite migration runner foundation;
4. add Engine Adapter boundaries and durable event repository interfaces;
5. update OpenAPI and API integration tests before adding Web controls.
