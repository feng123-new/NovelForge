# NovelForge Roadmap

This roadmap follows incremental, test-gated delivery. A phase is marked complete only when its production path, tests and documentation are in the repository; a page mock or unconnected button does not count.

## Phase 0 — Import upstream baseline — Complete

- Imported ainovel-cli from original upstream commit `c0900290be8dfbae4d1614726e48b53259efbd47`.
- Retained auditable non-workflow source lineage and surviving commit metadata; workflow-only commits may have been pruned and imported SHAs were rewritten. Details are in `UPSTREAM_BASE.md`.
- Preserved Apache-2.0 license and core packages.
- Baseline `gofmt`, `go vet ./...` and `go test ./...` passed before NovelForge changes.

## Phase 1 — Brand and compatibility — Complete

- `cmd/novelforge` brand entry with TUI/headless compatibility.
- Legacy `cmd/ainovel-cli` remains intact and keeps `.ainovel` path behavior.
- NovelForge supports global and project `.novelforge` directories with `.ainovel` fallback.
- Deterministic precedence: explicit config, project new/legacy, global new/legacy.
- New and legacy credentials at the same scope are never merged.
- First-run setup and generated rules use `.novelforge`; active legacy files are read in place.
- `novelforge doctor` reports active and shadowed layers without exposing secrets.
- `novelforge migrate` provides dry-run, backups, manifests, atomic copy-only commit, rollback cleanup and idempotence.
- TUI configuration writes and startup logs target the active NovelForge layer.
- Docker, installer, updater and GoReleaser target `novelforge`.
- Offline install/upgrade/uninstall smoke coverage preserves configuration data.
- Linux and Windows regression gates cover the dual-path behavior.
- License notices, migration, development, architecture, roadmap and upstream sync documentation are present.

## Phase 2 — Embedded server and API foundation — Foundation delivered

- `novelforge server --host --port --workspace`.
- Loopback default and non-loopback warning.
- Embedded responsive Web shell.
- Real existing-project discovery and progress aggregation.
- REST health/projects endpoints, OpenAPI 3.1 and SSE transport.
- Unit/API tests and graceful shutdown.

Next additions: project lifecycle writes, Engine adapter, durable event replay, API error schema expansion and generated OpenAPI validation.

## Phase 3 — Web workspace

- Svelte + Vite + Tailwind + DaisyUI build embedded with `go:embed`.
- Dashboard, Projects, Chapters, Models and Logs backed by real APIs.
- Dark/light responsive IDE layout.
- No fake Create/Generate/Start controls.

## Phase 4 — Structured Truth Store

- SQLite migrations and repository interfaces.
- Entity, Character, time-bounded CharacterState, Relation, Timeline, Knowledge, Inventory, Fact and Provenance.
- Event projections and Chapter-N temporal queries.
- Migration/state isolation tests, including “Chapter 100 cannot change Chapter 50.”

## Phase 5 — Librarian and Continuity gate

- JSON Schema fact proposals.
- Engine validation and atomic commit.
- Character state, timeline, inventory, relation and knowledge-boundary checks.
- Severe FAIL versus advisory WARN semantics.

## Phase 6 — Narrative ledger

- Foreshadow lifecycle, payoff windows and OVERDUE calculation.
- Independent Secret system and holder/public visibility.
- Planner injection and dashboard status.

## Phase 7 — Context Compiler and hybrid retrieval

- Five context layers and configurable token allocation.
- Structured/timeline/foreshadow/relation/recent/FTS5 pipeline.
- Vector retrieval interface without mandatory external service.
- Context latency and overflow tests.

## Phase 8 — Chapter version workflow

- Draft, continuity_fix, editor_revision, human_revision and final versions.
- Diff, restore, accept/reject and immutable provenance.
- Final-only Truth commit and human-edit invalidation/rebuild.

## Phase 9 — Durable Autopilot

- Local durable job queue: pending/running/paused/retrying/failed/completed/cancelled.
- START / PAUSE / STOP / CONTINUE calling the real Engine.
- Bounded rewrite policy and resumable checkpoints.
- Fake-LLM 300-chapter simulation.

## Phase 10 — Skills, style and reference systems

- Markdown Writing/Review/Polish/Planning Skills.
- Configurable anti-AI-flavor and phrase repetition rules.
- Separate Style Library and Knowledge Library with FTS indexing.

## Phase 11 — Novel lifecycle tooling

- Web TXT/MD/EPUB import with resumable analysis.
- TXT/Markdown/EPUB export.
- ZIP backup/restore excluding global API credentials.
- Project-format migration with automatic pre-migration backup.

## Phase 12 — Diagnostics, cost and observability

- Role/provider/model call, token and cost statistics.
- Provider health/fallback/pause policy.
- Stuck jobs, rewrite loops, stale summaries, contradictions, overdue foreshadows, context overflow and token explosion diagnostics.
- Structured logs and live SSE projection.

## Phase 13 — v0.1.0 hardening

- Complete unit, integration, migration, state-machine, context, Truth, API and regression suites.
- 100/500/1000 chapter benchmarks.
- Frontend tests/build, Docker build and release matrices.
- Full documentation set and signed/checksummed multi-platform release.
