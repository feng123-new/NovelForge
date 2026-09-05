# Phase 9 — Durable Autopilot

Phase 9 is reopened by the user's explicit request. Phase 11–13 remain paused. This document describes the production implementation; verification evidence and exact merged revision belong in the delivery PR. No full-suite or 300-chapter result is implied by a short-flow check.

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

An OS lock on `autopilot-worker.lock` admits only one worker per workspace. The worker and Web writes acquire the same `.ainovel.lock` used by the retained TUI/Headless Host. Locks release on process death; there is no heartbeat-only lease that allows an old paused process and a new worker to write simultaneously. Read requests remain available. Write operations require a paused/stopped task and an available project lock. Archiving/deleting requires an explicitly stopped or completed job, preventing orphaned tasks.

Foundation output is saved at `<project>/.novelforge/foundation-output.json`, keyed to the saved request ID, and reused by subsequent jobs using that request. It is not a user configuration file or a Truth projection. New requests are snapshotted; editing a request after enqueue does not change a running job's inputs. Model credentials are loaded from server/project configuration, never stored in job views/events.

## State and control semantics

States: `pending`, `running`, `paused`, `retrying`, `failed`, `completed`, `cancelled`.

- **Start** queues a finite inclusive chapter range (maximum target chapter 1000). The saved Foundation request is required. Per-role model selections must refer to configured providers; blank wizard selections use project configuration.
- **Pause** records a control intent. A running step is allowed to exit its bounded operation before the state becomes `paused`; the UI displays the pending intent meanwhile. It does not claim an in-flight writer is already stopped.
- **Stop** similarly records intent, then enters terminal `cancelled`. It deletes no drafts, Finals or facts. A cancelled job cannot be resumed; a new explicit job can continue after resolving retained artifacts.
- **Resume** is valid for paused/failed jobs. For `REVIEW_REQUIRED`, it explicitly approves the current selected candidate by its persisted candidate ID (a changed selection pauses for a new approval), without bypassing continuity or version validation. Every-N review pauses before the matching candidate's Final commit; the last chapter also requires approval. Zero means full automatic; default is one.
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

### Delivery evidence

Tested source: `3cf18bb1ed8393f794e0b6753baaaee6f9fbac6e`; source tree: `aff66ee85e2056d4d594f640c8fed79fcfed9f17`. GitHub Actions run `33962880165`, job `101297733300`, passed with Go 1.25.5 and Node 22. The final delivery PR may add documentation-only review; any such difference is checked separately and is not described as another full runtime test.

The configured two-chapter flow uses a temporary provider HTTP endpoint, not an injected quality service: it exercises project/server configuration, both new planning operations, the actual HTTP model adapter, persisted model-call replay, current context compilation, existing continuity/editor checks, human review and immutable Finalization, plus rebuilding the Server between chapters. Six named Go tests and one focused frontend test passed. The final executable was built again after the changed `web/dist` was generated.

Source whitespace checks and JavaScript syntax checks passed. Vite's exact generated JavaScript contains whitespace inside a string literal; that generated file is intentionally not rewritten by a whitespace trimmer. The first staging attempt stopped on that generic whitespace warning after its checks; no failing test was removed, no permanent CI configuration was weakened, and no product branch was published by that attempt.


## Phase 9 closure — 2026-09-05

PR #37 was merged at `1a96ab344a8d337af38da51bc080810a63d0a72d`. A conversation summary incorrectly said it was not delivered; repository history, not that summary, is the baseline. This follow-up closes specific orchestration gaps, without restarting Phase 11–13.

- STOP cannot be downgraded by a later PAUSE. Replayed no-op commands do not append new events or invalidate a displayed review revision. Each worker claim has a persisted generation, so an old attempt cannot checkpoint a new attempt. A step that returns no progress fails rather than spinning.
- REVIEW_REQUIRED resume needs `expected_revision` and `review_candidate_id` from the inspected detail response. Empty/stale approvals return 409. The UI requires inspecting the current revision before approval. Normal resume still accepts an empty object.
- Interrupted rewrites reconstruct the same previous draft, feedback and attempt. Persisted model results replay without changing the request identity or consuming a second rewrite slot.
- A finite batch does not shorten the novel's Foundation horizon. Book horizon is snapshotted independently. Foundations are size-bounded, validated on reuse and must have contiguous arc coverage. A saved Foundation that cannot cover an expanded book target stops with `FOUNDATION_HORIZON_MISMATCH`; create a new Foundation request rather than silently rewriting the existing plan.
- Planner uses its configured `planner` role and exactly the selected `planning_pov`. A different POV is rejected before Writer. Progress defaults come from contiguous completed version/saga/checkpoint proofs, not legacy summary counts.
- An opaque fingerprint of authoritative event positions and prior Finals binds the chapter plan. Changed accepted facts, Ledger or prior Finals stop old work with `CHAPTER_CONTEXT_CHANGED`. Fingerprints intentionally invalidate conservatively even for unrelated later authority edits; they are not story content. A migrated in-flight task lacking a baseline stops with `CONTEXT_BASELINE_REQUIRED`, not an invented current snapshot.
- Resolve a stale chapter through the existing explicit human version Check/Accept/Finalize path, then resume: a proved completed Human Final takes over the chapter without replacement by the older draft. An incomplete generated Final saga is replayed first with its original key. The system does not pretend that an old plan was automatically replanned or that an Active Final pointer proves commit completion.
- Project writes recheck the task under the project OS lock; archive/delete uses the live-slot query instead of a history page. Worker lock symlinks are rejected. No old migration is edited.

Verification remains named state/control, replay, short chapter flows, contract and UI checks plus entry and asset builds. Exact results and merge SHA are recorded in the closure PR. No full Go/Vitest/race suites, paid model calls, 300-chapter simulation or platform/scale matrix is claimed. Phase 9 is a bounded functional delivery; those explicitly deferred acceptance scopes remain unexecuted.

A newly started task encountering an existing unfinished chapter transaction stops with `EXISTING_DRAFT_REQUIRES_REVIEW`: resume the original paused/failed task, or explicitly review/finalize the retained content. It does not bind a newly generated plan to another task's old draft. Completed Human Finals can take over at planning or generation stages; a pending rebuild cannot satisfy Final completion. These are conservative recovery boundaries, not automatic re-planning or implicit acceptance.

Closure build and the initial 18 named Go / 3 frontend tests passed in Actions `33965208527` on product commit `39e67336d5615066cf5180309475aa79ff468af5`. Follow-up takeover checks are recorded with their exact final product in the delivery PR; this is not full-suite or scale acceptance.
