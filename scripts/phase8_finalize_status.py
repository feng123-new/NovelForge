from pathlib import Path

status_path = Path("docs/IMPLEMENTATION_STATUS.md")
status = status_path.read_text(encoding="utf-8")

old_verified = "- Phase 8 acceptance details are recorded in `docs/PHASE_08_ACCEPTANCE.md`; the acceptance-record PR itself must pass exact-head and merge-triggered `main` CI before Phase 9 branch creation."
new_verified = "\n".join([
    "- Phase 8 acceptance record PR #31 exact head `300e0e944069b3e902c19514503cdea5505c852c` passed CI `33747489810`; squash merge `8cc2759bd44d554b9c259531192fba120f38e5da` passed `main` CI `33747741718`.",
    "- Phase 8 final closure PR #32 exact head `bb7a532bf00fad371b6eedb010ef388c8c324bfc` passed CI `33748179603`; squash merge `2718a0728321328e51cfc6773c5f8fd40178f908` passed final closure `main` CI `33748407175`.",
    "- `feature/phase-09-autopilot` was created only after Phase 8 closure CI succeeded and contains no Phase 9 product implementation at handoff.",
])
if old_verified not in status:
    raise SystemExit("verified-state Phase 8 acceptance line not found")
status = status.replace(old_verified, new_verified, 1)

old_phase9 = "| 9–13 | not started | Phase 9 begins only after the Phase 8 acceptance-record PR and its final `main` CI succeed. |"
new_phase9 = "| 9–13 | not started | Phase 8 closure is complete; `feature/phase-09-autopilot` exists at the accepted handoff with no Phase 9 implementation yet. |"
if old_phase9 not in status:
    raise SystemExit("Phase 9 status row not found")
status = status.replace(old_phase9, new_phase9, 1)

old_evidence = "| Phase 8 production PR #30 | `dfc0c44ede7bfc59b1b56bd23e88643006f3ac62` | `33745966937` — success | `502cbf06611515484cb4bad2b2418b717e92f6b3` | `33746266244` — success |"
new_evidence = "\n".join([
    old_evidence,
    "| Phase 8 acceptance record PR #31 | `300e0e944069b3e902c19514503cdea5505c852c` | `33747489810` — success | `8cc2759bd44d554b9c259531192fba120f38e5da` | `33747741718` — success |",
    "| Phase 8 final closure PR #32 | `bb7a532bf00fad371b6eedb010ef388c8c324bfc` | `33748179603` — success | `2718a0728321328e51cfc6773c5f8fd40178f908` | `33748407175` — success |",
])
if old_evidence not in status:
    raise SystemExit("Phase 8 evidence row not found")
status = status.replace(old_evidence, new_evidence, 1)

old_resume = """## Exact resume point

1. Merge the Phase 8 acceptance-record PR only after its exact-head CI succeeds.
2. Verify the acceptance merge-triggered `main` CI succeeds across Go, Frontend, Windows and Docker.
3. Confirm no Phase 8 production or acceptance PR remains open and no temporary delivery helper is present in accepted `main`.
4. Create `feature/phase-09-autopilot` from that exact accepted `main` SHA.
5. Do not implement Phase 9 in the Phase 8 acceptance delivery."""
new_resume = """## Exact resume point

1. Use the existing `feature/phase-09-autopilot` branch for Phase 9 product work.
2. Before the first Phase 9 change, verify that branch matches the current accepted `main`; fast-forward it across any later documentation-only closure commit.
3. Preserve Phase 8 Scenario B, crash-recovery, race, OpenAPI, Windows, Docker, dependency-license and generated-asset gates throughout Phase 9.
4. Treat Phase 8 as closed except for an independently reviewed regression fix with its own exact-head and merged-main CI evidence.
5. Do not mix Phase 8 acceptance-history edits with Phase 9 product implementation."""
if old_resume not in status:
    raise SystemExit("old exact resume block not found")
status = status.replace(old_resume, new_resume, 1)
status_path.write_text(status, encoding="utf-8")

acceptance_path = Path("docs/PHASE_08_ACCEPTANCE.md")
acceptance = acceptance_path.read_text(encoding="utf-8")

anchor = """PR #31 changed only `docs/IMPLEMENTATION_STATUS.md` and this evidence file. Its exact Head passed Go, Frontend, Windows, and Docker, and the resulting `main` commit passed the same four-job workflow. Phase 8 is therefore formally accepted; Phase 9 may begin only from an accepted `main` descendant that preserves these gates.

## Delivered production behavior"""
replacement = """PR #31 changed only `docs/IMPLEMENTATION_STATUS.md` and this evidence file. Its exact Head passed Go, Frontend, Windows, and Docker, and the resulting `main` commit passed the same four-job workflow.

## Final closure correction

| Evidence | Value |
| --- | --- |
| Closure correction PR | #32 — `docs: finalize Phase 8 acceptance closure` |
| Exact green closure head | `bb7a532bf00fad371b6eedb010ef388c8c324bfc` |
| Exact-head closure CI | `33748179603` — success |
| Closure squash merge | `2718a0728321328e51cfc6773c5f8fd40178f908` |
| Final closure `main` CI | `33748407175` — success |

PR #32 changed only this evidence file, removed the stale pending label, passed all four exact-head jobs, and its merge passed all four `main` jobs. Phase 8 is therefore formally and internally consistently accepted.

## Delivered production behavior"""
if anchor not in acceptance:
    raise SystemExit("acceptance closure anchor not found")
acceptance = acceptance.replace(anchor, replacement, 1)

old_result = "Phase 8 is complete. Production PR #30 and acceptance PR #31 both passed exact-head CI, both were squash-merged, and both merge-triggered `main` workflows succeeded. `docs/IMPLEMENTATION_STATUS.md` marks Phase 8 complete and records Phase 9 as not started. The next permitted branch is `feature/phase-09-autopilot`, created from the final accepted `main` after repository hygiene verification."
new_result = "Phase 8 is complete. Production PR #30, acceptance record PR #31 and final closure correction PR #32 each passed exact-head CI, were squash-merged, and passed merge-triggered `main` CI. `docs/IMPLEMENTATION_STATUS.md` marks Phase 8 complete and Phase 9 not started. `feature/phase-09-autopilot` was created only after final closure CI and contains no Phase 9 product implementation at handoff."
if old_result not in acceptance:
    raise SystemExit("acceptance result paragraph not found")
acceptance = acceptance.replace(old_result, new_result, 1)
acceptance_path.write_text(acceptance, encoding="utf-8")
