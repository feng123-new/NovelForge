# Phase 9 — Durable Autopilot

Phase 9 is reopened by the user's explicit request. Phase 10–13 remain paused. This document describes the production implementation; verification evidence and exact merged revision belong in the delivery PR. No full-suite or 300-chapter result is implied by a short-flow check.

## Runtime and authority

The normal `novelforge server` entry enables a single durable worker per workspace. Embedders opt in with `server.Config.AutopilotEnabled`. Disabled/failed workers are reported unavailable; clients do not infer readiness from model catalog entries.

The worker uses the existing **deterministic Chapter Engine** (`qualitygate.Coordinator`) and the Phase 8 ChapterVersion Final bridge. It intentionally does not start a second legacy Host writing loop. The retained `engineadapter.LegacyAdapter` still serves the original Host entry; it is not a substitute for the newer Truth/Ledger/Version workflow. The worker has real model and coordinator calls, not a state-only placeholder.

```text
snapshot saved Foundation request and bounded target range
  -> generate/reuse project Foundation (planning only)
  -> collect Chapter-N/POV context
  -> persist a strict ChapterPlan
  -> existing Generate / Check / bounded Rewrite
  -> optional explicit human approval before Final
  -> immutable Final / Truth / Ledger / file / index / checkpoint
  -> advance durable chapter cursor
```

Writer and Librarian never receive authority to write Truth. Plans and Foundation remain lower-authority planning artifacts. Existing human Finals and unversioned prose cannot be overwritten automatically. A blocking continuity failure or exhausted rewrite budget stops progression with retained artifacts. An incomplete Final saga is replayed with the same key; an Active Final pointer alone is not considered completed work.

## Persistence and ownership

Workspace Migration 3 adds `autopilot_jobs` to `.novelforge/server.db`. A unique partial index permits one unfinished job per project, including paused and failed jobs. The JSON cursor stores the immutable non-secret start request, generated Foundation, selected planning context, current plan, retries, review approval and next boundary. Task views/SSE omit this internal payload and expose only progress/actions/error codes.

Each task update and its `autopilot.changed` event are committed in the **same SQLite transaction**. Live non-blocking SSE fan-out occurs after commit; reconnects replay the durable event. The browser also refreshes server state, rather than treating an optimistic button click as completion.

An OS lock on `autopilot-worker.lock` admits only one worker per workspace. The worker and Web writes acquire the same `.ainovel.lock` used by the retained TUI/Headless Host. Locks release on process death; there is no heartbeat-only lease that allows an old paused process and a new worker to write simultaneously. Read requests remain available. Write operations require a paused/stopped task and an available project lock.

Foundation output is saved at `<project>/.novelforge/foundation-output.json`, keyed to the saved request ID, and reused by subsequent jobs using that request. It is not a user configuration file or a Truth projection. New requests are snapshotted; editing a request after enqueue does not change a running job's inputs. Model credentials are loaded from server/project configuration, never stored in job views/events.

## State and control semantics

States: `pending`, `running`, `paused`, `retrying`, `failed`, `completed`, `cancelled`.

- **Start** queues a finite inclusive chapter range (maximum target chapter 1000). The saved Foundation request is required. Per-role model selections must refer to configured providers; blank wizard selections use project configuration.
- **Pause** records a control intent. A running step is allowed to exit its bounded operation before the state becomes `paused`; the UI displays the pending intent meanwhile. It does not claim an in-flight writer is already stopped.
- **Stop** similarly records intent, then enters terminal `cancelled`. It deletes no drafts, Finals or facts. A cancelled job cannot be resumed; a new explicit job can continue after resolving retained artifacts.
- **Resume** is valid for paused/failed jobs. For `REVIEW_REQUIRED`, it explicitly approves the current selected candidate, without bypassing continuity or version validation. Every-N review pauses before the matching candidate's Final commit; the last chapter also requires approval. Zero means full automatic; default is one.
- **Restart/shutdown** preserves the cursor. Only after acquiring the sole-worker lock are interrupted `running` rows recovered. Already persisted model results and Final commits replay through their existing idempotency boundaries.

Provider retries, job retries and chapter rewrites are separately bounded. Exhaustion is visible as `failed`; retry requires explicit user action. A checkpoint-storage failure stops the worker instead of continuing from uncertain progress. API errors/events use safe machine codes, never raw Provider errors or credentials.

A model provider may have completed a paid call before the local response was persisted when a process is killed. This unavoidable external-call gap is not advertised as exactly-once billing. The guarantee is replay of **persisted** results and idempotent authority commits.

## API

All writes require `Idempotency-Key` and reject unknown JSON fields.

```text
GET/POST /api/projects/{id}/autopilot
GET      /api/projects/{id}/autopilot/{job}
POST     /api/projects/{id}/autopilot/{job}/pause
POST     /api/projects/{id}/autopilot/{job}/stop
POST     /api/projects/{id}/autopilot/{job}/resume
```

Start fields: `start_chapter`, `target_chapter`, optional `review_every` (0–100), `max_rewrites` (0–5), `max_retries` (0–5). Omitted policies inherit the Foundation request/defaults. Collection reads are bounded to 100 jobs per page. Job identity is project-scoped, including all controls. Detail reads expose the selected candidate text for real human review, Foundation and the current plan; they do not expose model credentials.

New Novel saves the request without initiating a paid call. Its Autopilot link opens the task page; the user explicitly starts the finite job. Existing novel text must already be integrated through the accepted version workflow; the worker will stop with a sync/import requirement instead of silently adopting or overwriting raw files.

## Verification boundary

Targeted checks cover durable control intent, recovery, worker exclusion/shutdown, configured generation through the actual quality/version pipeline, two-chapter review and restart without repeating persisted calls, project isolation, protected writes, route contracts, and the frontend review controls. The real entry and changed frontend assets must build.

Full Go/frontend/race suites, platform matrices, paid models, and the historical 300-chapter simulation/1000-chapter performance targets are **not run under the current limited-check scope**. Foundation/planning creativity, general Provider compatibility, large casts and long-book quality retain those unverified limits. Phase 9 functional delivery must not be rewritten as full production-scale acceptance.
