# NovelForge Architecture

## 1. Purpose

NovelForge is an engineering evolution of ainovel-cli for novels in the 1–3 million Chinese-character range, 300–1000+ chapters, 50–300 characters, and 100,000+ facts/events. The system must support both Human Copilot and resumable Autopilot without sending the whole book to an LLM on every chapter.

The first import deliberately keeps the upstream module path and core package layout. NovelForge additions are introduced through new packages and adapters so upstream fixes can still be compared and integrated.

## 2. Current executable surfaces

```text
novelforge              -> existing interactive TUI
novelforge --headless   -> existing unattended engine entry
novelforge server       -> new embedded REST/SSE/Web entry
cmd/ainovel-cli         -> retained legacy/upstream-compatible command
```

The Web server is additive. It does not replace the TUI, Headless runner, Engine, Store, Checkpoint, Import, Export, Sync, Diagnose, Simulation Profile, or model providers.

## 3. Target component model

```text
┌────────────────────────────────────────────────────────────┐
│ Web / CLI / Headless                                         │
└─────────────────────────────┬───────────────────────────────┐
│ NovelForge API                                                │
│ REST commands · SSE events · OpenAPI · auth boundary later   │
└──────────────────────────────┬───────────────────────────────┘
                               │
┌──────────────────────────────▼───────────────────────────────┐
│ Deterministic Engine                                          │
│ state machine · idempotency · retries · checkpoints · jobs   │
└──────┬──────────────────────┬──────────────────────┬────────┘
        │                      │                      │
┌───────▼────────┐    ┌────────▼─────────┐   ┌───────▼────────┐
│ Agent Runtime  │    │ Context Compiler │   │ Quality Gate    │
│ role workers   │    │ budgeted layers  │   │ continuity/edit │
└──────┬────────┘    └────────┬─────────┘   └────────┬────────┘
        │                      │                      │
┌───────▼─────────────────────▼──────────────────────▼────────┐
│ LLM Router                    Truth Store / Event Projection  │
│ providers · fallback          SQLite · FTS5 · provenance     │
└──────────────────────────────┬──────────────────────────────┘
                                │
                     ┌──────────▼──────────┐
                     │ Checkpoint / Backup │
                     └─────────────────────┘
```

## 4. Deterministic versus semantic responsibility

**Go owns:** lifecycle, state transitions, routing, idempotency, retry limits, atomic commit, schema validation, chapter versions, job durability, API, permissions, files, migrations, provenance, event projection, checkpoint and recovery.

**LLMs own:** premise/world/character ideation, rolling planning, literary drafting, ambiguous continuity judgment, literary review, fact proposals and style analysis.

An LLM never writes authoritative Truth Store rows directly. It returns schema-validated proposals; the Engine validates and commits them atomically only after the corresponding chapter version becomes Final.

## 5. Authority order

When sources conflict, the Engine applies this order:

```text
Structured Truth Store
> Confirmed Human Edit
> Final Chapter Version
> Current Chapter Plan
> Arc Plan
> Volume Plan
~ Compass
> LLM Suggestion
```

Human-confirmed facts use `source=human` and the highest authority. Later agents may propose changes but cannot silently overwrite them.

## 6. Agent boundaries

NovelForge keeps the number of agents intentionally small:

- **Architect**: premise, world frame, character frame, theme, ending direction, compass, volume and arc; never writes chapter prose.
- **Planner**: rolling 3–8 chapter horizon and structured chapter plans.
- **Writer**: prose only; produces a Chapter Draft and cannot mutate Truth.
- **Continuity**: checks state, timeline, relations, inventory, knowledge boundary, rules and foreshadow consistency; emits structured FAIL/WARN results.
- **Editor**: literary quality, pacing, conflict, emotion, dialogue distinction, hooks, repetition and configurable AI-flavor patterns.
- **Librarian**: extracts structured Fact Proposals from a candidate final chapter; Engine validation is required before commit.

The upstream Architect/Writer/Editor/Arbiter behavior remains available while these boundaries are introduced through adapters rather than a destructive rewrite.

## 7. Truth Store target

V1 uses SQLite with versioned migrations and Repository interfaces. Core records include Novel, Entity, Character, time-bounded CharacterState, Relation, KnowledgeFact/Holder, Inventory Event, Timeline Event, Fact/Provenance, Foreshadow, Secret and ChapterVersion.

Longitudinal state is event-sourced where it materially improves recomputation. A location change, item acquisition, injury or death is an immutable event; projections answer “what was true at Chapter N.” Editing Chapter 50 invalidates and rebuilds derived projections after Chapter 50 without contaminating earlier state.

## 8. Context Compiler target

Context is assembled under an explicit token budget:

1. **Truth** — current character state, relations, knowledge, inventory, timeline, rules, secrets and active foreshadows.
2. **Narrative** — compass, volume, arc and current chapter plan.
3. **Recent** — 2–5 recent chapters or summaries.
4. **Historical Retrieval** — structured queries, timeline, foreshadow/relation lookup, FTS5 and optional vector retrieval.
5. **Style** — simulation profile, user rules, skills, voice and forbidden patterns.

Critical items such as current plan, POV state, hard world rules and urgent foreshadows are pinned before optional historical text. The daily path may not perform O(n²) whole-book scans.

## 9. Chapter transaction

```text
PLAN
  -> DRAFT
  -> EXTRACT FACT PROPOSALS
  -> CONTINUITY CHECK
  -> EDITOR REVIEW
  -> bounded REWRITE (default max 2)
  -> choose best candidate / HOLD severe errors
  -> FINAL
  -> atomic COMMIT TRUTH
  -> CHECKPOINT
  -> NEXT
```

Writer success followed by Librarian failure leaves a Draft. It never advances to Final. Checkpoints are created after every expensive, independently recoverable step so restart does not repeat paid calls.

## 10. Server/API foundation implemented now

`internal/server` is standard-library Go and has no LLM dependency. It provides:

- real project discovery from existing ainovel artifacts;
- stable opaque project IDs without exposing absolute filesystem paths;
- REST health/project endpoints and embedded OpenAPI;
- process-local non-blocking SSE fan-out with a stable Event envelope;
- an embedded responsive Web shell from `web/dist`;
- security headers, loopback default, graceful shutdown and non-loopback warnings.

This transport is intentionally read-only in the foundation phase. Mutating endpoints will be added only when they can call real Engine/Job/Truth services with atomic semantics.

## 11. Storage and project isolation

A Library workspace may contain multiple project directories. Each project will own its database, files, config, rules, skills, output and backups. API responses expose a relative project label and opaque ID; they do not return API keys or absolute host paths.

V1 does not require PostgreSQL, Redis, Kafka, ElasticSearch or Qdrant. SQLite, FTS5, local durable jobs, SSE and `go:embed` are the default single-machine deployment.

## 12. Compatibility strategy

- Keep `cmd/ainovel-cli` and upstream package paths during early phases.
- `cmd/novelforge` calls the same TUI/headless runtime.
- Continue reading `~/.ainovel` and `./.ainovel` unchanged in Phase 1.
- Introduce `~/.novelforge` through an explicit, backed-up migration rather than silently moving credentials.
- Add new features in separate packages and minimize churn in upstream core files.
- Record the exact upstream base and sync procedure in `UPSTREAM_BASE.md` and `docs/UPSTREAM_SYNC.md`.

## 13. License architecture

ainovel-cli code is Apache-2.0 and directly reused. show-me-the-story is MIT and is currently a design/UI reference only; any future direct reuse must retain its MIT copyright/license. NovelWriter (AGPL-3.0) and AI-Novel-Writer (GPL-3.0) are clean-room design references only. No source from either copyleft project may enter NovelForge's Apache-2.0 tree.
