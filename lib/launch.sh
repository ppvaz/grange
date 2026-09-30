#!/bin/bash
# lib/launch.sh - Run the grange binary, (re)building it when the Go sources
# are newer, so a `git pull` in grange reaches every adopted project.
# Usage: lib/launch.sh <grange subcommand> [args]

set -euo pipefail

GRANGE_HOME="$(cd "$(dirname "$(readlink -f "$0")")/.." && pwd)"
export GRANGE_HOME
bin="$GRANGE_HOME/bin/grange"

if [[ ! -x "$bin" ]] || [[ -n "$(find "$GRANGE_HOME/go.mod" "$GRANGE_HOME/cmd" "$GRANGE_HOME/internal" -newer "$bin" -print -quit)" ]]; then
  if ! command -v go > /dev/null; then
    echo "grange: building $bin needs Go (https://go.dev/dl)" >&2
    exit 1
  fi
  (cd "$GRANGE_HOME" && go build -o "$bin" ./cmd/grange) >&2
fi

exec "$bin" "$@"
