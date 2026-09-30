#!/bin/bash
# digest.sh - Generate human review digest from observation files
# Compiles new entries from BLOCKERS.md, CUTS.md, DRIFT.md, VISION_REVIEW.md
# into HUMAN_DIGEST.md for async human review.
#
# Usage: ./digest.sh          (manual run)
#        Called automatically by grow.sh on signal threshold or daily schedule

set -euo pipefail

WORK_DIR="${WORK_DIR:-.}"
LOCK_DIR="$WORK_DIR/.locks"
MARKERS_FILE="$LOCK_DIR/digest_markers"
OUTPUT_FILE="$WORK_DIR/HUMAN_DIGEST.md"

# Load environment variables from .env if present
if [[ -f "$WORK_DIR/.env" ]]; then
  set -a
  source "$WORK_DIR/.env"
  set +a
fi

GRANGE_HOME="$(cd "$(dirname "$(readlink -f "$0")")" && pwd)"
source "$GRANGE_HOME/lib/claude-cmd.sh"
require_claude_cmd CLAUDE_CHEAP_CMD

# Observation files to digest
OBSERVATION_FILES=(
  "BLOCKERS.md"
  "CUTS.md"
  "DRIFT.md"
  "VISION_REVIEW.md"
)

# Colors for logging
BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

log() {
  echo -e "$(date +%H:%M:%S) $1"
}

# Get marker (last digested line count) for a file
get_marker() {
  local file=$1
  local marker=0
  if [[ -f "$MARKERS_FILE" ]]; then
    marker=$(grep "^${file}:" "$MARKERS_FILE" 2>/dev/null | cut -d: -f2 || echo 0)
  fi
  echo "${marker:-0}"
}

# Get current line count for a file
get_line_count() {
  local file="$WORK_DIR/$1"
  if [[ -f "$file" ]]; then
    wc -l < "$file"
  else
    echo 0
  fi
}

# Get new lines since marker
get_new_lines() {
  local file=$1
  local marker=$2
  local filepath="$WORK_DIR/$file"

  if [[ ! -f "$filepath" ]]; then
    echo ""
    return
  fi

  local start=$((marker + 1))
  tail -n "+$start" "$filepath" 2>/dev/null || echo ""
}

# Save all markers atomically
save_markers() {
  local temp_markers="$MARKERS_FILE.tmp"
  : > "$temp_markers"

  for file in "${OBSERVATION_FILES[@]}"; do
    local count
    count=$(get_line_count "$file")
    echo "${file}:${count}" >> "$temp_markers"
  done

  mv "$temp_markers" "$MARKERS_FILE"
}

# Build digest prompt with new content
build_prompt() {
  local total_new=0
  local content_sections=""
  local metrics=""

  for file in "${OBSERVATION_FILES[@]}"; do
    local marker
    marker=$(get_marker "$file")
    local current
    current=$(get_line_count "$file")
    local new_lines=$((current - marker))

    if (( new_lines > 0 )); then
      total_new=$((total_new + new_lines))
      metrics="${metrics}\n- ${file}: +${new_lines} lines"

      local new_content
      new_content=$(get_new_lines "$file" "$marker")
      if [[ -n "$new_content" ]]; then
        content_sections="${content_sections}

### ${file}
\`\`\`
${new_content}
\`\`\`"
      fi
    else
      metrics="${metrics}\n- ${file}: no new content"
    fi
  done

  if (( total_new == 0 )); then
    echo ""
    return
  fi

  cat << EOF
You are a digest compiler for a multi-agent development system. Your task is to summarize new observations from agent activity into a concise human-readable digest.

## New Content Since Last Digest
${metrics}

## Raw Content
${content_sections}

## Instructions

Create a digest with this exact structure:

# Human Digest
Generated: $(date +"%Y-%m-%d %H:%M")

## Summary
[2-3 sentences summarizing the overall state and any urgent items]

## New Since Last Digest
${metrics}

## Observations
[Bullet points summarizing key items from each file with new content, grouped by source]

## Patterns
[Any cross-file themes or recurring issues you notice]

## Suggested Actions
1. [High impact action if any]
2. [Medium impact action if any]

**Estimated Review Time**: [estimate in minutes based on content volume]

---

Be concise. Focus on what a human needs to know to make decisions. Skip empty sections.
EOF
}

main() {
  mkdir -p "$LOCK_DIR"

  # Build the prompt (includes check for new content)
  local prompt
  prompt=$(build_prompt)

  if [[ -z "$prompt" ]]; then
    log "${YELLOW}[Digest]${NC} No new observations since last digest"
    exit 0
  fi

  log "${BLUE}[Digest]${NC} Compiling digest from observation files..."

  local result
  if result=$("${CLAUDE_CHEAP[@]}" -p "$prompt" 2>&1); then
    # Write output to HUMAN_DIGEST.md (append with separator)
    {
      echo ""
      echo "---"
      echo ""
      echo "$result"
    } >> "$OUTPUT_FILE"

    # Update markers
    save_markers

    log "${GREEN}[Digest]${NC} Digest written to $OUTPUT_FILE"
  else
    log "${YELLOW}[Digest]${NC} Failed to generate digest: $result"
    exit 1
  fi
}

main "$@"
