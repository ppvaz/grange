#!/bin/bash
# distill.sh - Discover cross-project patterns from domain specs
# Reads specs/{project}/, clusters by capability via LLM, generates
# anonymized pattern files in specs/patterns/.
#
# Usage: ./distill.sh              (discover and install patterns)
#        ./distill.sh --dry-run    (preview without writing)
#        ./distill.sh --clean      (wipe cache and re-run)

set -euo pipefail

# Early --help check before env loading
[[ "${1:-}" == "--help" ]] && {
  echo "Usage: $0 [--dry-run] [--clean] [--help]"
  echo ""
  echo "Discovers cross-project patterns from domain specs and generates"
  echo "anonymized pattern files in specs/patterns/."
  echo ""
  echo "Options:"
  echo "  --dry-run   Preview discovered clusters without writing pattern files"
  echo "  --clean     Wipe cached state and re-run from scratch"
  echo "  --help      Show this help message"
  exit 0
}

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
WORK_DIR="${WORK_DIR:-$SCRIPT_DIR}"
SPECS_DIR="$WORK_DIR/specs"
PATTERNS_DIR="$SPECS_DIR/patterns"
STATE_DIR="$WORK_DIR/.locks/generalize"

# Load environment variables from .env if present
if [[ -f "$WORK_DIR/.env" ]]; then
  set -a
  source "$WORK_DIR/.env"
  set +a
fi

GRANGE_HOME="$(cd "$(dirname "$(readlink -f "$0")")" && pwd)"
GRANGE="$GRANGE_HOME/lib/launch.sh"
# Model calls go through grange, which knows the configured backend
backend_check=$("$GRANGE" agent check Distill) || { echo "$backend_check" >&2; exit 1; }

# Colors for logging
BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
NC='\033[0m'

DRY_RUN=false

log() {
  echo -e "$(date +%H:%M:%S) $1"
}

usage() {
  echo "Usage: $0 [--dry-run] [--clean] [--help]"
  echo ""
  echo "Discovers cross-project patterns from domain specs and generates"
  echo "anonymized pattern files in specs/patterns/."
  echo ""
  echo "Options:"
  echo "  --dry-run   Preview discovered clusters without writing pattern files"
  echo "  --clean     Wipe cached state and re-run from scratch"
  echo "  --help      Show this help message"
  exit 0
}

# Ask the Distill role's backend (GRANGE_CHEAP unless GRANGE_AGENT_DISTILL), with retry.
# Only the answer reaches stdout; errors go to a log so they're never parsed as output.
call_llm() {
  local prompt=$1
  local errors="$STATE_DIR/llm-errors.log"
  local attempt
  for attempt in 1 2 3; do
    local result
    if result=$(printf '%s' "$prompt" | "$GRANGE" agent run --role Distill 2>> "$errors"); then
      echo "$result"
      return 0
    fi
    log "${YELLOW}[Distill]${NC} LLM call failed (attempt $attempt/3), retrying..."
    sleep $((attempt * 2))
  done
  log "${RED}[Distill]${NC} LLM call failed after 3 attempts; see $errors"
  return 1
}

# ─── Phase 1: Index ──────────────────────────────────────────────

build_catalog() {
  log "${BLUE}[Phase 1]${NC} Indexing domain specs..."
  local catalog="$STATE_DIR/catalog.txt"
  : > "$catalog"
  local count=0

  for project_dir in "$SPECS_DIR"/*/; do
    local project
    project=$(basename "$project_dir")
    [[ "$project" == "patterns" ]] && continue

    for spec_file in "$project_dir"*.md; do
      [[ -f "$spec_file" ]] || continue

      local relpath="${spec_file#"$WORK_DIR"/}"
      local title
      title=$(sed -n 's/^# //p' "$spec_file" | head -1)
      [[ -z "$title" ]] && title=$(basename "$spec_file" .md)

      local biz_req
      biz_req=$(sed -n '/^## Business Requirement/,/^## /{ /^## /d; p; }' "$spec_file" | sed '/^$/d' | head -1)

      local headers
      headers=$(grep '^## ' "$spec_file" | sed 's/^## //' | paste -sd',' -)

      echo "${relpath}|${project}|${title}|${biz_req}|${headers}" >> "$catalog"
      count=$((count + 1))
    done
  done

  log "${BLUE}[Phase 1]${NC} Cataloged $count specs from $(ls -d "$SPECS_DIR"/*/ 2>/dev/null | grep -cv patterns || echo 0) projects"
  if (( count == 0 )); then
    log "${YELLOW}[Distill]${NC} No domain specs found in specs/*/. Run harvest.sh first."
    exit 0
  fi
}

# ─── Phase 2: Cluster ────────────────────────────────────────────

cluster_specs() {
  local catalog="$STATE_DIR/catalog.txt"
  local clusters_file="$STATE_DIR/clusters.json"

  if [[ -f "$clusters_file" ]]; then
    log "${BLUE}[Phase 2]${NC} Using cached clusters"
    return 0
  fi

  log "${BLUE}[Phase 2]${NC} Clustering specs by capability..."

  # Collect existing pattern names
  local existing_patterns=""
  for p in "$PATTERNS_DIR"/*.md; do
    [[ -f "$p" ]] || continue
    existing_patterns="${existing_patterns}$(basename "$p" .md)\n"
  done

  local catalog_content
  catalog_content=$(cat "$catalog")

  local prompt
  prompt=$(cat << 'PROMPT_END'
You are analyzing a catalog of business specs from multiple software projects. Each line is pipe-delimited: filepath|project|title|first_line_of_business_requirement|section_headers

CATALOG:
PROMPT_END
)
  prompt="${prompt}
${catalog_content}

EXISTING PATTERNS (already extracted — skip these):
$(echo -e "$existing_patterns")

TASK:
1. Find specs from 2+ DIFFERENT projects that implement the same business capability.
2. Skip any cluster that matches an existing pattern name above.
3. Output ONLY a raw JSON array (no markdown fencing, no explanation).

Each element: {\"pattern\":\"kebab-case-name\",\"title\":\"Human Title\",\"rationale\":\"Why these are the same pattern\",\"specs\":[\"filepath1\",\"filepath2\",...]}

Output ONLY the JSON array. Nothing else."

  local result
  result=$(call_llm "$prompt") || { log "${RED}[Phase 2]${NC} Clustering failed"; exit 1; }

  # Extract JSON array from response (strip any surrounding text)
  local json
  json=$(echo "$result" | python3 -c "
import sys, json
text = sys.stdin.read()
# Find the JSON array in the response
start = text.find('[')
end = text.rfind(']')
if start == -1 or end == -1:
    print('[]')
    sys.exit(0)
arr = text[start:end+1]
parsed = json.loads(arr)
print(json.dumps(parsed, indent=2))
" 2>/dev/null) || { log "${RED}[Phase 2]${NC} Failed to parse JSON from LLM response"; echo "$result" > "$STATE_DIR/cluster_raw.txt"; exit 1; }

  echo "$json" > "$clusters_file"

  local cluster_count
  cluster_count=$(echo "$json" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))")
  log "${GREEN}[Phase 2]${NC} Found $cluster_count potential pattern clusters"

  if [[ "$DRY_RUN" == "true" ]]; then
    echo ""
    echo "$json" | python3 -c "
import sys, json
clusters = json.load(sys.stdin)
for c in clusters:
    print(f\"  {c['pattern']}: {c['title']}\")
    print(f\"    Rationale: {c['rationale']}\")
    print(f\"    Specs: {', '.join(c['specs'])}\")
    print()
"
  fi
}

# ─── Phase 3: Distill ────────────────────────────────────────────

distill_cluster() {
  local pattern_name=$1
  local title=$2
  local specs_json=$3  # JSON array of spec filepaths

  local output_file="$STATE_DIR/generated/${pattern_name}.md"
  if [[ -f "$output_file" ]]; then
    log "${BLUE}[Phase 3]${NC} Skipping $pattern_name (already generated)"
    return 0
  fi

  log "${BLUE}[Phase 3]${NC} Distilling: $pattern_name"

  # Read exemplar
  local exemplar
  exemplar=$(cat "$PATTERNS_DIR/entity-crud.md")

  # Read all referenced spec files
  local spec_contents=""
  local spec_paths
  spec_paths=$(echo "$specs_json" | python3 -c "import sys,json; [print(s) for s in json.load(sys.stdin)]")

  while IFS= read -r spec_path; do
    local full_path="$WORK_DIR/$spec_path"
    if [[ -f "$full_path" ]]; then
      spec_contents="${spec_contents}
--- FILE: ${spec_path} ---
$(cat "$full_path")
"
    else
      log "${YELLOW}[Phase 3]${NC} Spec not found: $spec_path"
    fi
  done <<< "$spec_paths"

  local prompt
  prompt="You are generating a cross-project pattern spec. Read the source specs below and produce ONE anonymized pattern document.

RULES:
- Anonymize ALL client/project names. Use \"Platform A\", \"Platform B\", etc.
- Remove any company names, developer names, or identifying details
- Match the EXACT section order from the FORMAT EXEMPLAR below
- Include a Customization Variables table with \$VARIABLE placeholders
- Include a Business Value section with a project implementation table (anonymized)
- Output raw markdown only. No code fences. No explanation before or after.

PATTERN NAME: ${pattern_name}
PATTERN TITLE: ${title}

FORMAT EXEMPLAR:
${exemplar}

SOURCE SPECS:
${spec_contents}

Generate the pattern document now. Output ONLY the markdown content."

  local result
  result=$(call_llm "$prompt") || { log "${RED}[Phase 3]${NC} Failed to distill $pattern_name"; return 1; }

  # Strip any markdown code fences if the LLM wrapped it
  result=$(echo "$result" | sed '/^```markdown$/d; /^```$/d; /^```md$/d')

  echo "$result" > "$output_file"

  # Validate required sections
  local required_sections=("Business Requirement" "What We Need" "Business Rules" "Success Criteria" "Business Value")
  local missing=()
  for section in "${required_sections[@]}"; do
    if ! grep -q "^## ${section}" "$output_file"; then
      missing+=("$section")
    fi
  done

  if (( ${#missing[@]} > 0 )); then
    log "${YELLOW}[Phase 3]${NC} Missing sections in $pattern_name: ${missing[*]}. Retrying..."
    rm "$output_file"
    result=$(call_llm "$prompt") || { log "${RED}[Phase 3]${NC} Retry failed for $pattern_name"; return 1; }
    result=$(echo "$result" | sed '/^```markdown$/d; /^```$/d; /^```md$/d')
    echo "$result" > "$output_file"

    # Check again
    local still_missing=false
    for section in "${required_sections[@]}"; do
      if ! grep -q "^## ${section}" "$output_file"; then
        still_missing=true
        break
      fi
    done
    if [[ "$still_missing" == "true" ]]; then
      log "${YELLOW}[Phase 3]${NC} $pattern_name still missing sections after retry — keeping best effort"
    fi
  fi

  log "${GREEN}[Phase 3]${NC} Generated: $pattern_name"
}

distill_all() {
  local clusters_file="$STATE_DIR/clusters.json"
  mkdir -p "$STATE_DIR/generated"

  local cluster_count
  cluster_count=$(python3 -c "import sys,json; print(len(json.load(open('$clusters_file'))))")

  if (( cluster_count == 0 )); then
    log "${YELLOW}[Phase 3]${NC} No clusters to distill"
    return 0
  fi

  log "${BLUE}[Phase 3]${NC} Distilling $cluster_count clusters..."

  python3 -c "
import json
clusters = json.load(open('$clusters_file'))
for c in clusters:
    specs = json.dumps(c['specs'])
    print(f\"{c['pattern']}|{c['title']}|{specs}\")
" | while IFS='|' read -r pattern_name title specs_json; do
    if [[ "$DRY_RUN" == "true" ]]; then
      log "${BLUE}[Dry Run]${NC} Would distill: $pattern_name ($title)"
    else
      distill_cluster "$pattern_name" "$title" "$specs_json"
    fi
  done
}

# ─── Phase 4: Dedup + Install ────────────────────────────────────

dedup_and_install() {
  local generated_dir="$STATE_DIR/generated"
  [[ -d "$generated_dir" ]] || return 0

  log "${BLUE}[Phase 4]${NC} Deduplicating and installing patterns..."

  local installed=0
  local skipped=0

  for gen_file in "$generated_dir"/*.md; do
    [[ -f "$gen_file" ]] || continue

    local name
    name=$(basename "$gen_file" .md)
    local target="$PATTERNS_DIR/${name}.md"

    # Tier 1: exact filename match
    if [[ -f "$target" ]]; then
      log "${YELLOW}[Skip]${NC} $name — already exists (filename match)"
      skipped=$((skipped + 1))
      continue
    fi

    # Tier 2: fuzzy title match against existing patterns
    local gen_title
    gen_title=$(sed -n 's/^# //p' "$gen_file" | head -1)
    local duplicate=false

    if [[ -n "$gen_title" ]]; then
      for existing in "$PATTERNS_DIR"/*.md; do
        [[ -f "$existing" ]] || continue
        local existing_title
        existing_title=$(sed -n 's/^# //p' "$existing" | head -1)
        # Check if generated title is substring of existing (case-insensitive)
        if echo "$existing_title" | grep -qiF "$gen_title"; then
          log "${YELLOW}[Skip]${NC} $name — title matches existing: $(basename "$existing" .md)"
          duplicate=true
          skipped=$((skipped + 1))
          break
        fi
        # Check if existing title is substring of generated
        if echo "$gen_title" | grep -qiF "$existing_title"; then
          log "${YELLOW}[Skip]${NC} $name — title matches existing: $(basename "$existing" .md)"
          duplicate=true
          skipped=$((skipped + 1))
          break
        fi
      done
    fi
    [[ "$duplicate" == "true" ]] && continue

    # Install
    if [[ "$DRY_RUN" == "true" ]]; then
      log "${BLUE}[Dry Run]${NC} Would install: $name"
    else
      cp "$gen_file" "$target"
      log "${GREEN}[Install]${NC} $name → specs/patterns/${name}.md"
    fi
    installed=$((installed + 1))
  done

  echo ""
  if [[ "$DRY_RUN" == "true" ]]; then
    log "${GREEN}[Phase 4]${NC} Would install $installed new patterns ($skipped skipped as duplicates)"
  else
    log "${GREEN}[Phase 4]${NC} Installed $installed new patterns ($skipped skipped as duplicates)"
  fi
}

# ─── Main ─────────────────────────────────────────────────────────

main() {
  local clean=false

  while [[ $# -gt 0 ]]; do
    case $1 in
      --dry-run) DRY_RUN=true; shift ;;
      --clean)   clean=true; shift ;;
      --help)    usage ;;
      *) log "${RED}[Error]${NC} Unknown option: $1"; usage ;;
    esac
  done

  # Validate specs directory
  if [[ ! -d "$SPECS_DIR" ]]; then
    log "${RED}[Error]${NC} Specs directory not found: $SPECS_DIR"
    exit 1
  fi

  if [[ "$clean" == "true" ]]; then
    log "${YELLOW}[Distill]${NC} Cleaning cached state..."
    rm -rf "$STATE_DIR"
  fi

  mkdir -p "$STATE_DIR"

  [[ "$DRY_RUN" == "true" ]] && log "${YELLOW}[Distill]${NC} DRY RUN — no files will be written"
  echo ""

  build_catalog
  cluster_specs
  distill_all
  dedup_and_install

  echo ""
  log "${GREEN}[Distill]${NC} Done"
}

main "$@"
