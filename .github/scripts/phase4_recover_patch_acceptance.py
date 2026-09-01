#!/usr/bin/env python3
from pathlib import Path
import sys

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")
needle = '''    path.write_text(text, encoding="utf-8")


def patch_docs() -> None:'''
replacement = r'''    if "phase4-accept:" not in text:
        text += r'''

  phase4-accept:
    name: Phase 4 acceptance and merge
    if: >-
      github.event_name == 'pull_request' &&
      github.event.pull_request.number == 10 &&
      github.event.pull_request.head.repo.full_name == github.repository &&
      github.event.pull_request.head.ref == 'feature/phase-04-truth-store'
    needs: [lint-test-build, frontend, windows, docker]
    runs-on: ubuntu-latest
    permissions:
      actions: write
      contents: write
      pull-requests: write
    env:
      GH_TOKEN: ${{ github.token }}
      REPO: ${{ github.repository }}
      PR_NUMBER: ${{ github.event.pull_request.number }}
      EXPECTED_HEAD: ${{ github.event.pull_request.head.sha }}
    steps:
      - name: Validate clean Phase 4 delivery
        run: |
          current_head="$(gh api "repos/$REPO/pulls/$PR_NUMBER" --jq .head.sha)"
          test "$current_head" = "$EXPECTED_HEAD"
          changed="$(gh api --paginate "repos/$REPO/pulls/$PR_NUMBER/files?per_page=100" --jq '.[].filename')"
          if printf '%s\n' "$changed" | grep -E '^\.github/(scripts/phase4_recover|workflows/phase4-recover)'; then
            echo "temporary recovery files remain in the pull request"
            exit 1
          fi
          for required in \
            internal/truthstore/model.go \
            internal/truthstore/migration.go \
            internal/truthstore/store.go \
            internal/truthstore/rebuild.go \
            internal/server/truth.go \
            internal/server/openapi.json \
            docs/TRUTH_STORE.md; do
            printf '%s\n' "$changed" | grep -Fx "$required" >/dev/null
          done

      - name: Mark ready and squash merge
        id: merge
        run: |
          if test "$(gh api "repos/$REPO/pulls/$PR_NUMBER" --jq .draft)" = "true"; then
            gh pr ready "$PR_NUMBER" --repo "$REPO"
          fi
          response="$(gh api -X PUT "repos/$REPO/pulls/$PR_NUMBER/merge" \
            -f merge_method=squash \
            -f sha="$EXPECTED_HEAD" \
            -f commit_title='feat: add structured truth store (#10)' \
            -f commit_message='Complete Phase 4 with append-only temporal truth events, deterministic projections, authority-aware conflicts, Chapter-N knowledge boundaries, provenance, explicit supersede/retract, bounded rebuild, verification, APIs, concurrency tests, and a 100,000-fact index gate.')"
          test "$(printf '%s' "$response" | jq -r .merged)" = "true"
          merge_sha="$(printf '%s' "$response" | jq -r .sha)"
          test -n "$merge_sha"
          test "$merge_sha" != "null"
          printf 'merge_sha=%s\n' "$merge_sha" >> "$GITHUB_OUTPUT"

      - name: Dispatch and verify merged main
        env:
          MERGE_SHA: ${{ steps.merge.outputs.merge_sha }}
        run: |
          dispatched=false
          for attempt in $(seq 1 20); do
            if gh workflow run ci.yml --repo "$REPO" --ref main; then
              dispatched=true
              break
            fi
            sleep 3
          done
          test "$dispatched" = "true"

          run_id=""
          for attempt in $(seq 1 180); do
            run_id="$(gh api --method GET "repos/$REPO/actions/runs" \
              -f branch=main -f event=workflow_dispatch -F per_page=100 \
              --jq ".workflow_runs[] | select(.name == \"CI\" and .head_sha == \"$MERGE_SHA\") | .id" | head -n 1)"
            if test -n "$run_id"; then
              state="$(gh api "repos/$REPO/actions/runs/$run_id" --jq '[.status, (.conclusion // "")] | join("|")')"
              case "$state" in
                completed\|success) break ;;
                completed\|*) echo "merged-main CI failed: $state"; exit 1 ;;
              esac
            fi
            sleep 10
          done
          test -n "$run_id"
          test "$(gh api "repos/$REPO/actions/runs/$run_id" --jq .conclusion)" = "success"

          for expected in \
            'Go lint, test and build' \
            'Frontend check, test and build' \
            'Windows test and build' \
            'Docker build'; do
            conclusion="$(gh api "repos/$REPO/actions/runs/$run_id/jobs?per_page=100" \
              --jq ".jobs[] | select(.name == \"$expected\") | .conclusion")"
            test "$conclusion" = "success"
          done

          gh pr comment "$PR_NUMBER" --repo "$REPO" --body "Phase 4 acceptance complete. Squash merge: \`$MERGE_SHA\`. Merge-triggered equivalent main validation was explicitly dispatched as Actions run \`$run_id\`; Go, Frontend, Windows, and Docker jobs all passed."
'''
    path.write_text(text, encoding="utf-8")


def patch_docs() -> None:'''
if needle not in text:
    raise RuntimeError("cannot locate generator CI patch terminator")
text = text.replace(needle, replacement, 1)
path.write_text(text, encoding="utf-8")
