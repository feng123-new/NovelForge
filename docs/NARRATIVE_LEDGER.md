# Narrative Ledger

## Purpose

The Narrative Ledger tracks story promises and knowledge boundaries without creating a second authority system. The Structured Truth Store remains authoritative for world facts. The ledger is a deterministic, indexed projection for **Foreshadows** and **Secrets**, and it accepts model-originated changes only through an accepted Phase 5 Final Candidate.

## Authority and write boundaries

Allowed production writers are:

1. the Phase 5 `ChapterCommitCoordinator`, after a continuity-safe candidate is selected and its fact proposal is accepted; and
2. an explicit local human write through the authenticated local API boundary.

Writer, Librarian, retrieval, and RAG components do not receive the ledger repository. Librarian `foreshadow_updates` and `secrets` remain proposals until Finalize. A rejected draft, continuity failure, or HOLD cannot mutate the ledger.

Each accepted Final uses a stable transaction/idempotency key. Replaying the same transaction and content returns the prior ledger commit; changing content under the same key fails with a conflict. Foreshadow, Secret, and holder audit events are therefore not duplicated by Finalize recovery.

## Foreshadow model and lifecycle

A Foreshadow records title, bounded description, importance, planted chapter, expected payoff range, optional actual payoff, related entity/arc IDs, last progress chapter, urgency, source version, authority, and UTC timestamps.

Persisted states are:

```text
planned -> planted -> progressing -> resolved
   |          |             |
   +----------+-------------+-> abandoned
              +-------------+-> contradicted
contradicted -> progressing | resolved | abandoned
```

Transitions are validated by Go. A resolved item cannot silently return to planned, actual payoff is accepted only for resolved items, and payoff ranges cannot precede the plant chapter. Every change writes an append-only audit event.

### Computed OVERDUE

`OVERDUE` is never stored as a mutable state. For a Chapter-N query it is computed as:

```text
current_chapter > expected_payoff_max
AND status NOT IN ('resolved', 'abandoned')
```

The response includes `overdue=true` and `overdue_by_chapters=current_chapter-expected_payoff_max`. Dashboard, diagnostics, list filters, and Planner Context use the same calculation.

The critical query is supported by `idx_foreshadows_project_status_payoff` and related project/importance/progress indexes. Tests inspect `EXPLAIN QUERY PLAN` and reject a Phase 6 implementation that loses indexed access.

## Secret model and Chapter-N knowledge

A Secret is independent of Foreshadow. It stores a bounded management description, authority truth, creation chapter, optional reveal chapter, public state, optional related Foreshadow, source version, authority, and UTC timestamps.

Holder rows contain:

- `valid_from_chapter`
- optional `valid_to_chapter`
- `source_version`
- `authority`
- Truth-compatible provenance (`type`, `id`, `chapter`, `version`)

A holder is visible at Chapter N only when the temporal range includes N. A Secret is public at Chapter N only when its public state is public and its reveal chapter is not later than N. Role-bound Planner responses never include the authority truth of an unknown Secret; they include an explicit unknown-boundary item instead. Administrative API reads require `include_truth=true` to return the truth field.

## Planner context

`NarrativeLedgerContextProvider` deterministically supplies:

1. all OVERDUE Foreshadows as mandatory;
2. all active critical Foreshadows as mandatory;
3. current-arc and upcoming Foreshadows in stable bounded order;
4. Secrets known to the POV holder or already public at Chapter N; and
5. explicit unknown Secret boundaries with no truth text.

Phase 6 does not perform token trimming. Phase 7 consumes these stable items and must retain mandatory OVERDUE/critical items even when optional context overflows.

## Database migration 4

Project migration 4 adds:

```text
narrative_ledger_operations
narrative_ledger_commits
foreshadows
foreshadow_entities
foreshadow_arcs
foreshadow_events
secrets
secret_holders
secret_events
narrative_ledger_meta
```

The existing migration runner supplies checksum verification, pre-migration backup, transactional apply, unknown-version rejection, and rollback on failure. Audit-event triggers prohibit update/delete of immutable event history.

## API

Read routes:

```text
GET /api/projects/{id}/foreshadows
GET /api/projects/{id}/foreshadows/{foreshadow}
GET /api/projects/{id}/secrets
GET /api/projects/{id}/secrets/{secret}
GET /api/projects/{id}/ledger/dashboard
GET /api/projects/{id}/ledger/diagnostics
GET /api/projects/{id}/ledger/planner-context
```

Write routes:

```text
POST  /api/projects/{id}/foreshadows
PATCH /api/projects/{id}/foreshadows/{foreshadow}
POST  /api/projects/{id}/secrets
PATCH /api/projects/{id}/secrets/{secret}
POST  /api/projects/{id}/secrets/{secret}/holders
POST  /api/projects/{id}/secrets/{secret}/holders/{holder}/close
```

Writes require `Idempotency-Key`, strict single-object JSON, the shared one-MiB request bound, project ownership checks, trace IDs, and the safe error envelope. Collections cap `limit` at 100 and use explicit stable ordering.

The embedded Web assets are rebuilt from `web/src` and committed exactly; CI fails closed on any `web/dist` or lockfile drift.

## Diagnostics

The diagnostic surface is deterministic and safe. It can emit:

```text
OVERDUE_FORESHADOW
CONTRADICTED_FORESHADOW
STALE_FORESHADOW_PROGRESS
PAYOFF_BEFORE_PLANT
INVALID_PAYOFF_RANGE
SECRET_REVEAL_BEFORE_CREATE
SECRET_HOLDER_RANGE_INVALID
KNOWLEDGE_BOUNDARY_CONFLICT
LEDGER_PROJECTION_STALE
```

Diagnostics contain evidence metadata, never API keys, raw SQL, stack traces, authorization headers, or absolute paths.

## Scenario E

The blocking regression scenario plants a critical Foreshadow at Chapter 20 with expected payoff 100–130. At Chapter 135 it must:

- remain in its persisted lifecycle state;
- compute `OVERDUE` with `overdue_by_chapters=5`;
- appear in Dashboard and Diagnostics; and
- remain mandatory in Planner Context.

The same scenario is exercised at repository and HTTP integration boundaries.
