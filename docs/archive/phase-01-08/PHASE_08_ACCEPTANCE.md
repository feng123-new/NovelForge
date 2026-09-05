# Phase 8 Acceptance Evidence

Status: **complete**

Date: 2026-09-03

## Production delivery

| Evidence | Value |
| --- | --- |
| Production PR | #30 — `feat: add chapter revision workflow` |
| Exact green PR head | `dfc0c44ede7bfc59b1b56bd23e88643006f3ac62` |
| Exact-head PR CI | `33745966937` — success |
| Squash merge | `502cbf06611515484cb4bad2b2418b717e92f6b3` |
| Merge-triggered `main` CI | `33746266244` — success |
| Project migration | 6 — `chapter_versions_human_sync` |

The exact production PR head passed all required jobs before merge: Go lint/test/build, Windows test/build, Frontend check/test/build, and Docker build. The Go job also passed the dedicated Phase 8 Scenario B, targeted race tests including `internal/chapterversion/...`, the Truth Store 100,000-fact temporal-index gate, OpenAPI route/schema validation, `CGO_ENABLED=0` build, module/dependency-license checks, embedded Web asset validation, lifecycle smoke, install/upgrade/uninstall smoke, and brand audit. The Frontend job passed Svelte/TypeScript checks, Vitest, production build, high-severity audit, dependency-license inventory, and committed build-drift verification.

The squash merge was independently revalidated on `main` by CI run `33746266244`, where Go, Windows, Frontend, and Docker all succeeded.

## Formal acceptance closure

| Evidence | Value |
| --- | --- |
| Acceptance record PR | #31 — `docs: record Phase 8 acceptance evidence` |
| Exact green acceptance head | `300e0e944069b3e902c19514503cdea5505c852c` |
| Exact-head acceptance CI | `33747489810` — success |
| Acceptance squash merge | `8cc2759bd44d554b9c259531192fba120f38e5da` |
| Acceptance merge-triggered `main` CI | `33747741718` — success |

PR #31 changed only `docs/IMPLEMENTATION_STATUS.md` and this evidence file. Its exact Head passed Go, Frontend, Windows, and Docker, and the resulting `main` commit passed the same four-job workflow.

## Final closure correction

| Evidence | Value |
| --- | --- |
| Closure correction PR | #32 — `docs: finalize Phase 8 acceptance closure` |
| Exact green closure head | `bb7a532bf00fad371b6eedb010ef388c8c324bfc` |
| Exact-head closure CI | `33748179603` — success |
| Closure squash merge | `2718a0728321328e51cfc6773c5f8fd40178f908` |
| Final closure `main` CI | `33748407175` — success |

PR #32 changed only this evidence file, removed the stale pending label, passed all four exact-head jobs, and its merge passed all four `main` jobs. Phase 8 is therefore formally and internally consistently accepted.

## Delivered production behavior

Phase 8 establishes immutable chapter revision history as the authority boundary between generated or human prose and accepted chapter state.

- Migration 6 stores immutable `ChapterVersion` rows, append-only review/audit events, monotonic per-chapter version numbers, parent-version lineage, one Active Final projection, external-SHA state, idempotent operations, recoverable Finalize sagas, rebuild records, checkpoints, and downstream plan-impact records.
- Saving in the Web editor appends a `human_revision`; it never overwrites the Active Final.
- Restore creates a new immutable version instead of mutating history.
- Reject retains the version and reason in History.
- Check/Sync re-run Librarian proposal, Continuity and Truth-conflict evaluation without granting those semantic services write authority.
- Accept records explicit approval but does not commit Truth.
- Finalize is the only operation that creates/switches an authoritative Final and commits the accepted proposal.
- Accepted Human Final is the highest authority and cannot be silently downgraded by a later generated Final.
- External chapter-file edits are detected by normalized SHA mismatch and require explicit sync; time-of-check/time-of-use changes fail closed.
- Human Final corrections append explicit Truth supersede/assert events and retain the prior generated events for audit.
- Narrative Ledger changes are committed through the accepted-final boundary and promoted to Human Final authority without weakening Secret holder/reveal Chapter-N visibility.
- Derived Truth/Context state rebuild begins at edited Chapter N and does not contaminate Chapter N-1 or earlier.
- Downstream changed assumptions are emitted as bounded `chapter_plan_impacts` rather than silently rewriting historical plans.
- Diff supports deterministic bounded `inline` and `side_by_side` modes with cursor paging and hard byte/line/work ceilings.
- The embedded Svelte workspace exposes real History, Markdown editor/reader, Human Revision save, Check, Accept, Reject, Restore, Finalize, external-sync warning/action, Inspector, plan impact and Diff controls backed by the production REST API.
- The Phase 8 OpenAPI extension is merged into `/api/openapi.json` and CI-validated against the registered transport contract.

## Scenario B

The permanent Scenario B test covers the Chapter 50 human correction contract:

1. generated Truth states Character A died;
2. the authoritative chapter file initially matches the generated Active Final;
3. an external edit changes the chapter so Character A is severely injured, escapes and remains alive;
4. SHA mismatch sets `sync_required` without replacing the Active Final;
5. explicit sync creates/reuses an immutable `human_revision` parented to the previous Final;
6. deterministic Librarian/Continuity evaluation exposes the old death as a conflict;
7. explicit Accept + Finalize creates a new Human Final;
8. the old generated death event remains in immutable Truth history and is explicitly superseded by `human_final` authority;
9. `alive=true`, `injury="severe"` and `escaped=true` are projected at Chapter 50;
10. Chapter 49 remains unchanged;
11. Chapter 50 rebuild completes and Chapter 51 plan-impact records are emitted;
12. repeated Sync/Finalize with the same idempotency identity does not duplicate versions, Truth events, checkpoints or Active Finals;
13. the final chapter-file SHA converges to the Active Human Final SHA and `sync_required` is cleared.

## Crash-recovery matrix

`internal/chapterversion/recovery_test.go` injects one failure at each recoverable boundary:

```text
after_version_ready
after_truth_commit
after_ledger_commit
after_chapter_file_write
after_active_final_switch
before_checkpoint
after_checkpoint
```

Retrying with the same idempotency identity must converge to the same completed Human Final with one checkpoint and one Active Final, without duplicating authoritative Truth. Finalize only short-circuits across a different key when a completed saga and checkpoint prove completion; merely observing an Active Final is not considered sufficient recovery evidence.

## Web and dependency evidence

Phase 8 adds no new npm or Go dependency. `web/dist/assets/app.css` and `web/dist/assets/app.js` are the exact deterministic Vite output committed from the locked dependency graph. The final exact-head Frontend CI passed the build-drift and frontend dependency-license gates, and `npm audit --audit-level=high` reported no high-severity blocker.

## Repository hygiene

The production PR was the only open Phase 8 production PR. A temporary artifact-sync workflow used during delivery was removed before the final acceptance head and therefore was not merged into `main`. The accepted production tree contains the normal permanent `.github/workflows/ci.yml` only for Phase 8 CI hardening; no recovery helper, probe, payload fragment, source generator or self-modifying finalizer was merged.

At formal acceptance closure there were no open Pull Requests. The acceptance branch contained only the two intended documentation changes at merge time. The accepted `main` contained no temporary Phase 8 workflow or status-generation script.

## Acceptance result

Phase 8 is complete. Production PR #30, acceptance record PR #31 and final closure correction PR #32 each passed exact-head CI, were squash-merged, and passed merge-triggered `main` CI. `docs/IMPLEMENTATION_STATUS.md` marks Phase 8 complete and Phase 9 not started. `feature/phase-09-autopilot` was created only after final closure CI and contains no Phase 9 product implementation at handoff.
