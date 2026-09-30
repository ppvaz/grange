#!/bin/bash
# reap.sh - IKE v3: extract a codebase's implicit knowledge in six stages
# (0a → 0b → 1a → 1b → 2a → 2b), pausing for human review after each. The
# pipeline lives in the Go binary (internal/reap); this shim keeps project
# symlinks working.
#
# Usage: ./reap.sh start /path/to/target | resume [dir] | status [dir] | reset <stage> [dir]
exec "$(cd "$(dirname "$(readlink -f "$0")")" && pwd)/lib/launch.sh" reap "$@"
