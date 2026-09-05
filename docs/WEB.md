# NovelForge Web Workspace

> Maintenance update (2026-09-05): the CLI server now loads project/global quality model configuration and accepts `server --config`. Writer requests include compiled project context. Provider availability is a configuration condition, not a health probe. Workers remain unavailable. See [PHASE_01_08_FIXES.md](PHASE_01_08_FIXES.md) for current scope and verification limits.

## Status

Phase 3 replaces the transitional handwritten asset with reproducible Svelte, TypeScript, Vite, Tailwind and DaisyUI source. The production build remains embedded with `go:embed`, so `novelforge server` still ships as one executable.

The Web workspace is additive. It does not replace the TUI, Headless entry, deterministic Engine, checkpoint recovery, import/export or legacy `cmd/ainovel-cli` command.

## Build

```sh
cd web
npm ci
npm run check
npm test
npm run build
```

`web/package-lock.json` and `web/dist` are committed release inputs. CI rebuilds them from the locked graph and rejects drift. It also generates `docs/FRONTEND_DEPENDENCY_LICENSES.md` from the installed graph and rejects missing, unknown or prohibited license identifiers.

## Layout

The shell has four persistent regions:

- left navigation for real workspace routes;
- the central page workspace;
- an AI Inspector that displays only connected capabilities;
- a Task / Log strip driven by the durable SSE stream.

The shell is responsive, keyboard reachable and supports light and dark themes. Theme preference is the only value stored in browser storage. Projects, requests, events and all future job state remain authoritative on the local server.

## Pages available in Phase 3

- **Dashboard** reads health and project metrics.
- **Projects** lists, filters, archives, restores, duplicates and moves projects to the safe workspace trash.
- **New Novel** is a six-step wizard that creates a real project and stores a non-secret Foundation request.
- **Chapters** lists bounded chapter metadata read from the selected project.
- **Models** reads the runtime model registry.
- **Logs** consumes durable and replayed SSE events.
- **Settings** reads path-free, credential-free server settings and capabilities.

Later phases add quality, Truth/Ledger and version workflows only after their production APIs exist. The interface does not display unsupported fake actions.

## API client

Components do not call `fetch` directly. `web/src/lib/api.ts` owns:

- same-origin requests;
- JSON decoding;
- the common error envelope;
- opaque project IDs;
- automatic `Idempotency-Key` generation for every write;
- typed request and response contracts.

The server provides:

```text
GET  /api/models
GET  /api/settings
GET  /api/projects/{id}/chapters
GET  /api/projects/{id}/foundation
POST /api/projects/{id}/foundation
```

The Foundation write stores a validated request and emits `foundation.requested`. It explicitly returns `worker_available=false`; it does not claim that the Phase 9 worker has started.

## SSE behavior

The client opens one process-wide `EventSource`. It exposes `connecting`, `connected`, `reconnecting` and `unavailable` states and keeps only the latest 100 decoded events in memory. The server remains the replay authority through its durable event repository and `Last-Event-ID` support.

## Security boundary

- Provider credentials are never returned by the Web settings or model endpoints.
- API keys are not stored in `localStorage` or embedded JavaScript.
- Foundation requests reject credential-looking keys and values before persistence.
- Project paths remain opaque or workspace-relative; settings expose only the workspace label.
- Chapter inspection skips symlinks and reads at most 1 MiB per file.
- The existing restrictive CSP works because production JavaScript and CSS are self-hosted files with no inline executable script.

## Phase 6 Narrative Ledger pages

Phase 6 adds real **Foreshadows** and **Secrets** routes to the embedded workspace. Both pages load projects and Chapter-N state through the typed API client, use server-generated authority as the source of truth, disable writes while pending, display structured error codes and trace IDs, and reload after each successful mutation.

The Foreshadows page exposes computed OVERDUE metrics, stable filtered lists, creation, progress, resolve, and abandon operations. The Secrets page deliberately separates the management-only authority truth from Chapter-N holder ranges and public status; it supports private creation, temporal holder addition, and explicit public reveal.

Component tests cover empty and structured-error states, idempotent writes, pending-state button protection, lifecycle transitions, temporal holder writes, public reveal, and authoritative refresh. `web/dist` is rebuilt from the committed lockfile and remains a CI-checked `go:embed` release input.

## Phase 8 Chapter Versions workspace

Phase 8 adds a real **Versions** route alongside Chapters. It is backed only by the ChapterVersion REST API and does not keep an independent browser version database. The route accepts `project` and `chapter` query parameters and reloads authoritative server state after every successful write.

The workspace includes:

- immutable version History with version number, type, SHA, Active Final and Rejected badges;
- a Markdown editor/reader that loads the selected immutable version;
- **Save Human Revision**, which always appends a `human_revision` and never overwrites Active Final;
- **Check**, **Accept**, **Reject**, **Restore as New Version** and **Finalize** controls mapped to the corresponding idempotent server operations;
- an Inspector for Active Final, Continuity, conflict count, derived-state/rebuild status, parent version, SHA and authority;
- downstream Plan Impact records;
- Inline and Side-by-side bounded Diff with deterministic next-cursor support;
- an External Edit warning showing expected and observed SHA plus an explicit **Sync External Edit** action.

The UI never treats the textarea as authority. After save/sync/finalize it enters a pending state, then reloads History and chapter state from the server. Component tests verify that saving a human edit posts to the collection endpoint with a fresh `Idempotency-Key`, creates a human revision, and leaves the displayed Active Final unchanged until Finalize.

External modification is intentionally noisy. If the chapter file SHA differs from the immutable Active Final SHA, the workspace displays `Sync required`; it does not silently choose either the file or database value. Sync submits the observed SHA to protect against time-of-check/time-of-use changes and the server re-runs Librarian/Continuity/Truth-conflict evaluation before the revision can be accepted.

The Phase 8 client uses `web/src/lib/chapterVersions.ts`. It normalizes the Go transport names (`parent_version`, rebuild `status`) only for compatibility with the current component view model and assembles a display evaluation from the persisted sync response; the canonical public contract remains the OpenAPI schema.

No new frontend dependency is introduced by Phase 8. The same `npm ci`, Svelte/TypeScript check, Vitest suite, production build, vulnerability audit, dependency-license inventory and committed `web/dist` drift gate remain mandatory before merge. See [CHAPTER_VERSIONS.md](CHAPTER_VERSIONS.md).
