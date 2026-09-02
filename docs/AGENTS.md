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
