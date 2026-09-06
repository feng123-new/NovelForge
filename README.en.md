# NovelForge

[简体中文](README.md) · [Interface languages](docs/I18N.md) · [rc.2 notes](docs/releases/v0.1.0-rc.2.md)

NovelForge is a local-first long-form fiction workspace built with Go, SQLite and an embedded Web interface. It combines structured story facts, chapter-bounded character knowledge, narrative ledgers, budgeted context, immutable chapter versions, resumable writing jobs, writing skills/reference libraries, import/export/backup and cost diagnostics. Model proposals do not become authoritative facts until the chapter is accepted and finalized.

## Bilingual workspace

Choose **简体中文 / English** in the header or Settings. Switching changes only the browser display preference. It preserves projects, manuscript language, unsaved edits, cursor/selection, selected files and tasks, and never invokes a model. Original manuscripts, resource Markdown and technical records are not translated.

## Getting started

Download the matching `v0.1.0-rc.2` prerelease archive from GitHub Releases and verify `novelforge_checksums.txt`. Extract into a new directory, then start in a separate test workspace:

```sh
./novelforge server --workspace ./workspace-test --no-autopilot
```

On Windows use `.\novelforge.exe`. Open `http://127.0.0.1:48090`. The flag disables the automatic task worker, not all writes. Configure providers using server-side files/environment references; do not store credentials in the browser. Set explicit model prices and budgets before removing the flag and starting paid work. The local server is not a public multi-user authenticated service.

Create a novel and save its foundation request, or import existing text as reviewable candidates. Configure resources and model roles, start a bounded writing job explicitly, then inspect/check/accept/finalize candidates in Chapter versions. Export finalized text and make project ZIP backups regularly.

## Source build

Use the Go version in `go.mod` (1.25.5). The committed `web/dist` is embedded into the binary. Frontend changes require Node 22 and rebuilding the embedded assets.

```sh
CGO_ENABLED=0 go build -trimpath -o novelforge ./cmd/novelforge
./novelforge server --workspace ./workspace-test --no-autopilot
```

## Verification boundaries

Phase 1–12 capabilities and Phase 13A candidate delivery are implemented; Phase 13B full acceptance is pending. The rc.2 change has focused bilingual/state-preservation tests and real Chrome route checks, not full scale or paid-model literary certification. Linux native no-model smoke is recorded; other platform archives require native acceptance. This is a prerelease, not stable/latest.

See [deployment](docs/DEPLOYMENT.md), [local acceptance](docs/LOCAL_ACCEPTANCE.md), [architecture](docs/ARCHITECTURE.md) and [documentation index](docs/README.md). Retained upstream code and notices remain under their stated licenses.
