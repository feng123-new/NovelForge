# Phase 11 — Manuscript lifecycle

Phase 11 connects manuscript interchange, resumable import analysis, portable project backup/restore and explicit project-format migration to the existing Web/ChapterVersion workflow. Phase 12 is delivered. Phase 13A supplies candidate delivery; Phase 13B full acceptance remains local. Exact tested source, named checks, merge and temporary-branch cleanup belong in the delivery PR. This document does not imply full-suite, platform-matrix or long-book acceptance.

## Import without implicit acceptance

Open **Import & Backup**, select a managed project and upload UTF-8 TXT/Markdown or an unencrypted text EPUB. Upload only parses and stages original source bytes and individual chapters in project Migration 10. It does not call a model, replace existing prose or write Truth. Review the chapter headings, count and order before continuing. A nonempty overlapping range containing raw chapter files, versions, another import or quality transactions is rejected. Re-uploading the identical source/name/range returns its existing import record.

TXT recognizes Chinese chapter headings and numbered English `Chapter` headings. Markdown uses numbered headings where present, otherwise level-one/two headings, excluding fenced code. A leading book title is not an extra chapter; ordinary preface text is preserved as a separate section. Chapter numbers are assigned in imported order starting at the requested chapter. Source numbering is retained in titles, not trusted as an instruction to overwrite arbitrary project positions. A text document without recognizable headings is one chapter. Non-UTF-8 input must be converted explicitly; encoding is not guessed.

EPUB follows the package spine, not ZIP filename order. Selected linear XHTML documents become sections. Plain paragraphs and a heading are retained; images, fonts, rich layout, scripting and encrypted/DRM books are not supported. No external resource or URL is fetched. Imported markup is never executed in the browser. This is manuscript text interchange, not a general EPUB renderer or lossless typography converter.

**Continue import** saves each staged chapter as an immutable human-revision candidate through the existing version service. It never switches Active Final. **Continue analysis** explicitly calls the configured Librarian/continuity/editor path and persists its evaluation. It requires user consent in the page and can incur provider charges. The UI issues one bounded chapter step at a time; pause takes effect after the current step exits. Closing the page stops the client from scheduling further steps. Reopening the import reads its durable cursor and resumes only missing work. This is an explicit resumable workflow, not an unbounded hidden worker.

Analysis completion does not mean PASS, acceptance, or Final. Follow the Versions link to inspect findings and explicitly Check/Accept/Finalize using the existing authority rules. Severe continuity failures still block acceptance. Imported chapters reserve their range: Autopilot will not overwrite them or fabricate acceptance. After the relevant Finals and checkpoints exist, an explicit Autopilot task can continue using an appropriate saved Foundation request; import does not silently invent a whole-book Foundation.

Stable import and Check keys recover saved candidates and persisted analysis results across HTTP retries or server recreation. The original model result can still be repeated if a process dies before it is durably saved; exactly-once provider billing is not promised. A failed step retains its candidate and exposes an import error code. Retry does not erase earlier completed chapters.

Limits: upload 32 MiB, at most 1000 sections within chapters 1–1000, each chapter at most 1 MiB. Reads paginate at 100 records maximum (the page uses 50). Source bytes, chapter hashes and import progress live in the project database, not browser storage or configuration files.

## Export only proved Finals

TXT, Markdown and EPUB export use an explicit contiguous chapter range. Omitted bounds mean chapter 1 through the highest active Final. Every selected chapter must have a matching completed Final saga, version/checkpoint proof, no pending derived-state rebuild and no external-file sync conflict. A gap, draft or partially committed Final stops export instead of producing a deceptively complete book. Export never promotes a candidate or calls a model.

TXT/Markdown retain manuscript text; the EPUB exporter XML-escapes it into paragraphs with a navigation document and an EPUB 3 package. Embedded formatting and Markdown syntax inside prose are not interpreted as active markup. The first EPUB ZIP entry is the uncompressed `mimetype`. Export response headers identify an attachment and provide its SHA-256; nothing is served as executable HTML.

## Portable project backups

Backup first acquires the same project OS lease as Web mutations, TUI/Headless and the new worker, and requires the active writing step to stop. It uses SQLite `VACUUM INTO` for a standalone committed snapshot including WAL data. Copying a live main `.db` file alone is deliberately avoided. Backups with unfinished Final/rebuild state or mismatched active manuscript files are refused: resolve/recover the transaction first.

The ZIP contains a versioned manifest with per-file size and SHA-256 plus an allowlisted project snapshot:

- `.novelforge/project.db` and typed `project.json`: versions, accepted facts/events, ledger, authoring libraries, model-result history and import progress/source bytes.
- Non-secret saved Foundation request/output when present.
- Text `.txt`/`.md`/`.markdown` files under chapters, references and project skills/style/rules directories.

Global and project model configuration/credentials, `.ainovel` configuration, workspace `server.db`, workspace jobs, locks, logs, trash, earlier backups, executables and arbitrary binary resources are excluded. Model output and arbitrary user-pasted prose may contain sensitive information; backup is not automatic redaction, secure erasure or a public sharing package. The allowlist is intentional, not a claim that arbitrary ancillary files are copied.

Limits: ZIP at most 64 MiB, expanded payload at most 256 MiB, at most 4096 archive entries. Unsafe/absolute/traversing paths, case collisions, duplicate entries, symlinks, encrypted members and unsupported compression are rejected. Actual read sizes and CRC are checked, not just advertised ZIP metadata. Large archives are rejected before publication, not partially restored.

## Restore safely into another workspace

Restore verifies the manifest, hashes and metadata, then writes into a private staging directory. It compares **all** SQLite schema objects and migration checksums with a pristine schema built from bundled migrations before querying application tables or applying pending migrations. Uploaded SQL, triggers or views are never adopted merely because a file hash is valid. It also checks SQLite integrity, foreign keys, project identities, version content hashes and active Final/file/checkpoint consistency. Known project schemas 1–11 are supported; unfamiliar/modified schemas are refused rather than guessed.

The restored project preserves its ID and all version/event references and is published to a new `restored-<id>` directory only after checks succeed. An existing project ID or destination is never overwritten; use another workspace to restore an older copy. Retrying the same archive after an interrupted response returns the already published restore without replacing its current contents. Unfinished orphaned workspace tasks for that project ID block restore so they cannot adopt restored data unexpectedly.

Workspace tasks are not copied or restarted. The result explicitly reports `jobs_resumed=false` and `requires_configuration=true`; create model configuration and explicitly start any new work. Project-local import progress can be resumed from the page. A retained unfinished draft from a former Autopilot task must go through explicit version review/finalization rather than being silently attached to a new task.

Only product-managed project snapshots are accepted. The retained legacy skeleton import can initialize an old project first; this new route does not reinterpret every historical upstream folder layout. Restored SQL provenance remains intact, not rekeyed by search-and-replace.

## Project-format migration and preimages

The explicit migration action confirms the exact project ID and expected format. Known format 1 is advanced to current format 2, and pending bundled database migrations are applied. This phase appends project Migration 10; it does not edit migrations 1–9 or change the meaning of existing checksums. Already-current state is a no-op.

Before an explicit change, a portable pre-migration ZIP is stored in the project's private backups directory and can be downloaded through the returned opaque backup label. Ordinary project database opening still uses the existing migration runner and its database pre-migration backup, not an implicit full-project ZIP. An older backup restored into a fresh workspace also uses the existing runner for pending known migrations. No migration is executed on a user's local project by merging source code on GitHub.

## API

All POSTs require `Idempotency-Key`. JSON controls reject unknown fields. Multipart uploads accept one `file` and, for manuscripts only, optional `start_chapter`; the transport hashes canonical filename/content-digest/range metadata rather than multipart boundary bytes. Binary response downloads are read operations but hold the project lease for consistency.

```text
GET/POST /api/projects/{id}/lifecycle/imports
GET      /api/projects/{id}/lifecycle/imports/{import}
POST     /api/projects/{id}/lifecycle/imports/{import}/step
GET      /api/projects/{id}/lifecycle/export?format=md&from=1&to=10
GET      /api/projects/{id}/lifecycle/backup
GET      /api/projects/{id}/lifecycle/backups/{backup}
POST     /api/projects/{id}/lifecycle/migrate
POST     /api/lifecycle/restore
```

Step body: `{chapter, analyze}`. Migration body: `{expected_format, confirm}`. Import detail exposes headings, candidate IDs and saved/analyzed cursors, not the uploaded source body. Download and API errors use safe codes without database statements, server paths or provider credentials.

## Validation boundary

The delivery uses named codec/path/schema tests, a pre-migration backup/restore sample, an import-analysis interruption/replay/explicit-Final/export/cross-workspace-restore short flow, API isolation/locking checks, focused UI consent tests, affected Go entry builds and a regenerated embedded Web build. Actual outcomes and source identifiers are recorded only after execution in the PR. No full Go/frontend/race suite, platform matrix, paid-provider run, EPUB-reader interoperability matrix or hundred-chapter/scale test is implied.
