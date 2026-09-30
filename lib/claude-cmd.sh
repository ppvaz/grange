# lib/claude-cmd.sh - Claude command for each agent tier.
# Sourced by grow.sh, digest.sh and distill.sh after they load .env.
#
# CLAUDE_SMART_CMD / CLAUDE_CHEAP_CMD are split on whitespace (no quoting),
# e.g. CLAUDE_CHEAP_CMD="claude --model claude-sonnet-5-5". The first word must
# be an executable on PATH: these scripts run under bash and wrap agents in
# `timeout`, so aliases and functions from your interactive shell are invisible.

CLAUDE_SMART_CMD="${CLAUDE_SMART_CMD:-claude}"
CLAUDE_CHEAP_CMD="${CLAUDE_CHEAP_CMD:-claude-cheap}"
read -ra CLAUDE_SMART <<< "$CLAUDE_SMART_CMD"
read -ra CLAUDE_CHEAP <<< "$CLAUDE_CHEAP_CMD"

# Usage: require_claude_cmd CLAUDE_CHEAP_CMD
require_claude_cmd() {
  local var=$1
  local -a cmd
  read -ra cmd <<< "${!var}"
  type -P "${cmd[0]}" > /dev/null && return 0

  echo "Error: '${cmd[0]}' (from $var) is not an executable on PATH." >&2
  echo "  Shell aliases and functions aren't visible to grange scripts." >&2
  echo "  Set $var in .env, e.g. $var=\"claude --model <model-id>\"" >&2
  exit 1
}
