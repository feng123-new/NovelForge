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
