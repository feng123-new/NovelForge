# NovelForge development

## Required toolchain

- Go version declared by `go.mod`
- Docker for image validation
- GitHub Actions for the cross-platform gate

## Baseline validation

Run from the repository root:

```bash
test -z "$(gofmt -l .)"
GOWORK=off go vet ./...
GOWORK=off go test -buildvcs=false -count=1 ./...
GOWORK=off go build -trimpath ./cmd/novelforge
docker build --tag novelforge-local .
```

Run targeted race tests when changing concurrent server, store, event or job code.

## Delivery protocol

Each roadmap phase is delivered through a focused feature branch and pull request:

1. start from the latest `main`;
2. update `docs/IMPLEMENTATION_STATUS.md`;
3. implement the production path before adding UI controls;
4. add unit and integration tests;
5. update OpenAPI and documentation when applicable;
6. run the local gates;
7. open a pull request;
8. merge only after GitHub Actions succeeds;
9. record the exact resume point.

Do not combine Phase 1 through Phase 13 into one unreviewable change.

## Compatibility work

The ainovel-compatible command and NovelForge command share packages. Compatibility changes must therefore distinguish between:

- behavior that must remain unchanged for `cmd/ainovel-cli`;
- behavior activated only by `cmd/novelforge`;
- project data that can be read without migration;
- explicit migrations that copy and back up user data.

Never silently move or delete API keys. Tests must isolate `HOME`, `USERPROFILE` and project roots.

## Migration tests

Migration tests must verify:

- dry-run performs no writes;
- source directories remain after success;
- destination directories are never overwritten;
- a backup exists before destination commit;
- manifests do not contain secret values;
- symbolic links are rejected;
- repeated execution is idempotent;
- cancellation leaves no partial destination;
- behavior passes on Linux and Windows CI.

## License review

Before adding a dependency:

1. identify its direct license;
2. inspect transitive dependencies;
3. confirm compatibility with Apache-2.0 distribution;
4. update `docs/LICENSES.md` and the dependency inventory;
5. preserve required notices for copied MIT code;
6. do not copy AGPL or GPL implementation code into the repository.
