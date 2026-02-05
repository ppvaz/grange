#!/bin/bash
# harvest.sh - Convert IKE knowledge prompts into spec library files
# Reads knowledge/prompts/PROMPT-NNN-name.md files from a project and
# creates corresponding spec files in specs/{project-name}/.
#
# Usage: ./harvest.sh /path/to/project          (convert and copy)
#        ./harvest.sh /path/to/project --dry-run (preview without writing)

set -euo pipefail

# Colors for logging
BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
NC='\033[0m'

log() {
  echo -e "$(date +%H:%M:%S) $1"
}

usage() {
  echo "Usage: $0 /path/to/project [--dry-run]"
  echo ""
  echo "Converts IKE knowledge/prompts/PROMPT-NNN-*.md files into spec format"
  echo "and copies them to specs/{project-name}/ in the grange repo."
  echo ""
  echo "Options:"
  echo "  --dry-run   Preview what would be created without writing files"
  exit 1
}

# Extract a section from a prompt file (content between ## Header and next ##)
extract_section() {
  local file=$1
  local header=$2
  sed -n "/^## ${header}/,/^## /{ /^## ${header}/d; /^## /d; p; }" "$file" | sed '/^$/{ N; /^\n$/d; }' | sed -e 's/^[[:space:]]*//' -e '/^$/d'
}

# Extract the title (first # line)
extract_title() {
  local file=$1
  sed -n 's/^# //p' "$file" | head -1
}

# Extract confidence from metadata line
extract_confidence() {
  local file=$1
  grep -i 'confidence' "$file" | head -1 | sed 's/.*: *//' || echo "N/A"
}

# Convert a single IKE prompt file to spec format
convert_prompt() {
  local input_file=$1
  local output_file=$2
  local dry_run=$3

  local title
  title=$(extract_title "$input_file")
  if [[ -z "$title" ]]; then
    title=$(basename "$input_file" .md | sed 's/^PROMPT-[0-9]*-//' | sed 's/-/ /g')
  fi

  local goal
  goal=$(extract_section "$input_file" "Goal")
  [[ -z "$goal" ]] && goal=$(extract_section "$input_file" "Business Goal")
  [[ -z "$goal" ]] && goal=$(extract_section "$input_file" "Objective")

  local inputs
  inputs=$(extract_section "$input_file" "Inputs")
  [[ -z "$inputs" ]] && inputs=$(extract_section "$input_file" "Data Needs")
  [[ -z "$inputs" ]] && inputs=$(extract_section "$input_file" "Required Data")

  local outputs
  outputs=$(extract_section "$input_file" "Outputs")
  [[ -z "$outputs" ]] && outputs=$(extract_section "$input_file" "Expected Output")
  [[ -z "$outputs" ]] && outputs=$(extract_section "$input_file" "Deliverables")

  local criteria
  criteria=$(extract_section "$input_file" "Acceptance Criteria")
  [[ -z "$criteria" ]] && criteria=$(extract_section "$input_file" "Success Criteria")
  [[ -z "$criteria" ]] && criteria=$(extract_section "$input_file" "Validation")

  local deps
  deps=$(extract_section "$input_file" "Dependencies")
  [[ -z "$deps" ]] && deps="None"

  local confidence
  confidence=$(extract_confidence "$input_file")

  if [[ "$dry_run" == "true" ]]; then
    log "${BLUE}[Dry Run]${NC} Would create: $output_file"
    log "  Title: $title"
    [[ -n "$goal" ]] && log "  Goal: $(echo "$goal" | head -1)"
    return 0
  fi

  cat > "$output_file" << SPEC
# ${title}

> Harvested from IKE extraction | Confidence: ${confidence}

## Business Requirement

${goal:-No goal extracted — review source prompt.}

## What We Need

### Required Operations

${outputs:-No outputs extracted — review source prompt.}

## Business Rules

${criteria:-No acceptance criteria extracted — review source prompt.}

## Data Needs

${inputs:-No inputs extracted — review source prompt.}

## Dependencies

${deps}

## Success Criteria

$(echo "$criteria" | sed 's/^- /- [ ] /' | sed 's/^[^-]/- [ ] &/')

## Reference

- Source: $(basename "$input_file")
- Harvested: $(date +%Y-%m-%d)
SPEC
}

main() {
  if [[ $# -lt 1 ]]; then
    usage
  fi

  local project_path="${1%/}"
  local dry_run="false"
  [[ "${2:-}" == "--dry-run" ]] && dry_run="true"

  if [[ ! -d "$project_path" ]]; then
    log "${RED}[Error]${NC} Directory not found: $project_path"
    exit 1
  fi

  local prompts_dir="$project_path/knowledge/prompts"
  if [[ ! -d "$prompts_dir" ]]; then
    log "${RED}[Error]${NC} No IKE prompts found at: $prompts_dir"
    log "  Expected: knowledge/prompts/PROMPT-NNN-name.md files"
    exit 1
  fi

  local project_name
  project_name=$(basename "$project_path" | sed 's/-experiment$//')

  local script_dir
  script_dir=$(cd "$(dirname "$0")" && pwd)
  local specs_dir="$script_dir/specs/$project_name"

  if [[ "$dry_run" == "false" ]]; then
    mkdir -p "$specs_dir"
  fi

  log "${BLUE}[Harvest]${NC} Project: $project_name"
  log "${BLUE}[Harvest]${NC} Source:  $prompts_dir"
  log "${BLUE}[Harvest]${NC} Target:  $specs_dir"
  [[ "$dry_run" == "true" ]] && log "${YELLOW}[Harvest]${NC} DRY RUN — no files will be written"
  echo ""

  local count=0
  local skipped=0

  for prompt_file in "$prompts_dir"/PROMPT-*.md; do
    [[ -f "$prompt_file" ]] || continue

    local basename_file
    basename_file=$(basename "$prompt_file")
    # Convert PROMPT-001-user-identity.md → user-identity.md
    local spec_name
    spec_name=$(echo "$basename_file" | sed 's/^PROMPT-[0-9]*-//')

    local output_file="$specs_dir/$spec_name"

    if [[ -f "$output_file" && "$dry_run" == "false" ]]; then
      log "${YELLOW}[Skip]${NC} Already exists: $spec_name"
      skipped=$((skipped + 1))
      continue
    fi

    convert_prompt "$prompt_file" "$output_file" "$dry_run"
    count=$((count + 1))
  done

  echo ""
  log "${GREEN}[Harvest]${NC} Done: $count specs ${dry_run:+would be }created, $skipped skipped (already exist)"
}

main "$@"
