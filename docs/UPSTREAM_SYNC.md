# Synchronizing ainovel-cli Upstream

NovelForge keeps ainovel-cli as the long-term core upstream. Git remotes are local clone configuration and cannot be committed, so every working clone must create the remote explicitly.

## One-time setup

```bash
git remote add ainovel-upstream https://github.com/voocel/ainovel-cli.git
git fetch ainovel-upstream --tags
```

Verify:

```bash
git remote -v
git show ainovel-upstream/main --no-patch
```

The original first import revision is recorded in `UPSTREAM_BASE.md`.

## Compare before integrating

```bash
git fetch ainovel-upstream --tags
git log --left-right --cherry-pick --oneline main...ainovel-upstream/main
git diff --stat main...ainovel-upstream/main
```

Because the initial hosted import filtered historical `.github/workflows`, imported commit SHAs differ from original upstream SHAs. Compare by patch/content (`git range-diff`, `git cherry`, file diffs and original commit messages), not by assuming SHA identity.

## Recommended integration workflow

```bash
git switch main
git pull --ff-only origin main
git switch -c chore/sync-ainovel-YYYYMMDD
```

For a small number of isolated fixes, cherry-pick with provenance:

```bash
git cherry-pick -x <ainovel-upstream-sha>
```

For a coherent upstream release, merge it on the sync branch and resolve conflicts there:

```bash
git merge --no-ff ainovel-upstream/main
```

Do not force-push `main`. Open a pull request containing:

- upstream range/release and original SHAs;
- conflict list and resolution rationale;
- `gofmt`, `go vet ./...`, `go test ./...`, NovelForge command build and Docker build results;
- explicit review of CLI, config, store, engine, model routing and imported workflow changes.

## Conflict policy

Prefer upstream implementation for inherited core behavior unless it violates a documented NovelForge invariant. Prefer NovelForge adapters for Web/API/Truth/Job concerns. Avoid mass renames and formatting-only churn in upstream packages because they make future range-diff and conflict review harder.

High-risk paths include:

- `cmd/ainovel-cli`
- `internal/bootstrap`
- `internal/host`
- `internal/flow`
- `internal/store`
- `internal/agents`
- `internal/models`

NovelForge-specific packages such as `internal/server`, future `internal/truth`, `internal/context`, `internal/jobs`, and `web` should remain loosely coupled to those paths.

## After integration

```bash
gofmt -w ./cmd ./internal ./web
go vet ./...
go test ./...
go build ./cmd/novelforge
docker build .
```

Update `UPSTREAM_BASE.md` or an append-only sync log with the integrated upstream revision and date. Preserve upstream copyright/license changes and review any newly introduced third-party dependencies.
