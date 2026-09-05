#!/bin/sh
# Run an extracted candidate on loopback. No credentials are written by this helper.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
BIN="$ROOT/novelforge"
[ -x "$BIN" ] || { echo 'Extract the matching release archive before using this helper.' >&2; exit 1; }
WORKSPACE="${NOVELFORGE_WORKSPACE:-$ROOT/workspace}"
if [ -n "${NOVELFORGE_CONFIG:-}" ]; then
  exec "$BIN" server --host 127.0.0.1 --port "${NOVELFORGE_PORT:-48090}" --workspace "$WORKSPACE" --config "$NOVELFORGE_CONFIG" "$@"
fi
exec "$BIN" server --host 127.0.0.1 --port "${NOVELFORGE_PORT:-48090}" --workspace "$WORKSPACE" "$@"
