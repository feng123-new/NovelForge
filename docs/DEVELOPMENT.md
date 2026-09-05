# NovelForge development

## Required toolchain

- Go version declared by `go.mod`
- POSIX shell utilities for packaging smoke tests
- Docker for image validation
- GitHub Actions for Linux and Windows gates

## Baseline validation

Run from the repository root:

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

Run targeted race tests when changing concurrent server, store, event or job code.

## Delivery protocol

Each roadmap phase is delivered through a focused feature branch and pull request:

1. start from the latest `main`;
2. update `docs/archive/phase-01-08/IMPLEMENTATION_STATUS.md`;
3. implement the production path before UI controls;
4. add unit and integration tests;
5. update OpenAPI and documentation when applicable;
6. run local gates;
7. open a pull request;
8. merge only after GitHub Actions succeeds;
9. record the exact resume point.

Do not combine Phase 2 through Phase 13 into one unreviewable change.

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
