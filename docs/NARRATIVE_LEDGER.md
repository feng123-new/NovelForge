# Narrative Ledger

Phase 6 adds an authoritative, temporal ledger for narrative obligations that do not fit a flat fact table. It is deliberately separate from retrieval and from Phase 7's Context Compiler.

## Authority boundary

Generated content reaches the ledger only through a completed Phase 5 chapter transaction whose accepted Final candidate has a durable Fact Proposal. The project adapter reconciles completed transactions into `narrative_ledger_commits`; the transaction ID is the idempotency key and a content hash rejects same-key/different-content replay. Writer, Librarian, retrieval and RAG paths do not receive a ledger write handle.

Explicit local human edits use the same event tables with `human` authority and an `Idempotency-Key`. Retrieval authority is rejected.

## Foreshadows

Stored lifecycle:

```text
planned -> planted -> reinforced -> revealed
    |          |             |
    +----------+-------------+-> abandoned
```

`overdue` is never stored. It is computed for Chapter N when an active foreshadow has `due_chapter < N`. Migration 4 supplies `foreshadow_status_view` for the current accepted chapter and indexed Chapter-N queries for arbitrary historical views.

Planner injection is deterministic:

1. every OVERDUE item is mandatory;
2. every active CRITICAL item is mandatory;
3. UPCOMING items due within three chapters are appended with a bounded optional count.

Phase 6 never drops mandatory ledger obligations. Phase 7 will account for their tokens while preserving that invariant.

## Secrets and knowledge boundaries

Secrets have their own lifecycle and are not encoded as foreshadows. `secret_knowledge` stores holder intervals with `known_from_chapter` and optional `known_until_chapter`. `public_from_chapter` gives the public boundary. Queries always accept Chapter N, so future holders and future public revelations cannot leak into earlier planning or diagnostics.

A boundary-only query omits the secret description and exposes only key, title, status, public state and holders.

## Persistence

Project migration 4 adds:

```text
narrative_ledger_commits
foreshadows
foreshadow_events
secrets
secret_knowledge
secret_events
narrative_ledger_current_chapter (view)
foreshadow_status_view (view)
secret_status_view (view)
```

Events are immutable. Current rows are projections that carry the latest source transaction and updated chapter. Finalize replay cannot duplicate commits or events.

## Diagnostics

Stable Phase 6 codes include:

```text
LEDGER_FORESHADOW_OVERDUE
LEDGER_FORESHADOW_DUE_SOON
LEDGER_INVALID_TRANSITION
LEDGER_SOURCE_CONTENT_CONFLICT
LEDGER_AUTHORITY_REJECTED
```

Dashboard counts are calculated from authoritative Chapter-N rows; no placeholder or client-side estimate is used.

## HTTP surface

```text
GET|POST  /api/projects/{id}/foreshadows
GET|PATCH /api/projects/{id}/foreshadows/{key}
GET|POST  /api/projects/{id}/secrets
GET|PATCH /api/projects/{id}/secrets/{key}
GET        /api/projects/{id}/ledger/planner-context
GET        /api/projects/{id}/ledger/dashboard
GET        /api/projects/{id}/ledger/diagnostics
```

All writes require `Idempotency-Key`, strict single-object JSON and the existing one-MiB request bound. Responses use `Cache-Control: no-store`.

## Scenario E

The blocking Scenario E proves that a critical planted foreshadow becomes computed OVERDUE after its due chapter, remains mandatory in planner context, emits a stable diagnostic, and survives replay without duplicate ledger events. The same scenario proves that a secret's Chapter-N holder and public states do not reveal future knowledge.
