#!/bin/bash
# reap.sh - IKE v3 Sequential Wizard
# Walks through the 6 IKE stages (0a → 0b → 1a → 1b → 2a → 2b),
# running grow.sh for each stage and pausing for human review.
#
# Usage: ./reap.sh start /path/to/target-project
#        ./reap.sh resume [work-dir]
#        ./reap.sh status [work-dir]
#        ./reap.sh reset <stage-index> [work-dir]

set -euo pipefail

# ============================================
# COLORS & LOGGING (matches grow.sh)
# ============================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

log() {
  echo -e "$(date +%H:%M:%S) $1"
}

# ============================================
# STAGE DEFINITIONS (parallel arrays)
# ============================================

STAGE_IDS=(    "0a"            "0b"          "1a"              "1b"             "2a"        "2b"       )
STAGE_NAMES=(  "Lens Analysis" "Synthesis"   "Deep Extraction" "Prompts & Review" "Build"   "Verify"   )
STAGE_VISIONS=(
  "VISION-stage0a-lenses.md"
  "VISION-stage0b-synthesis.md"
  "VISION-stage1a-extraction.md"
  "VISION-stage1b-prompts.md"
  "VISION-stage2a-build.md"
  "VISION-stage2b-verify.md"
)
STAGE_DESCRIPTIONS=(
  "Run three parallel analyses (BA, PM, QA lenses) of the target codebase"
  "Reconcile lenses with your context, generate extraction spec"
  "Extract entities, rules, flows, integrations with confidence scores"
  "Generate atomic prompts, dependency graph, adversarial review"
  "Pre-build decisions, project setup, implement prompts"
  "Full test suite, flow verification, risk resolution, final report"
)
STAGE_CHECKPOINTS=(
  "Review lens files → Create recon/HUMAN-CONTEXT.md"
  "Review VISION-stage1-extraction.md → Check BLOCKERS.md"
  "Review CONFIDENCE-SUMMARY.md → Validate low-confidence items"
  "Review RISK-REGISTER.md → Check EXTRACTION-COMPLETE.md"
  "Review PROGRESS.md → Unblock issues → Add docs/HUMAN-DECISIONS.md"
  "Review REBUILD-COMPLETE.md → Ship or iterate"
)

NUM_STAGES=${#STAGE_IDS[@]}

# macOS detection (matches grow.sh)
IS_MACOS=false
[[ "$(uname)" == "Darwin" ]] && IS_MACOS=true

# Portable sed -i
sed_i() {
  if [[ "$IS_MACOS" == true ]]; then
    sed -i '' "$@"
  else
    sed -i "$@"
  fi
}

# ============================================
# HELPERS
# ============================================

usage() {
  echo "Usage: $0 <command> [args]"
  echo ""
  echo "IKE v3 Sequential Wizard — walk through the 6 extraction stages"
  echo "with human checkpoints between each grow.sh run."
  echo ""
  echo "Commands:"
  echo "  start  /path/to/project  Begin pipeline from Stage 0a"
  echo "  resume [work-dir]        Resume from last completed stage (default: cwd)"
  echo "  status [work-dir]        Show pipeline progress"
  echo "  reset  <stage> [work-dir] Jump to stage N (0-$((NUM_STAGES - 1)))"
  echo ""
  echo "Stages:"
  for i in "${!STAGE_IDS[@]}"; do
    printf "  %d  %-4s %-20s %s\n" "$i" "${STAGE_IDS[$i]}" "${STAGE_NAMES[$i]}" "${STAGE_DESCRIPTIONS[$i]}"
  done
  exit 1
}

banner() {
  local idx=$1
  local id="${STAGE_IDS[$idx]}"
  local name="${STAGE_NAMES[$idx]}"
  local desc="${STAGE_DESCRIPTIONS[$idx]}"

  echo ""
  log "${GREEN}══════════════════════════════════════════════════════${NC}"
  log "${GREEN}  Stage ${id}: ${BOLD}${name}${NC}${GREEN}  ($((idx + 1))/${NUM_STAGES})${NC}"
  log "${GREEN}══════════════════════════════════════════════════════${NC}"
  log "${DIM}${desc}${NC}"
  echo ""
}

resolve_grange() {
  # Find grow.sh: script dir → GRANGE_DIR env → WORK_DIR
  local script_dir
  script_dir=$(cd "$(dirname "$0")" && pwd)

  if [[ -f "$script_dir/grow.sh" ]]; then
    GRANGE_DIR="$script_dir"
  elif [[ -n "${GRANGE_DIR:-}" && -f "$GRANGE_DIR/grow.sh" ]]; then
    : # Already set
  elif [[ -f "$WORK_DIR/grow.sh" ]]; then
    GRANGE_DIR="$WORK_DIR"
  else
    log "${RED}[Error]${NC} Cannot find grow.sh"
    log "  Searched: $script_dir, \$GRANGE_DIR, $WORK_DIR"
    exit 1
  fi

  GROW_SH="$GRANGE_DIR/grow.sh"
  VISIONS_DIR="$GRANGE_DIR/visions/ike-v3"

  if [[ ! -d "$VISIONS_DIR" ]]; then
    log "${RED}[Error]${NC} Vision templates not found at $VISIONS_DIR"
    exit 1
  fi
}

# ============================================
# STATE MANAGEMENT
# ============================================

STATE_FILE=""  # Set after WORK_DIR is known

init_state_file() {
  STATE_FILE="$WORK_DIR/.ike-state"
}

load_state() {
  if [[ -f "$STATE_FILE" ]]; then
    set -a
    source "$STATE_FILE"
    set +a
  fi
  # Defaults
  CURRENT_STAGE="${CURRENT_STAGE:--1}"
  TARGET_PATH="${TARGET_PATH:-}"
  TARGET_STACK="${TARGET_STACK:-}"
  BUILD_DIR="${BUILD_DIR:-}"
  GRANGE_DIR="${GRANGE_DIR:-}"
  STARTED_AT="${STARTED_AT:-}"
}

write_state() {
  cat > "$STATE_FILE" << EOF
CURRENT_STAGE=$CURRENT_STAGE
TARGET_PATH=$TARGET_PATH
TARGET_STACK=$TARGET_STACK
BUILD_DIR=$BUILD_DIR
GRANGE_DIR=$GRANGE_DIR
STARTED_AT=$STARTED_AT
EOF
}

# ============================================
# STAGE OPERATIONS
# ============================================

prepare_vision() {
  local idx=$1
  local template="${STAGE_VISIONS[$idx]}"
  local vision_src="$VISIONS_DIR/$template"

  # Stage 1a special: use generated extraction spec from 0b if it exists
  if [[ $idx -eq 2 ]]; then
    local generated="$WORK_DIR/VISION-stage1-extraction.md"
    if [[ -f "$generated" ]]; then
      log "${BLUE}[Vision]${NC} Using generated extraction spec from Stage 0b"
      cp "$generated" "$WORK_DIR/VISION.md"
      return 0
    else
      log "${YELLOW}[Vision]${NC} No generated extraction spec found, using template"
      log "${YELLOW}        ${NC} (Stage 0b should have created VISION-stage1-extraction.md)"
    fi
  fi

  # Stage 2a special: prompt for stack and build dir
  if [[ $idx -eq 4 ]]; then
    if [[ -z "$TARGET_STACK" ]]; then
      echo ""
      log "${BLUE}[Setup]${NC} Stage 2a needs a target technology stack."
      printf "  Target stack (e.g. 'Node.js + TypeScript + PostgreSQL'): "
      read -r TARGET_STACK
      if [[ -z "$TARGET_STACK" ]]; then
        log "${YELLOW}[Warning]${NC} No stack specified, using placeholder"
        TARGET_STACK="[DEFINE YOUR TARGET STACK]"
      fi
    fi

    if [[ -z "$BUILD_DIR" ]]; then
      echo ""
      log "${BLUE}[Setup]${NC} Build directory for the new project."
      log "${DIM}         Default: $WORK_DIR (same directory)${NC}"
      printf "  Build directory [Enter for default]: "
      read -r BUILD_DIR
      if [[ -z "$BUILD_DIR" ]]; then
        BUILD_DIR="$WORK_DIR"
      else
        # Expand to absolute path
        BUILD_DIR=$(cd "$(dirname "$BUILD_DIR")" 2>/dev/null && echo "$(pwd)/$(basename "$BUILD_DIR")" || echo "$BUILD_DIR")
        if [[ ! -d "$BUILD_DIR" ]]; then
          log "${BLUE}[Setup]${NC} Creating build directory: $BUILD_DIR"
          mkdir -p "$BUILD_DIR"
        fi
        # Copy knowledge/ to build dir
        if [[ -d "$WORK_DIR/knowledge" ]]; then
          log "${BLUE}[Setup]${NC} Copying knowledge/ to build directory"
          cp -r "$WORK_DIR/knowledge" "$BUILD_DIR/"
        fi
      fi
    fi
    write_state
  fi

  if [[ ! -f "$vision_src" ]]; then
    log "${RED}[Error]${NC} Vision template not found: $vision_src"
    exit 1
  fi

  cp "$vision_src" "$WORK_DIR/VISION.md"

  # Apply placeholder substitutions
  if [[ $idx -eq 0 && -n "$TARGET_PATH" ]]; then
    sed_i "s|\[TARGET_CODEBASE_PATH\]|$TARGET_PATH|g" "$WORK_DIR/VISION.md"
  fi

  if [[ $idx -eq 4 && -n "$TARGET_STACK" ]]; then
    sed_i "s|\[DEFINE YOUR TARGET STACK\]|$TARGET_STACK|g" "$WORK_DIR/VISION.md"
  fi

  log "${BLUE}[Vision]${NC} Copied ${template} → VISION.md"
}

clean_stage() {
  # Remove per-stage transient files; preserve cumulative artifacts
  local dir="$1"
  rm -f "$dir/DONE.md"
  rm -f "$dir/PLAN.md" && touch "$dir/PLAN.md"
  rm -f "$dir/BLOCKERS.md" && touch "$dir/BLOCKERS.md"
  rm -f "$dir/CUTS.md" && touch "$dir/CUTS.md"
  rm -f "$dir/DRIFT.md" && touch "$dir/DRIFT.md"
  rm -f "$dir/VISION_REVIEW.md"
  rm -rf "$dir/.locks"
  rm -f "$dir/LOG.md"

  log "${DIM}[Clean]${NC} ${DIM}Reset transient files for new stage${NC}"
}

show_artifacts() {
  local idx=$1
  local dir="$2"

  echo ""
  log "${GREEN}[Artifacts]${NC} Key outputs from Stage ${STAGE_IDS[$idx]}:"

  case $idx in
    0)  # 0a — Lenses
      for f in "$dir"/recon/lens-*.md; do
        [[ -f "$f" ]] && log "  ${GREEN}✓${NC} $(basename "$f")"
      done
      ;;
    1)  # 0b — Synthesis
      for f in "$dir"/recon/synthesis.md "$dir"/recon/hotspot-map.md "$dir"/recon/complexity-assessment.md "$dir/VISION-stage1-extraction.md"; do
        [[ -f "$f" ]] && log "  ${GREEN}✓${NC} $(basename "$f")"
      done
      ;;
    2)  # 1a — Extraction
      [[ -d "$dir/knowledge" ]] && {
        local entity_count rule_count flow_count int_count
        entity_count=$(find "$dir/knowledge/entities" -name '*.md' 2>/dev/null | wc -l)
        rule_count=$(find "$dir/knowledge/rules" -name '*.md' 2>/dev/null | wc -l)
        flow_count=$(find "$dir/knowledge/flows" -name '*.md' 2>/dev/null | wc -l)
        int_count=$(find "$dir/knowledge/integrations" -name '*.md' 2>/dev/null | wc -l)
        log "  ${GREEN}✓${NC} knowledge/entities/ ($entity_count files)"
        log "  ${GREEN}✓${NC} knowledge/rules/ ($rule_count files)"
        log "  ${GREEN}✓${NC} knowledge/flows/ ($flow_count files)"
        log "  ${GREEN}✓${NC} knowledge/integrations/ ($int_count files)"
      }
      [[ -f "$dir/knowledge/CONFIDENCE-SUMMARY.md" ]] && log "  ${GREEN}✓${NC} CONFIDENCE-SUMMARY.md"
      ;;
    3)  # 1b — Prompts
      if [[ -d "$dir/knowledge/prompts" ]]; then
        local prompt_count
        prompt_count=$(find "$dir/knowledge/prompts" -name 'PROMPT-*.md' 2>/dev/null | wc -l)
        log "  ${GREEN}✓${NC} knowledge/prompts/ ($prompt_count prompts)"
      fi
      [[ -f "$dir/knowledge/prompts/DEPENDENCY-GRAPH.md" ]] && log "  ${GREEN}✓${NC} DEPENDENCY-GRAPH.md"
      [[ -f "$dir/knowledge/RISK-REGISTER.md" ]] && log "  ${GREEN}✓${NC} RISK-REGISTER.md"
      [[ -f "$dir/knowledge/EXTRACTION-COMPLETE.md" ]] && log "  ${GREEN}✓${NC} EXTRACTION-COMPLETE.md"
      ;;
    4)  # 2a — Build
      [[ -d "$dir/src" ]] && log "  ${GREEN}✓${NC} src/"
      [[ -d "$dir/tests" ]] && log "  ${GREEN}✓${NC} tests/"
      [[ -f "$dir/docs/TRACEABILITY.md" ]] && log "  ${GREEN}✓${NC} docs/TRACEABILITY.md"
      [[ -f "$dir/docs/PROGRESS.md" ]] && log "  ${GREEN}✓${NC} docs/PROGRESS.md"
      [[ -f "$dir/docs/DISCOVERED-GAPS.md" ]] && log "  ${GREEN}✓${NC} docs/DISCOVERED-GAPS.md"
      ;;
    5)  # 2b — Verify
      for f in TEST-RESULTS.md FLOW-VERIFICATION.md RISK-RESOLUTION.md REBUILD-COMPLETE.md KNOWLEDGE-GAPS.md; do
        [[ -f "$dir/docs/$f" ]] && log "  ${GREEN}✓${NC} docs/$f"
      done
      ;;
  esac
  echo ""
}

prompt_continue() {
  local idx=$1
  local checkpoint="${STAGE_CHECKPOINTS[$idx]}"

  log "${YELLOW}╔══════════════════════════════════════════════════════╗${NC}"
  log "${YELLOW}║  CHECKPOINT — Human Review Required                 ║${NC}"
  log "${YELLOW}╚══════════════════════════════════════════════════════╝${NC}"
  echo ""
  log "${BOLD}Your task:${NC} $checkpoint"
  echo ""

  if [[ $((idx + 1)) -lt $NUM_STAGES ]]; then
    local next_name="${STAGE_NAMES[$((idx + 1))]}"
    log "${DIM}Next: Stage ${STAGE_IDS[$((idx + 1))]} — ${next_name}${NC}"
  else
    log "${GREEN}This was the final stage!${NC}"
  fi
  echo ""

  while true; do
    printf "  ${BOLD}[Enter]${NC} continue  |  ${BOLD}[r]${NC} rerun stage  |  ${BOLD}[q]${NC} quit > "
    read -r choice
    case "${choice:-}" in
      "")  return 0 ;;     # Continue to next stage
      r|R) return 1 ;;     # Rerun same stage
      q|Q)
        log "${YELLOW}[IKE]${NC} Paused. Resume with: $0 resume $WORK_DIR"
        exit 0
        ;;
      *)
        echo "  Invalid choice. Press Enter, 'r', or 'q'."
        ;;
    esac
  done
}

# ============================================
# CORE STAGE RUNNER
# ============================================

run_grow() {
  local dir="$1"
  (cd "$dir" && bash "$GROW_SH" start)
}

run_stage() {
  local idx=$1

  banner "$idx"

  # Determine working directory (stages 2a/2b may use BUILD_DIR)
  local stage_dir="$WORK_DIR"
  if [[ $idx -ge 4 && -n "$BUILD_DIR" && "$BUILD_DIR" != "$WORK_DIR" ]]; then
    stage_dir="$BUILD_DIR"
    log "${BLUE}[IKE]${NC} Using build directory: $stage_dir"
  fi

  clean_stage "$stage_dir"
  prepare_vision "$idx"

  # If using a separate build dir, copy VISION.md there
  if [[ "$stage_dir" != "$WORK_DIR" ]]; then
    cp "$WORK_DIR/VISION.md" "$stage_dir/VISION.md"
  fi

  log "${BLUE}[IKE]${NC} Starting grow.sh..."
  echo ""

  local grow_exit=0
  run_grow "$stage_dir" || grow_exit=$?

  if [[ $grow_exit -ne 0 ]]; then
    log "${YELLOW}[IKE]${NC} grow.sh exited with code $grow_exit"
  fi

  # Check for DONE.md
  if [[ -f "$stage_dir/DONE.md" ]]; then
    log "${GREEN}[IKE]${NC} Stage ${STAGE_IDS[$idx]} completed — DONE.md found"
  else
    log "${YELLOW}[IKE]${NC} Stage ${STAGE_IDS[$idx]} ended without DONE.md"
    log "${YELLOW}       ${NC} grow.sh may have been interrupted"
  fi

  show_artifacts "$idx" "$stage_dir"

  # Checkpoint prompt
  local continue_result=0
  prompt_continue "$idx" || continue_result=$?

  if [[ $continue_result -eq 1 ]]; then
    # Rerun same stage
    log "${BLUE}[IKE]${NC} Rerunning Stage ${STAGE_IDS[$idx]}..."
    run_stage "$idx"
    return $?
  fi

  # Mark stage complete
  CURRENT_STAGE=$idx
  write_state
}

# ============================================
# STATUS DISPLAY
# ============================================

show_status() {
  local dir="${1:-.}"
  local state_file="$dir/.ike-state"

  if [[ ! -f "$state_file" ]]; then
    log "${YELLOW}[Status]${NC} No .ike-state found in $dir"
    exit 1
  fi

  STATE_FILE="$state_file"
  load_state

  echo ""
  log "${BOLD}IKE v3 Pipeline Status${NC}"
  log "${DIM}Work directory: $dir${NC}"
  [[ -n "$TARGET_PATH" ]] && log "${DIM}Target codebase: $TARGET_PATH${NC}"
  [[ -n "$STARTED_AT" ]] && log "${DIM}Started: $STARTED_AT${NC}"
  echo ""

  for i in "${!STAGE_IDS[@]}"; do
    local marker=" "
    local color="$DIM"
    if [[ $i -le $CURRENT_STAGE ]]; then
      marker="✓"
      color="$GREEN"
    elif [[ $i -eq $((CURRENT_STAGE + 1)) ]]; then
      marker="▶"
      color="$YELLOW"
    fi
    printf "  ${color}[%s] %d  %-4s %s${NC}\n" "$marker" "$i" "${STAGE_IDS[$i]}" "${STAGE_NAMES[$i]}"
  done

  echo ""
  if [[ $CURRENT_STAGE -ge $((NUM_STAGES - 1)) ]]; then
    log "${GREEN}Pipeline complete!${NC}"
  else
    local next=$((CURRENT_STAGE + 1))
    log "Next: Stage ${STAGE_IDS[$next]} — ${STAGE_NAMES[$next]}"
    log "Run: ${BOLD}$0 resume $dir${NC}"
  fi
  echo ""
}

# ============================================
# RESET
# ============================================

reset_to() {
  local target_stage=$1
  local dir="${2:-.}"
  local state_file="$dir/.ike-state"

  if [[ ! -f "$state_file" ]]; then
    log "${RED}[Error]${NC} No .ike-state found in $dir"
    exit 1
  fi

  if [[ $target_stage -lt 0 || $target_stage -ge $NUM_STAGES ]]; then
    log "${RED}[Error]${NC} Stage must be 0-$((NUM_STAGES - 1))"
    exit 1
  fi

  STATE_FILE="$state_file"
  WORK_DIR="$dir"
  load_state

  CURRENT_STAGE=$((target_stage - 1))
  write_state

  log "${BLUE}[Reset]${NC} Pipeline will resume from Stage ${STAGE_IDS[$target_stage]} — ${STAGE_NAMES[$target_stage]}"
  log "Run: ${BOLD}$0 resume $dir${NC}"
}

# ============================================
# SIGINT TRAP
# ============================================

handle_interrupt() {
  echo ""
  log "${YELLOW}[IKE]${NC} Interrupted."
  log "${YELLOW}      ${NC} Resume with: $0 resume ${WORK_DIR:-.}"
  exit 130
}

# ============================================
# MAIN
# ============================================

main_start() {
  local target_path="${1:-}"

  if [[ -z "$target_path" ]]; then
    log "${RED}[Error]${NC} Missing target codebase path"
    echo "  Usage: $0 start /path/to/target-project"
    exit 1
  fi

  # Resolve to absolute path
  if [[ ! -d "$target_path" ]]; then
    log "${RED}[Error]${NC} Directory not found: $target_path"
    exit 1
  fi
  target_path=$(cd "$target_path" && pwd)

  WORK_DIR="$target_path"
  init_state_file
  resolve_grange

  # Check for existing state
  if [[ -f "$STATE_FILE" ]]; then
    load_state
    if [[ $CURRENT_STAGE -ge 0 ]]; then
      log "${YELLOW}[Warning]${NC} Existing pipeline state found (completed through Stage ${STAGE_IDS[$CURRENT_STAGE]})"
      printf "  Continue from Stage ${STAGE_IDS[$((CURRENT_STAGE + 1))]}? [y/N] "
      read -r yn
      if [[ "$yn" =~ ^[Yy] ]]; then
        main_resume "$WORK_DIR"
        return
      fi
      printf "  Start fresh? This will reset all progress. [y/N] "
      read -r yn
      if [[ ! "$yn" =~ ^[Yy] ]]; then
        log "${YELLOW}[IKE]${NC} Cancelled"
        exit 0
      fi
    fi
  fi

  # Initialize fresh state
  TARGET_PATH="$target_path"
  CURRENT_STAGE=-1
  TARGET_STACK=""
  BUILD_DIR=""
  STARTED_AT=$(date -Iseconds 2>/dev/null || date +%Y-%m-%dT%H:%M:%S)

  write_state

  trap handle_interrupt SIGINT SIGTERM

  log "${GREEN}══════════════════════════════════════════════════════${NC}"
  log "${GREEN}  IKE v3 — Institutional Knowledge Extractor${NC}"
  log "${GREEN}══════════════════════════════════════════════════════${NC}"
  log "${DIM}Target: $TARGET_PATH${NC}"
  log "${DIM}Grange: $GRANGE_DIR${NC}"
  echo ""

  # Run stages sequentially
  for i in "${!STAGE_IDS[@]}"; do
    run_stage "$i"
  done

  echo ""
  log "${GREEN}══════════════════════════════════════════════════════${NC}"
  log "${GREEN}  IKE v3 Pipeline Complete!${NC}"
  log "${GREEN}══════════════════════════════════════════════════════${NC}"
  echo ""
}

main_resume() {
  local dir="${1:-.}"

  # Resolve to absolute path
  if [[ ! -d "$dir" ]]; then
    log "${RED}[Error]${NC} Directory not found: $dir"
    exit 1
  fi
  dir=$(cd "$dir" && pwd)

  WORK_DIR="$dir"
  init_state_file

  if [[ ! -f "$STATE_FILE" ]]; then
    log "${RED}[Error]${NC} No .ike-state found in $dir"
    log "  Start a new pipeline with: $0 start $dir"
    exit 1
  fi

  load_state
  resolve_grange

  trap handle_interrupt SIGINT SIGTERM

  local next_stage=$((CURRENT_STAGE + 1))

  if [[ $next_stage -ge $NUM_STAGES ]]; then
    log "${GREEN}[IKE]${NC} Pipeline already complete!"
    show_status "$dir"
    exit 0
  fi

  log "${BLUE}[IKE]${NC} Resuming from Stage ${STAGE_IDS[$next_stage]} — ${STAGE_NAMES[$next_stage]}"
  echo ""

  for (( i=next_stage; i<NUM_STAGES; i++ )); do
    run_stage "$i"
  done

  echo ""
  log "${GREEN}══════════════════════════════════════════════════════${NC}"
  log "${GREEN}  IKE v3 Pipeline Complete!${NC}"
  log "${GREEN}══════════════════════════════════════════════════════${NC}"
  echo ""
}

# ============================================
# CLI
# ============================================

case "${1:-}" in
  start)
    shift
    main_start "$@"
    ;;
  resume)
    shift
    main_resume "${1:-.}"
    ;;
  status)
    shift
    show_status "${1:-.}"
    ;;
  reset)
    shift
    if [[ $# -lt 1 ]]; then
      log "${RED}[Error]${NC} Missing stage index"
      echo "  Usage: $0 reset <stage-index> [work-dir]"
      exit 1
    fi
    reset_to "$1" "${2:-.}"
    ;;
  --help|-h)
    usage
    ;;
  *)
    usage
    ;;
esac
