#!/bin/sh
# Guard active NovelForge packaging and usage examples against stale ainovel-cli branding.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
failed=false

reject() {
	pattern="$1"
	shift
	if grep -nE "$pattern" "$@"; then
		failed=true
	fi
}

reject 'ENTRYPOINT[[:space:]]*\[[[:space:]]*"ainovel-cli"' "$ROOT/Dockerfile"
reject 'binary:[[:space:]]*ainovel-cli' "$ROOT/.goreleaser.yml"
reject 'voocel/ainovel-cli/releases' "$ROOT/scripts/install.sh" "$ROOT/cmd/novelforge/main.go"
reject 'ainovel-cli[[:space:]]+server' "$ROOT/README.md" "$ROOT/docs/MIGRATION.md" "$ROOT/docs/DEVELOPMENT.md"
reject '/root/\.ainovel' "$ROOT/docker-compose.yml"

if [ "$failed" = true ]; then
	echo "NovelForge brand audit failed" >&2
	exit 1
fi

echo "NovelForge brand audit: PASS"
