# Diagnostics, costs and request protection

Phase 12 records the normal Web/Autopilot non-streaming Generate boundary. It does not claim to meter retained TUI/Headless streams or retries hidden inside an external SDK/provider. Tests with a controlled HTTP provider exercise the actual current SDK primary/fallback path; third-party billing is still authoritative.

## Meaning of the counters

A logical operation (chapter writing, planning, review, import analysis) may have multiple explicit SDK attempts. Every primary, configured fallback and outer runtime retry is separately admitted and recorded before Generate. A durable local result replay is counted separately and creates no new attempt or cost. Old `model_calls` without these observation links are reported as untracked history, not backfilled as zero-cost calls. The default manual Web path has the task scope `manual`; durable Autopilot operations use their job ID.

Project Migration 11 appends attempt records, policy revision, replay counts and immutable policy/reconciliation operations to the project database. Library/Truth authority and chapter finalization are unchanged. Prices and configured limits are not model credentials. Phase 11 backups include these records and verify the current bundled schema as well as older known schemas. Restoring does not clear spent project budgets or start jobs.

`input_tokens`/`output_tokens` are nullable. Provider-returned totals are distinct from the conservative local rune-based input estimate. Missing usage or missing prices produce null cost, never invented zero usage. Monetary integers are millionths of the chosen currency; input/output rates use millionths per million tokens. Each attempt freezes its own price and revision. Known costs are **rate-card estimates**, not invoices; cached/reasoning/special billing categories are not separately discounted or added a second time. Currency cannot change after attempts exist; future price changes do not reprice history. A user must explicitly configure a zero price for a free model.

## Admission and recovery

The default rate card is empty. Monetary/call quotas are disabled by zero limits; users can enable them from Diagnostics & Cost. Defaults still cap output at 8192 tokens, the input estimate at 100000, and cool a provider after three consecutive failures for 60 seconds. No health probe is issued by opening a page or constructing clients.

Admission acquires the SQLite write lock, examines project and task quotas, reserves estimated input plus configured maximum output cost, and saves a pending attempt before the network call. Output limits are passed to the model call. Pending records block further requests; a cancelled client does not prevent a bounded attempt to persist the outcome. If metadata storage fails after a successful response, the business response is preserved while the pending record blocks the next paid call. A result can still have been processed remotely before local persistence; exactly-once billing is not promised.

Configured monetary budgets require an exact alias/model price and block unresolved unknown costs. An input estimate and a provider output limit are not invoice guarantees: hidden retries, special billing and inaccurate usage can exceed the local estimate. Failures remain visible; an unknown-cost request can require manual invoice reconciliation. Do not approve an amount while the request is still active. API writes use the project task/OS-lock guard and a collection revision. Reconciliation retains the prior attempt in an immutable operation record; it does not erase unknown token counts or promote manuscript content.

Budget/call/input limits, unresolved attempts and provider pause/cooldown become a durable Autopilot pause with a safe reason code. They are not literature failures and do not consume unbounded chapter retries. Nonretryable provider failures stop the task without bypassing continuity. Configured fallback may be used for an unavailable provider, but project/task/storage gates do not trigger another charge through fallback. Editing limits does not automatically resume a job.

## User surfaces

**Diagnostics & Cost** offers project/task/chapter filters, paginated attempts, role/provider/model totals, observed provider states, policy and exact rate-card editing, explicit reconciliation and a redacted JSON report. The current totals cover the selected filters; cache replay and legacy coverage counts are project-wide. Up to 500 groups and 100 attempts per page are returned. Provider state is an observation of recent attempts, not a global service-status claim.

The diagnostics view reuses existing task errors, completed-version/rebuild evidence, unresolved conflicts and Chapter-N ledger items. Stale progress is an advisory, not proof that a worker died; no second worker is started to fix it. The bounded recent job history and ledger context are not a whole-book audit. Novelty/repetition or stale-summary semantic judgments are not invented from a timer; pending derived-state rebuilds and existing review results are surfaced instead. Existing Foreshadows/Versions/Autopilot pages remain the action endpoints.

Attempt records commit before safe structured logs/SSE invalidations. SSE publication failure does not rerun a paid call. The page also refreshes from persisted state, so a lost invalidation cannot permanently replace backend truth. Logs contain IDs and codes, not full prompts, provider error bodies, URLs, keys or prose. The default report hashes project/task/provider/model identities and excludes free-form notes and configuration. It is deliberately separate from a Phase 11 project ZIP, which can contain manuscript and model-response history. Inspect even redacted metadata before sharing.

## API and local verification

- GET/POST `/api/projects/{id}/observability`
- GET `/api/projects/{id}/observability/diagnostics`
- GET `/api/projects/{id}/observability/report`

Mutations require Idempotency-Key, expected_revision and exactly one policy or reconciliation object. Existing write/task locks apply. The OpenAPI extension and typed client match the runtime routes. Read paths do not perform paid model probes.

Named checks cover quotas/reservations, unknown costs, immutable price snapshots, reconciliation/replay, concurrent admission, provider cooldown, actual configured HTTP fallback, actual result replay, quota-driven task pause, project isolation, report redaction and schema-compatible backup. Their exact executed outcome belongs in the delivery PR. No complete test suite, paid-provider invoice reconciliation, long-book or platform matrix is asserted by this document.
