# NovelForge configuration migration

NovelForge is being moved from the ainovel-compatible configuration layout to a dedicated layout without deleting or silently moving credentials.

## Directory model

Global directories:

```text
~/.novelforge
~/.ainovel
```

Project directories:

```text
./.novelforge
./.ainovel
```

The current compatibility iteration keeps `.ainovel` as the runtime source. The migration command prepares a verified `.novelforge` copy so the later precedence switch can be made without destructive file operations.

## Inspect the current state

```bash
novelforge doctor
novelforge doctor --json
```

`doctor` reports which configuration files exist and whether they are readable. It never prints provider credentials or configuration contents.

Useful options:

```bash
novelforge doctor --project-root /path/to/book
novelforge doctor --home /temporary/home --json
```

## Preview a migration

```bash
novelforge migrate --dry-run
```

By default both the global and current-project legacy directories are considered. Scope can be limited:

```bash
novelforge migrate --global-only --dry-run
novelforge migrate --project-only --project-root /path/to/book --dry-run
```

## Apply a migration

```bash
novelforge migrate
```

For every selected scope the command:

1. checks that the `.ainovel` source is a real directory;
2. refuses symbolic links and unsupported file types;
3. skips the scope when `.novelforge` already exists;
4. creates a timestamped backup under `.novelforge-migration-backups`;
5. records relative paths, file sizes and SHA-256 checksums in a manifest;
6. copies to a temporary directory beside the final destination;
7. atomically renames the temporary directory to `.novelforge`;
8. retains the original `.ainovel` directory.

A failure before the final rename removes the temporary destination. The already-created backup is retained for diagnosis.

## Idempotence and conflicts

Re-running the command does not overwrite an existing `.novelforge` directory. The action is reported as `destination_exists`.

The command intentionally does not merge two configuration directories. Merging credential stores can accidentally combine unrelated API keys or providers. Conflicts must be reviewed explicitly.

## Backup layout

Global backups:

```text
~/.novelforge-migration-backups/<timestamp>-global-ainovel/
```

Project backups:

```text
<project>/.novelforge-migration-backups/<timestamp>-project-ainovel/
```

Both the backup and staged destination receive `migration-manifest.json`. The manifest contains metadata and checksums, never file contents.

## Current limitation

Until the next Phase 1 compatibility slice is merged, keep `.ainovel` in place. The normal TUI and headless runtime still use it. The migration command is deliberately copy-only and does not claim that the new path is active yet.
