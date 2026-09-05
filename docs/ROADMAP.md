# NovelForge Roadmap

## Current scope — 2026-09-05

Phase 1–8 delivery and the cleanup remain the baseline. Phase 9 has been explicitly reopened and implemented as a durable local Autopilot over the existing quality/version coordinators. Phase 10–13 remain paused; finishing this phase does not start them automatically.

[AUTOPILOT.md](AUTOPILOT.md) is the current Phase 9 behavior and verification boundary. [PHASE_01_08_FIXES.md](PHASE_01_08_FIXES.md) records the earlier integration repair. Original reviews and acceptance evidence remain in [the archive](archive/README.md); they are not rewritten as new acceptance claims.

Verification is limited to the affected entry build, named short-flow tests, changed frontend type checking and asset build. No full Go/frontend/race suites, platform matrices, paid-model run or 300-chapter simulation is claimed. Actual run and exact source references are recorded in the delivery PR. Broader scale acceptance remains unverified.

## Phase 0 — Import upstream baseline — Historical acceptance retained

- Imported ainovel-cli from original upstream commit `c0900290be8dfbae4d1614726e48b53259efbd47`.
- Retained auditable non-workflow source lineage and surviving commit metadata; workflow-only commits may have been pruned and imported SHAs were rewritten. Details are in `UPSTREAM_BASE.md`.
- Preserved Apache-2.0 license and core packages.
- Baseline `gofmt`, `go vet ./...` and `go test ./...` passed before NovelForge changes; these are historical results, not tests rerun during consolidation.

## Phase 1 — Brand and compatibility — Delivered; consolidate entry boundaries

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

Current focus: default Web quality services now use project/global configuration and server --config; preserve configuration isolation. Do not rewrite the legacy entry or perform implicit data migration.

## Phase 2 — Embedded server and API foundation — Delivered; trace service wiring

- `novelforge server --host --port --workspace`, loopback default and non-loopback warning.
- Project lifecycle writes, opaque IDs, safe deletion and project-local databases.
- Engine adapter boundary, durable event replay and persistent idempotency.
- REST health/projects endpoints, safe error envelopes and OpenAPI validation.
- Unit/API tests and graceful shutdown retained.

Current focus: follow the actual server constructor into configured services. An adapter interface alone does not prove that the browser can start a working model-backed flow.

## Phase 3 — Web workspace — Delivered; reconcile capability availability

- Svelte + Vite + Tailwind + DaisyUI build embedded with `go:embed`.
- Dashboard, Projects, Chapters, Models and Logs backed by APIs.
- Dark/light responsive IDE layout.
- Later delivered Ledger and Versions pages consume their corresponding APIs.

Current focus: distinguish module availability from model-service readiness and each action's prerequisites. New Novel stores a Foundation request; Phase 9 now executes it after an explicit start from the Autopilot page.

## Phase 4 — Structured Truth Store — Delivered; preserve authority boundaries

- SQLite migrations and repository interfaces.
- Entity, Character, time-bounded CharacterState, Relation, Timeline, Knowledge, Inventory, Fact and Provenance.
- Event projections and Chapter-N temporal queries.
- Migration/state isolation tests, including “Chapter 100 cannot change Chapter 50,” retained as historical coverage.

Current focus: trace accepted writes, temporal query boundaries and the relationship between immutable events and derived state. Do not mark new runtime or scale acceptance from a static review.

## Phase 5 — Librarian and Continuity gate — Delivered module; default Web configuration connected

- JSON Schema fact proposals and deterministic validation.
- Draft persistence, bounded revision selection and recoverable Final commit.
- Character state, timeline, inventory, relation and knowledge-boundary checks.
- Severe FAIL versus advisory WARN semantics.

Current focus: model services are connected through normal startup/configuration without weakening validation or allowing a blocking FAIL to finalize; verify affected paths only. This is a Phase 1–8 integration follow-up, not an Autopilot implementation.

## Phase 6 — Narrative ledger — Delivered; trace accepted-Final consumption

- Foreshadow lifecycle, payoff windows and computed OVERDUE.
- Independent Secret system and holder/public visibility.
- Planner-facing provider and dashboard status.

Current focus: preserve accepted-Final-only model-originated writes, distinguish explicit management actions from model proposals, and trace Chapter-N/POV boundaries into consuming context paths. Do not infer complete runtime integration merely from provider existence.

## Phase 7 — Context Compiler and hybrid retrieval — Delivered module; selected-input integration repaired

- Five context layers and configurable token allocation.
- Structured/timeline/foreshadow/relation/recent/FTS5 pipeline.
- Vector retrieval interface without mandatory external service.
- Context latency and overflow tests retained.

Current focus: legacy output now contains only selected records, and Web Writer receives compiled project context. Additive character FTS supports Chinese substring terms. Verify selected inputs and small retrieval samples, not full-book scale.

## Phase 8 — Chapter version workflow — Delivered module; review complete action chain

- Draft, continuity_fix, editor_revision, human_revision and final versions.
- Diff, restore, accept/reject and immutable provenance.
- Final-only Truth commit and human-edit invalidation/rebuild.

Current focus: distinguish save/restore from semantic Check/Accept and Finalize. Trace the existing coordinator's model dependency, accepted evaluation, Truth/Ledger commits, file and Active Final switches, derived state and checkpoint. Saving a human revision alone is not successful end-to-end acceptance.

## Phase 9 — Durable Autopilot — Implemented; targeted validation only

- Local durable job queue: pending/running/paused/retrying/failed/completed/cancelled.
- START / PAUSE / STOP / CONTINUE calling the real Engine.
- Bounded rewrite policy and resumable checkpoints.
- Foundation and ChapterPlan generation use the configured model and persisted selected context.
- Chapter work reuses the actual quality/version coordinators; Web has real controls and review text.
- The previously planned Fake-LLM 300-chapter simulation is not run under the current limited-check scope; do not infer large-scale acceptance.

## Phase 10 — Skills, style and reference systems — Deferred; frozen

- Markdown Writing/Review/Polish/Planning Skills.
- Configurable anti-AI-flavor and phrase repetition rules.
- Separate Style Library and Knowledge Library with FTS indexing.

## Phase 11 — Novel lifecycle tooling — Deferred; frozen

- Web TXT/MD/EPUB import with resumable analysis.
- TXT/Markdown/EPUB export.
- ZIP backup/restore excluding global API credentials.
- Project-format migration with automatic pre-migration backup.

## Phase 12 — Diagnostics, cost and observability — Deferred; frozen

- Role/provider/model call, token and cost statistics.
- Provider health/fallback/pause policy.
- Stuck jobs, rewrite loops, stale summaries, contradictions, overdue foreshadows, context overflow and token explosion diagnostics.
- Structured logs and live SSE projection.

## Phase 13 — v0.1.0 hardening — Deferred; frozen

- Complete unit, integration, migration, state-machine, context, Truth, API and regression suites.
- 100/500/1000 chapter benchmarks.
- Frontend tests/build, Docker build and release matrices.
- Full documentation set and signed/checksummed multi-platform release.
