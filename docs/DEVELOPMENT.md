# NovelForge development

## Current maintenance scope

Use the [current documentation index](README.md) and [Roadmap](ROADMAP.md) for Phase 1–8 maintenance. Phase 9–13 remain paused. This maintenance round uses static logic-chain checks, an affected entry build and named tests only; it does not run the full validation commands below. Record the actual commit and verification scope in the PR and current maintenance record. Historical documents under `docs/archive/` preserve prior evidence and are not the place to record new completion claims.

## Required toolchain

- Go version declared by `go.mod`
- POSIX shell utilities for packaging smoke tests
- Docker for image validation
- GitHub Actions for Linux and Windows gates

## Full baseline validation — retained reference, not this maintenance round

Run from the repository root when full validation is explicitly in scope:

```bash
test -z "$(gofmt -l .)"
GOWORK=off go vet ./...
GOWORK=off go test -buildvcs=false -count=1 ./...
GOWORK=off go build -trimpath ./cmd/novelforge
sh -n scripts/install.sh scripts/uninstall.sh scripts/install_lifecycle_smoke.sh scripts/brand_audit.sh
sh scripts/install_lifecycle_smoke.sh
sh scripts/brand_audit.sh
docker build --tag novelforge-local .
```

Select affected checks for the agreed change scope. Do not silently expand a limited maintenance task into a full regression run, or report skipped checks as passed.

## Delivery protocol

Changes are delivered through a focused branch and pull request:

1. start from the latest `main`;
2. update the applicable current module or maintenance documentation, not archived acceptance records;
3. implement the production path before UI controls when adding functionality;
4. preserve existing tests and add targeted coverage when behavior changes;
5. update OpenAPI and documentation when applicable;
6. run the checks agreed for the current scope and record what was not run;
7. open a pull request;
8. merge only after the agreed checks succeed and applicable branch protections permit it;
9. record the exact delivery point and retain required historical evidence.

Do not combine Phase 2 through Phase 13 into one unreviewable change. Finishing a cleanup does not reopen a paused phase.

## Command compatibility profiles

The shared packages deliberately support two process profiles:

- `cmd/ainovel-cli`: unchanged legacy `.ainovel` paths;
- `cmd/novelforge`: new-path-first `.novelforge` behavior with `.ainovel` fallback.

`cmd/novelforge` activates the profile before shared packages read configuration, rules, caches, setup state or startup logs. Tests that need NovelForge semantics set `compat.RuntimeProfileEnv`; tests must isolate `HOME`, `USERPROFILE` and cwd.

Configuration precedence and migration guarantees are documented in `docs/MIGRATION.md`.

## Compatibility invariants

- normal startup never moves or copies credentials;
- one file is selected per scope, so `.novelforge` and `.ainovel` credentials at the same scope are not merged;
- project may overlay global after each scope selects one generation;
- explicit configuration is authoritative;
- first-run NovelForge setup writes `.novelforge`;
- TUI configuration writes update the active layer;
- legacy command behavior remains covered by regression tests;
- startup logs use owner-only permissions;
- migrations retain source and verified backup data.

## Installation lifecycle tests

`scripts/install_lifecycle_smoke.sh` is offline and deterministic. It supplies a fake GitHub Release transport and verifies:

1. checksum-validated initial installation;
2. checksum-validated upgrade replacement;
3. dry-run uninstall does not remove the binary;
4. uninstall removes only the executable;
5. `.novelforge` and `.ainovel` credentials remain byte-for-byte unchanged.

`scripts/brand_audit.sh` checks active packaging surfaces for stale executable, release and Docker path branding while allowing explicit compatibility documentation and upstream credits.

## Migration tests

Migration tests verify:

- dry-run performs no writes;
- source directories remain after success;
- destination directories are never overwritten;
- a backup exists before destination commit;
- manifests do not contain secret values;
- symbolic links are rejected;
- repeated execution is idempotent;
- cancellation after backup removes staging and leaves no destination;
- behavior passes on Linux and Windows CI.

## License review

Before adding a dependency:

1. identify its direct license;
2. inspect transitive dependencies;
3. confirm compatibility with Apache-2.0 distribution;
4. update `docs/LICENSES.md` and the dependency inventory;
5. preserve required notices for copied MIT code;
6. do not copy AGPL or GPL implementation code into the repository.

Phase 1 adds no third-party dependency.
