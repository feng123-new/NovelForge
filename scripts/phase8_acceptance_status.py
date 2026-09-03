from pathlib import Path

path = Path("docs/IMPLEMENTATION_STATUS.md")
text = path.read_text(encoding="utf-8")

anchor = "- Phase 7 Context Compiler and Hybrid Retrieval was delivered by PR #28 from exact head `b5600f21cbb91180aab8faed75865734dc515739`; PR CI `33716621992` succeeded, squash merge `30a91deac2dbe1add0ba8353c05f89401a53b108` passed merge-triggered main CI `33716865357`."
phase8_state = "\n".join(
    [
        "- Phase 8 Chapter Version and Human Edit Sync was delivered by PR #30 from exact head `dfc0c44ede7bfc59b1b56bd23e88643006f3ac62`.",
        "- Phase 8 production PR CI `33745966937` succeeded; squash merge `502cbf06611515484cb4bad2b2418b717e92f6b3` passed merge-triggered `main` CI `33746266244` across Go, Frontend, Windows and Docker.",
        "- Phase 8 acceptance details are recorded in `docs/PHASE_08_ACCEPTANCE.md`; the acceptance-record PR itself must pass exact-head and merge-triggered `main` CI before Phase 9 branch creation.",
    ]
)
if phase8_state.splitlines()[0] not in text:
    if anchor not in text:
        raise SystemExit("Verified repository state anchor not found")
    text = text.replace(anchor, anchor + "\n" + phase8_state, 1)

old_status = "| 8–13 | not started | Phase 8 begins from the accepted Phase 7 `main`. |"
new_status = "\n".join(
    [
        "| 8 — Chapter Version and Human Edit Sync | complete | PR #30 exact-head CI `33745966937`, squash merge `502cbf06611515484cb4bad2b2418b717e92f6b3`, and merged-main CI `33746266244` passed; acceptance evidence is recorded separately. |",
        "| 9–13 | not started | Phase 9 begins only after the Phase 8 acceptance-record PR and its final `main` CI succeed. |",
    ]
)
if old_status not in text:
    raise SystemExit("Phase status anchor not found")
text = text.replace(old_status, new_status, 1)

marker = "\n## Next phase\n"
if marker not in text:
    raise SystemExit("Next phase marker not found")
prefix = text.split(marker, 1)[0].rstrip()
phase8 = r'''

## Phase 8 — Chapter Version and Human Edit Sync

### Completed

- Added project Migration 6 and `internal/chapterversion` as the immutable chapter-revision authority boundary.
- Added monotonic per-chapter version numbers, parent lineage, append-only review/audit events, one Active Final projection, idempotent operations, external-SHA state, recoverable Finalize sagas, rebuild records, checkpoints and downstream plan impacts.
- Added Human Revision save, Check, Accept, Reject, Restore-as-new-version, Finalize and explicit external-file Sync without silent overwrite of Active Final.
- Added Human Final authority above generated final, explicit Truth supersede/assert history, Narrative Ledger Human Final promotion and Chapter-N derived-state rebuild.
- Added bounded deterministic inline/side-by-side Diff with cursor pagination and hard byte/line/work ceilings.
- Added the real Versions Web workspace and a Phase 8 OpenAPI extension merged into `/api/openapi.json`.
- Fixed SQLite single-connection cursor/write deadlocks by materializing rows before nested decoration or FTS writes.
- Hardened Finalize recovery so an Active Final alone cannot skip unfinished rebuild/checkpoint work; completed replay requires persisted saga/checkpoint evidence.

### Migration 6

Migration `chapter_versions_human_sync` adds immutable `chapter_versions`, `chapter_version_events`, `chapter_active_finals`, `chapter_revision_operations`, `chapter_external_state`, `derived_state_rebuilds`, `chapter_plan_impacts`, `chapter_finalize_sagas`, `chapter_version_checkpoints`, and the per-chapter version counter migration. Immutable history is protected by database triggers; the Active Final is a separate mutable projection.

### Authority and synchronization decisions

- Browser/editor save creates `human_revision`; it is not a Truth write.
- External files are compared by normalized SHA against the immutable Active Final and require explicit sync on mismatch.
- Librarian, Continuity and Editor may evaluate a revision but cannot promote authority.
- Accept records approval without committing Truth.
- Finalize alone may create/switch a Final and commit the accepted proposal.
- Accepted Human Final is the highest authority and cannot be silently downgraded by generated output.
- Human corrections append explicit supersede/assert Truth events; previous generated events remain auditable.
- Rebuild begins at edited Chapter N; Chapter N-1 and earlier remain unchanged.
- Downstream changed assumptions are recorded as plan impacts rather than silently rewriting historical plans.

### Web and API

Phase 8 adds authoritative chapter-version state, list/get/create, Diff, Check, Restore, Accept, Reject, Finalize, Sync Status, Sync, Rebuild and Plan Impact routes. All writes use `Idempotency-Key` and the common safe error envelope. Web History, Markdown editor/reader, Inspector, external edit warning, plan impact and Diff controls reload server authority after writes. No new frontend dependency was added.

### Scenario B and recovery tests

Permanent Scenario B verifies a Chapter 50 correction from generated death to severe injury, escape and survival: SHA mismatch is detected, sync creates a human revision without replacing the old Final, the generated death remains in Truth history, Human Final explicitly supersedes it, Chapter 49 remains unchanged, Chapter 50+ projects the correction, Chapter 51 plan impacts are emitted, file SHA converges to the Active Human Final and repeated operations remain idempotent.

The Finalize recovery matrix injects one failure after version creation, Truth commit, Ledger commit, chapter-file write, Active Final switch, before checkpoint and after checkpoint. Same-key retry must converge to one completed Human Final, one Active Final and one checkpoint without duplicate authoritative Truth.

### Acceptance evidence

| Delivery | Exact head | PR CI | Merge | Main CI |
| --- | --- | --- | --- | --- |
| Phase 8 production PR #30 | `dfc0c44ede7bfc59b1b56bd23e88643006f3ac62` | `33745966937` — success | `502cbf06611515484cb4bad2b2418b717e92f6b3` | `33746266244` — success |

Full evidence and the acceptance rule are in `docs/PHASE_08_ACCEPTANCE.md`.

### Repository hygiene

PR #30 was the single Phase 8 production PR. The temporary workflow used to recover exact CI-generated Web assets was deleted before the final green production head and was not merged. The accepted production tree contains no Phase-specific recovery workflow, payload fragment, probe, source generator or self-modifying finalizer.

### Known issues

No Phase 8 data-integrity, idempotency, crash-recovery, Scenario B, OpenAPI, Web build-drift, race, Windows, Docker, dependency-license, lifecycle or generated-asset blocker remains in the production merge. Phase 9 durable Autopilot work has not started.

## Next phase

```text
Phase 9 — Durable Autopilot
feature/phase-09-autopilot
```

## Exact resume point

1. Merge the Phase 8 acceptance-record PR only after its exact-head CI succeeds.
2. Verify the acceptance merge-triggered `main` CI succeeds across Go, Frontend, Windows and Docker.
3. Confirm no Phase 8 production or acceptance PR remains open and no temporary delivery helper is present in accepted `main`.
4. Create `feature/phase-09-autopilot` from that exact accepted `main` SHA.
5. Do not implement Phase 9 in the Phase 8 acceptance delivery.
'''

path.write_text(prefix + phase8, encoding="utf-8")
