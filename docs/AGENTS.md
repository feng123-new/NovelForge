# Agent Boundaries

NovelForge keeps creative model calls behind service interfaces and keeps authoritative state changes in deterministic Go code.

## Phase 5 services

| Service | Responsibility | Forbidden responsibility |
| --- | --- | --- |
| `ArchitectService` | Premise, world, character frame, theme, ending, Story Compass, volume/arc structure | Chapter Truth commits |
| `PlannerService` | Structured rolling-horizon `ChapterPlan` | Mutating Truth from a plan |
| `WriterService` | Chapter prose and non-authoritative writing metadata | Truth or projection writes |
| `LibrarianService` | Extract a provenance-bound `FactProposal` from a durable candidate | Direct Truth commit or Final selection |
| `ContinuityService` | Deterministic Chapter-N continuity check against authoritative Truth | Treat RAG or model prose as authority |
| `EditorService` | 0–10 literary review and revision guidance | Override a blocking Continuity FAIL |

The model-backed Writer, Librarian and Editor adapters receive `IdempotentModelCaller`; they do not receive a SQLite handle, HTTP handler, Truth repository or chapter file path. `TruthContinuityService` receives only the Truth repository and the candidate/proposal metadata needed for deterministic checks.

## Structured output

Librarian and Editor model adapters pass raw model responses to the strict decoder. Unknown fields, trailing bytes and multiple JSON values are rejected. Optional repair is bounded and revalidated. Validation exhaustion is an error that leaves durable prior artifacts available for HOLD/recovery.

## Quality transaction

The coordinator owns the finite loop:

```text
PLAN → DRAFT → FACT PROPOSAL → CONTINUITY → EDITOR
     → bounded REWRITE → FINAL CANDIDATE
     → TRUTH COMMIT → CHECKPOINT → COMPLETE / HOLD
```

`max_rewrites` defaults to 2. No agent can create an unbounded self-loop. The coordinator is the only component allowed to turn an accepted proposal into `generated_final` Truth events.

See [QUALITY_GATE.md](QUALITY_GATE.md) for state transitions, recovery and HTTP behavior.

## Narrative Ledger boundaries (Phase 6)

- **Writer** receives no Narrative Ledger repository and cannot create, progress, resolve, or reveal records.
- **Librarian** may propose `foreshadow_updates` and `secrets`; proposals remain non-authoritative.
- **ChapterCommitCoordinator** is the only model-to-ledger production boundary and invokes the ledger only for an accepted Final Candidate.
- **Planner** consumes `NarrativeLedgerContextProvider`; OVERDUE and critical items are mandatory, and Secret truth is filtered by role/Chapter-N knowledge.
- **Continuity** may use the full authority view for checks, but its result cannot make a character know a Secret.
- **Retrieval/RAG** is not a writer and never overrides ledger/Truth authority.

## Phase 8 human-edit and version boundaries

Phase 8 does not give any model agent permission to mutate an accepted human chapter. `ChapterVersion` persistence and the `Coordinator` remain deterministic Go-owned boundaries.

- **Human editor / external file** may supply prose only. Saving creates `human_revision`; it is not a Truth write and does not replace Active Final.
- **Librarian** may extract a new proposal from that immutable revision. It cannot auto-accept the revision, switch Active Final, or supersede Truth.
- **Continuity** may return PASS/WARN/FAIL against Chapter-N Truth. A blocking FAIL prevents acceptance/finalization and cannot be overruled by Editor prose quality.
- **Editor** may score and explain a revision but cannot promote authority or suppress a Truth conflict.
- **Planner / Context Compiler** consume rebuilt Chapter-N state after an accepted Human Final; they do not infer the correction from the raw edited file.
- **Retrieval/RAG** remains evidence only and is rebuilt/read after the authoritative boundary, never used as the reason to accept a human revision.
- **ChapterVersion Coordinator** owns explicit `Check`, `Accept`, `Finalize`, external synchronization, Active Final switching, Human Final authority promotion and bounded rebuild orchestration.

A human revision becomes authoritative only through explicit acceptance followed by successful Finalize. The accepted Human Final has higher authority than generated finals, so later model output cannot silently downgrade it. See [CHAPTER_VERSIONS.md](CHAPTER_VERSIONS.md).
