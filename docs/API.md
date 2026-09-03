# API

The canonical machine-readable contract is `internal/server/openapi.json` plus its static phase extensions, exposed as OpenAPI 3.1 at `GET /api/openapi.json`.

## Chapter quality transaction

```text
POST /api/projects/{id}/chapters/{chapter}/generate
POST /api/projects/{id}/chapters/{chapter}/check
POST /api/projects/{id}/chapters/{chapter}/rewrite
POST /api/projects/{id}/chapters/{chapter}/finalize
GET  /api/projects/{id}/chapters/{chapter}/quality
GET  /api/projects/{id}/chapters/{chapter}/candidates
```

`generate` and `rewrite` accept strict `ChapterPlan` JSON. `check` and `finalize` accept an empty object. Every write requires `Idempotency-Key`, is subject to the global 1 MiB body limit, validates the opaque project boundary, and returns the common error envelope containing a safe code/message and `trace_id`.

Quality reads return metadata only. Candidate prose is omitted so the diagnostics surface cannot accidentally become a full-book prompt or content export endpoint.

Typical state-bearing response:

```json
{
  "snapshot": {
    "transaction": {
      "state": "continuity_warn",
      "attempt": 1,
      "max_rewrites": 2,
      "quality_threshold": 7
    },
    "candidates": [],
    "continuity": {"status":"WARN","blocking":false,"issues":[]},
    "editor": {"score":8.1}
  },
  "actions": {
    "generate": false,
    "check": true,
    "rewrite": false,
    "finalize": false
  }
}
```

The `actions` object is derived from server state and configured production services; the Web client does not duplicate the quality state machine.

## Truth authority

Truth APIs remain documented in [PROJECT_API.md](PROJECT_API.md) and [TRUTH_STORE.md](TRUTH_STORE.md). Phase 5 Finalize converts only the accepted candidate's persisted Fact Proposal into idempotent `generated_final` Truth events.

## Narrative Ledger API (Phase 6)

The project-scoped Foreshadow and Secret routes are documented in the embedded OpenAPI 3.1 document. Foreshadow reads accept `chapter`, status/overdue/importance/urgency/arc/entity/query filters and bounded pagination. Secret reads accept `chapter`, public/holder/query filters and an explicit administrative `include_truth` flag. Dashboard, diagnostics, and Planner Context are read-only Chapter-N views.

All Narrative Ledger writes require `Idempotency-Key`, strict JSON, project boundary validation, and the common safe error envelope. Generic PATCH operations implement lifecycle actions through validated state changes rather than transport-side state logic. Holder add/close operations preserve effective chapter ranges and provenance.

## ChapterVersion and human-edit API (Phase 8)

Phase 8 adds the immutable chapter revision surface below. The embedded OpenAPI extension `internal/server/openapi_phase8.json` is merged into the canonical `/api/openapi.json` document and is validated by CI against registered routes and schemas.

```text
GET  /api/projects/{id}/chapters/{chapter}
GET  /api/projects/{id}/chapters/{chapter}/versions
POST /api/projects/{id}/chapters/{chapter}/versions
GET  /api/projects/{id}/chapters/{chapter}/versions/{version}
GET  /api/projects/{id}/chapters/{chapter}/diff
POST /api/projects/{id}/chapters/{chapter}/versions/{version}/check
POST /api/projects/{id}/chapters/{chapter}/versions/{version}/restore
POST /api/projects/{id}/chapters/{chapter}/versions/{version}/accept
POST /api/projects/{id}/chapters/{chapter}/versions/{version}/reject
POST /api/projects/{id}/chapters/{chapter}/versions/{version}/finalize
GET  /api/projects/{id}/chapters/{chapter}/sync-status
POST /api/projects/{id}/chapters/{chapter}/sync
GET  /api/projects/{id}/chapters/{chapter}/rebuild
GET  /api/projects/{id}/chapters/{chapter}/plan-impact
```

`POST .../versions` accepts only chapter `content` and always appends a `human_revision`; it does not replace Active Final. Restore also appends a new immutable version. Reject retains the version and records its reason. Accept records approval but does not commit Truth. Finalize is the only Phase 8 endpoint that may create/switch a Final and commit accepted Human Final or generated-final authority.

Every Phase 8 write requires `Idempotency-Key`. The same key bound to the same canonical operation replays its stored result. Reusing a key for different content/version/operation fails closed with a structured conflict. The external-sync write may include the `observed_sha` shown by `sync-status`; if the file changes before synchronization begins, the server returns a safe content-changed error rather than accepting stale browser state.

Version JSON uses `parent_version` for the immutable parent ID. Rebuild JSON uses `status`. `check` returns `{ "evaluation": ... }`; restore/accept/reject return `{ "version": ... }`; external sync returns the synchronized `version`, persisted proposal/continuity/review fields, a conflict count and `sync_required`. The Web client is intentionally aligned with these transport shapes rather than inventing a second API model.

Diff supports `inline` and `side_by_side`, has a hard per-page limit of 500 lines, bounded byte/line/work ceilings, and a deterministic cursor for truncated results. Oversized or adversarial input fails with a safe structured error; the server never attempts an unbounded quadratic whole-book diff.

`GET .../chapters/{chapter}` is the compact authoritative view containing Active Final, latest version, total version count, external SHA state and derived-state status. `GET .../plan-impact` is bounded/paginated and surfaces downstream assumptions affected by an accepted Human Final rather than silently editing historical plans.

See [CHAPTER_VERSIONS.md](CHAPTER_VERSIONS.md) for state transitions and Scenario B.
