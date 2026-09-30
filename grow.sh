#!/bin/bash
# grow.sh - Drive a project toward its VISION.md. The loop lives in the Go
# binary (cmd/grange, internal/grow); this shim keeps project symlinks working.
#
# Usage: ./grow.sh [start|executor|planner|gap|oracle|visionary|status]
exec "$(cd "$(dirname "$(readlink -f "$0")")" && pwd)/lib/launch.sh" grow "$@"
