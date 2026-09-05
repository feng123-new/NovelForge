# Phase 10 — Skills, style and reference libraries

Phase 10 adds project-local Markdown Skills, distinct style/reference collections and configurable advisory expression/repetition checks. Phase 11–13 remain paused. The current Web/Autopilot model path is connected; this does not rewrite the retained upstream TUI skill/configuration loader.

## Storage and editing

Project Migration 9 appends `authoring_state`, `authoring_entries`, `authoring_search`, immutable mutation operations and immutable model-input selections to the existing project database. Old migration definitions and checksums are unchanged. Existing database initialization retains its pre-migration backup and validation. No user database is migrated by a Git merge itself.

The **Skills & Libraries** page supports create/edit/enable/disable/delete, Markdown import (one UTF-8 `.md` file up to 16 KiB), a plain-text source note, priority, chapter availability and optional POV scope. Markdown and HTML are displayed as text; no script, tool, URL or filesystem path in a resource is executed. Do not put credentials in references: selected content is intended to be sent to the configured model when a writing task runs. The page itself does not initiate a model call.

Four embedded Markdown methods cover Writing, Review, Polish and Planning. They remain read-only base guidelines; project Skills supplement the selected role. Rewrites include both Writing and Polish. Custom Skills retain their role, and are not treated as executable plugins.

Every mutation requires an Idempotency-Key and the current collection revision. Conflicting edits return 409 without changing state; retries with the original request replay the same mutation. At most 500 entries are stored per project. Updates/deletes maintain the full-text index in the same transaction. Mutation records and previously selected request data are retained for reproducible replay, not erased by deleting a library entry.

## Retrieval and authority

Style and knowledge entries are distinct kinds and have separate views/search filters. They are never written into Truth Store, Narrative Ledger, chapter prose or the novel's canonical FTS index. Search uses FTS5 unicode text plus an encoded-rune phrase index for Chinese substrings such as two-character names. It is not semantic/vector, alias or synonym search.

Retrieval filters `enabled`, `from_chapter <= current chapter`, and matching/global POV before ranking. A resource with blank POV is explicitly global; label POV-sensitive references appropriately. This filtering cannot infer secrets hidden in arbitrarily pasted prose. Authoring/editor management views are not character knowledge.

A model selection contains the applicable built-in and up to 16 custom Skills, up to six pinned candidates and six search matches per style/reference kind, plus configured rules. Duplicate entries are removed deterministically. Selected Markdown, source provenance, rules and resource revision are pinned to a durable operation scope **before** computing the model request hash. Retried operations reuse that selection, while new scopes see current resources.

All selected authoring material goes through the Context Compiler: Skills/rules and style examples use the Style layer; research references use Historical/structured retrieval, not Truth. Mandatory method/rule overflow fails closed; optional examples/references may be trimmed. Writer uses its existing overall budget. Planner shares its existing Chapter-N budget. Foundation and Editor craft input use a bounded 6,000-token estimate. No unlimited reference map is appended after compilation.

The runtime prompt explicitly says: instructions are subordinate to system rules, accepted facts, output schemas and POV boundaries. Style examples demonstrate expression, not plot or character canon. References do not grant characters new knowledge. Librarian extraction remains based only on actual candidate prose.

## Model and task integration

- Writer: custom Writing and selected style/reference material enter `compiled_context`.
- Rewrite: Writing plus Polish enter the actual existing rewrite request, retaining previous draft and feedback.
- Planner: Planning and scoped references enter the same selected Chapter-N context. Foundation generation receives budgeted Planning input.
- Editor: Review Skills, style material and deterministic advisory findings enter the request before hashing. Findings are also appended to persisted review weaknesses without changing the model's literary score or continuity result.
- Phase 9: the authoring revision participates in the authoritative context fingerprint. Editing while a job runs is rejected by the existing project/task guard. Editing after pause invalidates an already planned chapter rather than silently continuing with a new library. Old in-flight fingerprints may require explicit review after upgrading.

## Expression and repetition checks

Rules are user-configurable and advisory, **not an AI-authorship detector**. Presets draw attention to a few common stock phrases; zero allowed phrase occurrences flags every use. Checks count case-insensitive literal phrases, normalized repeated sentences within the chapter and exact normalized sentence reuse in recent accepted Finals. Deliberate motifs, dialogue habits and quotations require human judgment. No hidden score deduction, forced acceptance or automatic fact modification occurs.

Limits: 32 phrases, 160 UTF-8 bytes each; sentence minimum 4–200 runes; recent comparison at most three earlier active Finals, the first 48,000 characters of each. Candidate input is bounded to 1 MiB including transport limits. At most 64 findings are returned. This is not fuzzy paraphrase or whole-book plagiarism detection. The model-facing report is bounded and passes through the compiler as well. The local preview uses saved rules, no paid model.

## API

```
GET  /api/projects/{id}/authoring?kind=skill|style|knowledge&limit=50&offset=0
POST /api/projects/{id}/authoring
GET  /api/projects/{id}/authoring/search?kind=knowledge&q=张三&chapter=10&pov=Mira
POST /api/projects/{id}/authoring/lint
```

A mutation contains `expected_revision` and exactly one of `entry`, `delete_id`, or `rules`. Blank entry ID creates a new resource; a nonblank ID must exist in that project. Search supports bounded pagination. Lint accepts `{chapter,text}` and returns `{revision,report}`. All POST requests reject unknown fields and require Idempotency-Key. API errors do not expose filesystem paths or database/Provider details.

## Verification scope

Only affected entry builds, named backend short flows, focused frontend tests, type checking and regenerated embedded assets are in scope. Final PR records exact commits and actual outcomes. No complete Go/Vitest/race suites, platform matrix, paid-model creativity run, long-book simulation or Phase 11–13 delivery is implied.
