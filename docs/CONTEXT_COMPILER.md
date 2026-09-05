# Context Compiler and Hybrid Retrieval

> Current integration: compiler-selected context is used by `novel_context` and the configured Web Writer. These are Phase 1–8 paths, now reused by the Phase 9 worker; Phase 10–13 remain deferred. Delivery evidence and limitations are in [PHASE_01_08_FIXES.md](PHASE_01_08_FIXES.md).

Phase 7 introduces `internal/contextcompiler`, a deterministic, read-only boundary between NovelForge's authoritative project state and model prompts. The compiler does not create facts and does not grant Writer, retrieval, FTS, or vector implementations a write capability.

## Five layers

Every compiled context uses the same stable layer order:

1. **Truth** — Chapter-N character state, relations, knowledge, inventory, timeline, world rules, Secret knowledge boundaries, and active or critical Foreshadows.
2. **Narrative** — Story Compass, Volume Plan, Arc Plan, Current Chapter Plan, and required contract beats.
3. **Recent** — a configurable two-to-five chapter window of text or summaries.
4. **Historical** — bounded retrieval in the exact order Structured Query, Timeline, Foreshadow, Relation, Recent, FTS5, then optional Vector.
5. **Style** — simulation profile, writing rules, skills, voice, forbidden patterns, and user style.

System instructions retain a separate reservation and cannot be consumed by provider content.

## Token budget

The default whole-token allocation is:

| Bucket | Percent |
| --- | ---: |
| Truth | 20% |
| Narrative | 15% |
| Recent | 25% |
| Historical | 20% |
| Style | 10% |
| System | 10% |

Callers may supply another configuration, but all percentages must be between zero and one hundred and must total exactly one hundred. Token counting is injected through `TokenCounter`; the default counter is deterministic and CJK-aware, while a provider-specific tokenizer can be used without changing selection semantics.

Each layer first receives its explicit allocation. Unused content budget is then redistributed in a deterministic global priority order. This avoids wasting an empty layer while retaining reproducible output and a stable SHA-256 context hash.

## Mandatory retention and fail-closed behavior

The following records are represented by explicit `Requirement` values and are always mandatory:

- Current Chapter Plan
- POV Character State
- Critical World Rules
- Critical Foreshadows
- Explicit Knowledge Boundary
- Required Contract Beats

Mandatory records are selected before optional records and may borrow unused capacity. The compiler returns `ErrMandatoryOverflow` instead of dropping one when all mandatory records cannot fit. A requested requirement that was never supplied returns `ErrMissingRequirement`.

Any optional item whose source chapter is later than the requested Chapter N is rejected with the `future_state` trim reason. A future mandatory item fails closed with `ErrFutureMandatory`, because silently compiling a different boundary would hide a provider integrity error.

## Retrieval

Historical retrieval is provider-based and executes in this order:

```text
Structured Query
→ Timeline
→ Foreshadow
→ Relation
→ Recent
→ FTS5
→ Optional Vector
```

`VectorRetriever` is an optional interface. V1 has no dependency on an external vector database and remains fully functional when it is nil.

Migration 5 adds `context_documents`, its project/chapter index, an external-content FTS5 table, and synchronization triggers. Searches are project-scoped, bounded, stable, and include `source_chapter <= Chapter N`; future chapters and other projects cannot leak into the result.

Additive Migration 8 maintains a separate character index for Chinese substring terms, including two-character names. The original text and English token index remain. Backfill and synchronization triggers use the deterministic Go-registered `novelforge_search_characters` function. External SQLite clients must not write `context_documents` without registering that function. Alias, simplified/traditional conversion and synonym recall are not guaranteed by this index.

## Diagnostics

Every compile returns:

- total, system, content, used, and remaining tokens;
- allocated, input, used, selected, and trimmed counts per layer;
- every trimmed item and its reason;
- future and duplicate counts;
- stable rendered context and SHA-256 hash.

`novel_context` calls `CompileLegacyMap` followed by `SelectLegacyMap`: only selected records are returned, preserving their legacy field shape. Compilation errors stop the request rather than returning the original unselected map. `_context_compiler` reports `version: 2` and `status: applied`; detailed layer diagnostics are optional and are compacted before content is trimmed again when the byte budget is tight. `_loading_summary` and `_trimmed` describe the actual returned selection.

The configured Web Writer separately receives compiled project Truth, POV-filtered Ledger, recent documents and FTS evidence as `compiled_context`, before the idempotent model-request hash is calculated. The adapters retain estimated token budgets and bounded queries; this does not imply that every legacy Agent path or whole-book retrieval has been revalidated.

## Determinism and authority

Ordering is stable across runs: layer, historical stage, mandatory status, priority, source chapter, kind, and ID are explicit tie-breaks. FTS5 is retrieval evidence, not Truth. Authoritative facts continue to come from the Chapter-N Truth Store and Narrative Ledger providers.

## Tests and benchmark

Phase 7 tests cover default and configurable allocation, mandatory overflow, missing requirements, deterministic ordering/hash, exact retrieval stage order, duplicate handling, future-state rejection, Knowledge Boundary retention, FTS5 project/chapter isolation, upsert/delete synchronization, query-plan index use, legacy migration, and per-layer diagnostics. `BenchmarkCompilerFiveLayers` measures deterministic five-layer assembly with hundreds of bounded records.

## Phase 8 correction boundary and invalidation

The Context Compiler never watches raw chapter files and never treats browser/editor state as authority. Phase 8 first converts an external or in-Web edit into an immutable `human_revision`, evaluates it, and requires explicit Accept + Finalize. Only the resulting Active Human Final and rebuilt project projections are eligible to feed later prompt compilation.

A Human Final at Chapter N invalidates derived state from N forward, not backward. Phase 8 records a bounded rebuild operation and rebuilds Truth/Ledger-dependent state beginning at N. Context requested for Chapter N-1 continues to resolve against the same earlier authority; context for N and later sees the newly accepted Human Final facts once rebuild completes.

Downstream chapter-plan assumptions that conflict with the new Human Final are surfaced as `chapter_plan_impacts`. They are planning diagnostics, not automatic edits to historical plans. A future Planner/Autopilot worker may act on them only through the normal deterministic planning boundary.

Historical FTS/vector material remains evidence after a correction. It may contain text from a superseded chapter version, but it cannot override the rebuilt Truth/Narrative layers. The mandatory authority layers are therefore selected before optional historical retrieval, preserving the Human Final > Generated Final authority order.

Phase 8 Scenario B verifies the key boundary at Chapter 50: Chapter 49 context-driving Truth remains unchanged while Chapter 50+ projects the accepted human survival/injury/escape correction. See [CHAPTER_VERSIONS.md](CHAPTER_VERSIONS.md) and [TRUTH_STORE.md](TRUTH_STORE.md).
