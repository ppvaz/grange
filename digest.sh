#!/bin/bash
# digest.sh - Summarise new BLOCKERS/CUTS/DRIFT/VISION_REVIEW entries into
# HUMAN_DIGEST.md. grow runs this itself once 10+ new lines pile up.
exec "$(cd "$(dirname "$(readlink -f "$0")")" && pwd)/lib/launch.sh" digest "$@"
