# NovelForge configuration migration

NovelForge uses a dedicated configuration layout while continuing to read existing ainovel-compatible installations. Migration is explicit, backed up and copy-only: normal startup never moves or copies API keys.

## Active directory model

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

The legacy `cmd/ainovel-cli` command remains on `.ainovel`. The `novelforge` command activates the compatibility-aware NovelForge profile.

## Configuration precedence

Highest to lowest:

```text
--config <path>
NOVELFORGE_CONFIG=<path>
./.novelforge/config.json
./.ainovel/config.json
~/.novelforge/config.json
~/.ainovel/config.json
built-in defaults
```

`--config` overrides `NOVELFORGE_CONFIG` because the CLI value is applied after process startup.

Rules:

1. An explicit configuration is authoritative and is not merged with project or global files.
2. At one scope, `.novelforge` completely shadows `.ainovel`; the two credential files are never merged.
3. The selected project layer overlays the selected global layer.
4. A corrupt selected project or explicit configuration fails loudly; NovelForge does not silently fall back to a lower-precedence peer.
5. A corrupt global layer can be ignored when a complete project layer is available, preserving the established project recovery behavior.
6. Reading a legacy fallback does not create `.novelforge` or modify `.ainovel`.
7. TUI `/config` and `/model` writes target the active explicit, project or global layer.

Examples:

```bash
novelforge --config /secure/config.json
NOVELFORGE_CONFIG=/secure/config.json novelforge --headless --prompt-file prompt.txt
```

## Rules directories

NovelForge uses the same new-path-first rule for writing preferences:

```text
./.novelforge/rules  >  ./.ainovel/rules
~/.novelforge/rules  >  ~/.ainovel/rules
```

One directory is selected per scope; new and legacy rule directories at the same scope are not combined. A fresh setup creates `~/.novelforge/rules`. The legacy command remains on `.ainovel/rules`.

## Inspect the current state

```bash
novelforge doctor
novelforge doctor --json
```

`doctor` reports:

- project-root accessibility;
- selected configuration source and layers;
- new/legacy shadowing;
- legacy fallback use;
- parse and runtime validation failures;
- explicit configuration resolution.

It never prints provider credentials or configuration contents.

Useful options:

```bash
novelforge doctor --project-root /path/to/book
novelforge doctor --home /temporary/home --json
novelforge doctor --config /secure/config.json
```

## Preview a migration

```bash
novelforge migrate --dry-run
```

By default both global and current-project legacy directories are considered. Limit the scope with:

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
2. rejects symbolic links and unsupported file types;
3. skips the scope when `.novelforge` already exists;
4. creates a timestamped backup under `.novelforge-migration-backups`;
5. records relative paths, file sizes and SHA-256 checksums in a manifest;
6. copies to a same-parent temporary directory;
7. writes a destination manifest;
8. atomically renames the temporary directory to `.novelforge`;
9. retains the original `.ainovel` directory.

No configuration contents are stored in the manifest.

## Failure and rollback behavior

Before destination commit, any error:

- leaves the original `.ainovel` directory unchanged;
- retains a completed pre-migration backup for diagnosis;
- removes the temporary staging directory;
- leaves no `.novelforge` destination;
- returns a non-zero exit status.

After migration, rollback is explicit and non-destructive:

1. stop NovelForge;
2. rename the new `.novelforge` directory out of the way;
3. run `novelforge doctor`;
4. NovelForge will select the retained `.ainovel` fallback;
5. only remove backups after independently verifying the replacement.

Do not delete either credential directory as part of an automated uninstall.

## Idempotence and conflicts

Re-running migration never overwrites an existing `.novelforge` directory. The action is reported as `destination_exists`.

The command intentionally does not merge directories. Review conflicts explicitly, then keep one complete configuration per scope.

## Backup layout

Global backups:

```text
~/.novelforge-migration-backups/<timestamp>-global-ainovel/
```

Project backups:

```text
<project>/.novelforge-migration-backups/<timestamp>-project-ainovel/
```

Both backup and destination contain `migration-manifest.json` with checksums and metadata only.

## Uninstall safety

`scripts/uninstall.sh` removes only the `novelforge` executable. It preserves:

```text
~/.novelforge
~/.ainovel
./.novelforge
./.ainovel
```

Configuration removal is intentionally manual because these locations can contain API keys and irreplaceable project rules.
