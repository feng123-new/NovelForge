# Project and API Foundation

This document describes the Phase 2 project lifecycle and local HTTP boundary. It is an implementation contract for the current routes, not a promise of later Truth Store or Autopilot behavior.

## Workspace model

A server instance owns one configured workspace. NovelForge-created projects use this layout:

```text
<workspace>/<project>/
  .novelforge/
    project.json
    project.db
    config.json
    rules/
    skills/
    style/
    output/
    backups/
    trash/
  chapters/
  references/
```

The workspace also owns `.novelforge/server.db`, which contains only server control data: durable events and idempotency records. Story truth remains isolated in each project's `project.db`.

Existing ainovel projects are discovered from their current markers (`meta/book.json`, `meta/progress.json`, `outline.json`, or `chapters/`) and remain readable without an implicit format conversion. `POST /api/projects` with `import_path` explicitly adds NovelForge metadata and a project database to an existing workspace-relative skeleton.

## Project identity and paths

API project IDs are opaque. A legacy project receives a deterministic ID derived from its workspace-relative label so that existing clients can reopen it consistently. NovelForge-created projects use cryptographically random IDs.

Responses never include an absolute host path. The optional `path` field is a workspace-relative compatibility label only. Write requests may not provide an arbitrary destination path. `import_path` is the only filesystem selector and must be a workspace-relative child directory.

## Lifecycle

The implemented lifecycle is:

- create a new project;
- import an existing project skeleton;
- list and open project details;
- update mutable metadata;
- archive and unarchive;
- duplicate while removing runtime credentials and transient database files;
- delete to workspace-private trash by default;
- permanently delete only when `permanent: true` is explicit.

Deletion requires `confirm` to equal the project ID or exact project title. Before moving or removing data, the repository rejects traversal, absolute and Windows drive/UNC paths, symlink escapes, the workspace itself, the filesystem root, the user's home directory, the current working directory, and directories that are themselves Git repositories. Default deletion uses an atomic rename into `<workspace>/.novelforge/trash` and writes a tombstone when possible.

Duplication excludes `.env*`, runtime configuration files, credential-like file names, backups, trash, SQLite WAL/SHM files, and symbolic links. The destination receives a sanitized `.novelforge/config.json`; sensitive keys such as API keys, authorization values, credentials, passwords, private keys, secrets, and tokens are recursively removed.

## HTTP routes

The local API exposes:

```text
GET    /api/health
GET    /api/openapi.json
GET    /api/events
GET    /api/projects
POST   /api/projects
GET    /api/projects/{id}
PATCH  /api/projects/{id}
POST   /api/projects/{id}/archive
POST   /api/projects/{id}/unarchive
POST   /api/projects/{id}/duplicate
DELETE /api/projects/{id}
```

The embedded OpenAPI 3.1 document at `/api/openapi.json` is checked by tests for implemented path coverage and unique `operationId` values.

### Collection queries

`GET /api/projects` supports stable title/ID ordering, `limit` (maximum 100), `offset`, `query`, and `archived`. The response includes the total, current pagination values, and `next_offset` when another page exists.

### Write idempotency

Every write route requires an `Idempotency-Key` header of 1–128 safe ASCII characters. The workspace control database stores:

- key;
- operation;
- project ID;
- request hash over method, request URI, and exact body;
- in-progress/completed status;
- response status and exact response body;
- creation and expiry timestamps.

Repeating the same request returns the original status and body with `Idempotency-Replayed: true`. Reusing a key for a different operation, project, URI, or body returns `409 IDEMPOTENCY_KEY_CONFLICT`. Producers therefore do not repeat a completed filesystem mutation.

### Errors and tracing

REST errors use one envelope:

```json
{
  "error": {
    "code": "PROJECT_NOT_FOUND",
    "message": "project not found",
    "details": {},
    "retryable": false,
    "trace_id": "opaque-trace-id"
  }
}
```

The same trace ID is returned in `X-Trace-ID`. Transport errors expose server-owned codes and messages only; raw SQL, stack traces, credentials, authorization headers, provider responses, and absolute paths are not serialized. JSON bodies are limited to one MiB, reject unknown fields, and must contain exactly one JSON object.

## Durable events and SSE

Mutations append an event to `server.db` before in-process fan-out. `/api/events` supports:

- typed SSE events;
- heartbeats;
- `project` filtering;
- `Last-Event-ID` replay;
- restart recovery from persisted events;
- bounded subscriber queues so a slow client cannot block a producer.

A reconnect replays persisted matching events after the supplied ID and then transitions to live delivery without duplicating event IDs. An overloaded subscriber is disconnected rather than blocking state changes.

## SQLite migrations

`internal/db/migrate` uses the CGo-free `modernc.org/sqlite` driver. Every database enables foreign keys, a bounded busy timeout, and WAL mode. Migrations have an immutable version, name, SHA-256 checksum, and UTC application time in `schema_migrations`.

For an existing database with pending migrations, the runner checkpoints WAL, closes the handle, creates a timestamped owner-readable backup, then applies all pending changes and migration records in one transaction. A failed migration rolls the transaction back; a changed checksum or unknown applied version is rejected rather than guessed.

## Engine boundary

`internal/server/engineadapter.EngineService` is the Web-facing engine interface. `LegacyAdapter` delegates to the existing `host.Host` lifecycle and event/stream channels. The HTTP layer therefore does not copy planning, writing, resume, or checkpoint logic, and later durable jobs can use the same boundary.

## Security defaults

The server listens on loopback by default and retains the existing warning for non-loopback listeners. It serves a restrictive CSP, denies framing, disables MIME sniffing, and uses a graceful shutdown path that closes the control database-backed server resources.
