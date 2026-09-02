# NovelForge Web Workspace

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

Chapter editing, Truth views, version/diff controls and durable Autopilot controls belong to later phases. The interface does not display those actions before their production APIs exist.

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
