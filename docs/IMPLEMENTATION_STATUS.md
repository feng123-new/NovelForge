# NovelForge Implementation Status

Last updated: 2026-08-31

## Repository state

- Base branch: `main`
- Base commit: `3400b868a04c228841316156d56e36dba06b7e02`
- Active branch: `feature/phase-1-compatibility-tooling`
- Active phase: Phase 1 — brand, configuration and compatibility
- Migration manifest version: `1`
- Pull request: https://github.com/feng123-new/NovelForge/pull/5
- CI: pending

## Phase status

| Phase | Status | Notes |
| --- | --- | --- |
| 0 — upstream baseline | complete | Imported ainovel-cli baseline and retained provenance. |
| 1 — compatibility | in progress | This iteration adds safe migration tooling and diagnostics. Runtime config precedence still uses `.ainovel`; activation of `.novelforge` remains the next compatibility slice. |
| 2 — project/API foundation | foundation only | Existing server remains read-only. |
| 3–13 | not started | Must follow test-gated delivery. |

## Completed in this iteration

- Added a platform-neutral path inventory for global and project `.novelforge` / `.ainovel` locations.
- Added `novelforge doctor` with text and JSON reports that never print configuration contents or API keys.
- Added `novelforge migrate` with global/project scopes and dry-run support.
- Migration creates a timestamped backup before writing the destination.
- Migration copies through a same-parent staging directory and commits with an atomic rename.
- Legacy directories remain in place.
- Existing destination directories are never overwritten.
- Symbolic links and unsupported file types are rejected.
- Backup and destination manifests contain relative file names, sizes and SHA-256 values, not file contents.
- Added unit tests for precedence inventory, dry-run, idempotence, backups, manifests, cancellation, symlink rejection and secret-safe command output.

## Validation

Local container limitations prevent cloning the repository or downloading the Go 1.25.5 toolchain because outbound DNS is disabled.

Executed locally against the isolated new packages with the installed Go 1.23.2 toolchain:

```text
GOTOOLCHAIN=local go test ./internal/compat ./cmd/novelforge
PASS
```

The repository GitHub Actions run is the authoritative full gate for:

- formatting
- `go vet ./...`
- `go test -buildvcs=false -count=1 ./...`
- Linux build
- Windows test/build
- Docker build

## Known issues

- The main TUI/headless runtime still reads `~/.ainovel` and `./.ainovel` in this iteration.
- Running `novelforge migrate` prepares a backed-up `.novelforge` copy but does not delete or deactivate `.ainovel`.
- `doctor` deliberately reports the legacy runtime source until the dual-path loader is merged.
- Installer upgrade/uninstall smoke coverage is not yet complete.

## Exact resume point

Next branch should start from the merged result of this iteration and implement the Phase 1 runtime switch:

1. keep `cmd/ainovel-cli` on the legacy layout;
2. make `cmd/novelforge` read `.novelforge` first with `.ainovel` fallback;
3. make first-run setup write `.novelforge` without silently copying credentials;
4. make TUI configuration writes target the active NovelForge layer;
5. add dual-path and cross-platform regression tests;
6. complete installer upgrade/uninstall smoke tests;
7. update this file and only then declare Phase 1 complete.
