#!/bin/bash
# evolve.sh - Event-driven multi-agent system
# Agents react to file changes instead of polling on timers

set -euo pipefail

# Config
WORK_DIR="${WORK_DIR:-.}"
LOG_FILE="$WORK_DIR/LOG.md"
LOCK_DIR="$WORK_DIR/.locks"
SIGNAL_FILE="$WORK_DIR/.locks/signal_count"
GIT_SIGNAL="$WORK_DIR/.git-commit-signal"
MIN_INTERVAL=30  # Minimum seconds between runs of same agent

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() {
  echo -e "$(date +%H:%M:%S) $1" | tee -a "$LOG_FILE"
}

# Timeout wrapper with fallback for systems without timeout command
run_with_timeout() {
  local seconds=$1
  shift
  
  if command -v timeout &> /dev/null; then
    timeout "$seconds" "$@"
  else
    # Fallback using background process + sleep + kill
    "$@" &
    local pid=$!
    local killed_by_watchdog=false
    
    (
      sleep "$seconds"
      if kill -0 "$pid" 2>/dev/null; then
        kill -TERM "$pid" 2>/dev/null
      fi
    ) &
    local watchdog=$!
    
    # Wait for main process and capture its exit status
    local exit_status=0
    wait "$pid" 2>/dev/null || exit_status=$?
    
    # Clean up watchdog
    if kill -0 "$watchdog" 2>/dev/null; then
      # Watchdog still running = process exited naturally before timeout
      kill "$watchdog" 2>/dev/null
      wait "$watchdog" 2>/dev/null || true
      return $exit_status
    else
      # Watchdog finished = it triggered the timeout
      wait "$watchdog" 2>/dev/null || true
      # 124 is timeout's conventional exit code for timeout
      return 124
    fi
  fi
}

# Ensure required files exist
init_workspace() {
  mkdir -p "$LOCK_DIR"
  touch "$WORK_DIR/VISION.md" "$WORK_DIR/PLAN.md" "$LOG_FILE"
  [[ -f "$WORK_DIR/BLOCKERS.md" ]] || touch "$WORK_DIR/BLOCKERS.md"
  [[ -f "$WORK_DIR/CUTS.md" ]] || touch "$WORK_DIR/CUTS.md"
  [[ -f "$WORK_DIR/DRIFT.md" ]] || touch "$WORK_DIR/DRIFT.md"
  [[ -f "$SIGNAL_FILE" ]] || echo "0" > "$SIGNAL_FILE"
  
  # Setup git hook if in a git repo
  setup_git_hook
}

# Setup git post-commit hook for event-driven commit detection
setup_git_hook() {
  local git_dir
  git_dir=$(git rev-parse --git-dir 2>/dev/null) || return 0
  
  local hooks_dir="$git_dir/hooks"
  local hook_file="$hooks_dir/post-commit"
  local marker="# EVOLVE_HOOK"
  
  mkdir -p "$hooks_dir"
  
  # Check if our hook is already installed
  if [[ -f "$hook_file" ]] && grep -q "$marker" "$hook_file"; then
    return 0
  fi
  
  # Append our hook (preserve existing hooks)
  cat >> "$hook_file" << EOF

$marker
# Signal evolve.sh about new commits
touch "${GIT_SIGNAL}" 2>/dev/null || true
EOF
  
  chmod +x "$hook_file"
  log "${GREEN}[Setup]${NC} Installed git post-commit hook"
}

# Check if DONE.md exists (vision achieved)
check_done() {
  [[ -f "$WORK_DIR/DONE.md" ]] && return 0 || return 1
}

# Atomic signal counter operations (fixes subshell scope issue)
get_signal_count() {
  cat "$SIGNAL_FILE" 2>/dev/null || echo "0"
}

add_signal_count() {
  local amount=${1:-1}
  (
    flock -x 201
    local current=$(cat "$SIGNAL_FILE" 2>/dev/null || echo "0")
    echo $((current + amount)) > "$SIGNAL_FILE"
  ) 201>"$SIGNAL_FILE.lock"
}

reset_signal_count() {
  (
    flock -x 201
    echo "0" > "$SIGNAL_FILE"
  ) 201>"$SIGNAL_FILE.lock"
}

# Rate-limited agent runner
# Prevents same agent from running more than once per MIN_INTERVAL
run_agent() {
  local name=$1
  local prompt=$2
  local lock_file="$LOCK_DIR/${name}.lock"
  local time_file="$LOCK_DIR/${name}.last"
  local run_marker="$LOCK_DIR/running_${name}"
  
  # Atomic check for DONE + acquire run marker (fixes race condition)
  if check_done || ! mkdir "$run_marker" 2>/dev/null; then
    if check_done; then
      log "${GREEN}[$name]${NC} Vision achieved. Skipping."
    else
      log "${YELLOW}[$name]${NC} Already starting, skipping"
    fi
    return 0
  fi
  
  # Cleanup run marker on exit
  trap "rmdir '$run_marker' 2>/dev/null || true" RETURN
  
  # Check rate limit
  if [[ -f "$time_file" ]]; then
    local last_run=$(cat "$time_file")
    local now=$(date +%s)
    local elapsed=$((now - last_run))
    if (( elapsed < MIN_INTERVAL )); then
      log "${YELLOW}[$name]${NC} Rate limited (${elapsed}s < ${MIN_INTERVAL}s)"
      return 0
    fi
  fi
  
  # Acquire lock (non-blocking)
  exec 200>"$lock_file"
  if ! flock -n 200; then
    log "${YELLOW}[$name]${NC} Already running, skipping"
    return 0
  fi
  
  log "${BLUE}[$name]${NC} Running..."
  date +%s > "$time_file"
  
  if run_with_timeout 300 opencode run "$prompt" 2>&1 | tee -a "$LOG_FILE"; then
    log "${GREEN}[$name]${NC} Completed"
  else
    local exit_code=$?
    if [[ $exit_code -eq 124 ]]; then
      log "${RED}[$name]${NC} TIMEOUT after 300s"
    else
      log "${RED}[$name]${NC} Failed with exit code $exit_code"
    fi
  fi
  
  flock -u 200
}

# ============================================
# AGENT DEFINITIONS
# ============================================

executor() {
  run_agent "Executor" "You are the Executor agent. Do the most important incomplete task (- [ ]) in PLAN.md that serves VISION.md. After completing, mark it done (- [x]) and commit with a descriptive message. If blocked, document in BLOCKERS.md. Focus on one task only."
}

planner() {
  run_agent "Planner" "You are the Planner agent. Review VISION.md and PLAN.md. Add ONE concrete next task that moves toward the vision. Tasks should be atomic and actionable. No duplicates. Format: '- [ ] <task description>'. Add to the most logical position in PLAN.md."
}

critic() {
  run_agent "Critic" "You are the Critic agent. Review PLAN.md against VISION.md. If any task doesn't serve the vision or is redundant, remove it and log what you cut to CUTS.md with reasoning. If everything aligns well, do nothing. Be conservative - only cut what clearly doesn't fit."
}

gap_finder() {
  run_agent "Gap" "You are the Gap Finder agent. Pick a random completed task (- [x]) from PLAN.md. Review its implementation against VISION.md. If the implementation drifted from the vision's intent, add a corrective task to PLAN.md and log the drift to DRIFT.md. If implementation is solid, do nothing."
}

oracle() {
  run_agent "Oracle" "You are the Oracle agent. Review VISION.md, PLAN.md, and the codebase holistically. If the vision is fully achieved with no remaining work needed, create DONE.md with a summary of what was accomplished. Be certain before declaring done - check thoroughly. If not complete, do nothing."
}

visionary() {
  # Build context about previous observations to avoid repetition
  local history_context=""
  if [[ -f "$WORK_DIR/VISION_REVIEW.md" ]]; then
    local previous_observations
    previous_observations=$(grep -E "^## Observation" -A 3 "$WORK_DIR/VISION_REVIEW.md" 2>/dev/null | tail -20 || true)
    if [[ -n "$previous_observations" ]]; then
      history_context="

IMPORTANT: You have previously made these observations (avoid repeating them unless the pattern has significantly worsened):
$previous_observations
"
    fi
  fi

  run_agent "Visionary" "You are the Visionary agent. Review VISION.md, PLAN.md, BLOCKERS.md, CUTS.md, and DRIFT.md for patterns.

Look for signals that the vision needs refinement:
- Recurring blockers suggesting the vision is unrealistic
- Many cuts suggesting scope creep or misalignment  
- Drift patterns suggesting the vision is ambiguous
- Completed tasks that don't feel like progress
${history_context}
If you detect a NEW pattern worth addressing, APPEND to VISION_REVIEW.md (don't overwrite). Structure each entry as:

---
**Date:** $(date +%Y-%m-%d)

## Observation
<what pattern you noticed>

## Question
<specific question for the human to consider>

## Suggested Refinement (optional)
<concrete suggestion if you have one>

---

NEVER modify VISION.md directly - only append to VISION_REVIEW.md.
If everything looks healthy and coherent, or you've already noted the same patterns, do nothing."
}

# ============================================
# EVENT WATCHERS
# ============================================

# Watch for file changes and trigger appropriate agents
watch_plan() {
  log "${GREEN}[Watcher]${NC} Monitoring PLAN.md for changes..."
  while read -r dir event file; do
    if check_done; then
      log "${GREEN}[Watcher]${NC} DONE.md exists, stopping plan watcher"
      break
    fi
    log "${BLUE}[Watcher]${NC} PLAN.md changed, triggering agents..."
    executor &
    sleep 2
    critic &
  done < <(inotifywait -m -e close_write,moved_to "$WORK_DIR/PLAN.md" 2>/dev/null)
}

watch_vision() {
  log "${GREEN}[Watcher]${NC} Monitoring VISION.md for changes..."
  while read -r dir event file; do
    if check_done; then
      log "${GREEN}[Watcher]${NC} DONE.md exists, stopping vision watcher"
      break
    fi
    log "${BLUE}[Watcher]${NC} VISION.md changed, re-evaluating plan..."
    critic &
    sleep 5
    planner &
  done < <(inotifywait -m -e close_write,moved_to "$WORK_DIR/VISION.md" 2>/dev/null)
}

# Watch for signs the vision needs review
watch_signals() {
  log "${GREEN}[Watcher]${NC} Monitoring BLOCKERS.md, CUTS.md, DRIFT.md for patterns..."
  
  # Track file sizes to detect growth
  local last_blockers=$(wc -l < "$WORK_DIR/BLOCKERS.md" 2>/dev/null || echo 0)
  local last_cuts=$(wc -l < "$WORK_DIR/CUTS.md" 2>/dev/null || echo 0)
  local last_drift=$(wc -l < "$WORK_DIR/DRIFT.md" 2>/dev/null || echo 0)
  
  # Process substitution keeps loop in main shell, preserving variable state
  while read -r dir event file; do
    if check_done; then
      log "${GREEN}[Watcher]${NC} DONE.md exists, stopping signal watcher"
      break
    fi
    
    # Count how much the files grew
    local curr_blockers=$(wc -l < "$WORK_DIR/BLOCKERS.md" 2>/dev/null || echo 0)
    local curr_cuts=$(wc -l < "$WORK_DIR/CUTS.md" 2>/dev/null || echo 0)
    local curr_drift=$(wc -l < "$WORK_DIR/DRIFT.md" 2>/dev/null || echo 0)
    
    local growth=$(( (curr_blockers - last_blockers) + (curr_cuts - last_cuts) + (curr_drift - last_drift) ))
    
    if (( growth > 0 )); then
      # Use file-based counter to persist across subshell iterations
      add_signal_count "$growth"
      local signal_count
      signal_count=$(get_signal_count)
      log "${YELLOW}[Watcher]${NC} Signal files grew by $growth lines (total signals: $signal_count)"
      
      # Trigger Visionary after accumulating enough signals
      if (( signal_count >= 3 )); then
        log "${BLUE}[Watcher]${NC} Threshold reached, triggering Visionary..."
        reset_signal_count
        visionary &
      fi
    fi
    
    last_blockers=$curr_blockers
    last_cuts=$curr_cuts
    last_drift=$curr_drift
  done < <(inotifywait -m -e close_write,moved_to "$WORK_DIR/BLOCKERS.md" "$WORK_DIR/CUTS.md" "$WORK_DIR/DRIFT.md" 2>/dev/null)
}

# Git commit watcher - event-driven via hook signal file
watch_commits() {
  log "${GREEN}[Watcher]${NC} Monitoring git commits via hook signal..."
  
  # Create signal file if it doesn't exist
  touch "$GIT_SIGNAL"
  
  while read -r dir event file; do
    # Only react to our signal file
    [[ "$file" == "$(basename "$GIT_SIGNAL")" ]] || continue
    
    if check_done; then
      log "${GREEN}[Watcher]${NC} DONE.md exists, stopping commit watcher"
      break
    fi
    
    local current_commit
    current_commit=$(git rev-parse HEAD 2>/dev/null || echo "none")
    log "${BLUE}[Watcher]${NC} New commit detected: ${current_commit:0:8}"
    
    # After a commit, evaluate and plan next steps
    gap_finder &
    sleep 5
    planner &
    sleep 10
    oracle &
  done < <(inotifywait -m -e close_write,moved_to,create "$(dirname "$GIT_SIGNAL")" 2>/dev/null)
}

# Periodic heartbeat for agents that need regular checks
heartbeat() {
  local interval=${1:-300}  # Default 5 minutes
  
  while true; do
    if check_done; then
      log "${GREEN}[Heartbeat]${NC} DONE.md exists, stopping heartbeat"
      break
    fi
    
    sleep "$interval"
    log "${BLUE}[Heartbeat]${NC} Periodic check..."
    oracle &
    sleep 30
    planner &  # Ensure forward progress
  done
}

# ============================================
# MAIN
# ============================================

main() {
  init_workspace
  
  log "${GREEN}========================================${NC}"
  log "${GREEN}  Evolve.sh - Event-Driven Agent System${NC}"
  log "${GREEN}========================================${NC}"
  
  # Check dependencies
  if ! command -v inotifywait &> /dev/null; then
    log "${RED}[Error]${NC} inotifywait not found. Install with: apt install inotify-tools"
    exit 1
  fi
  
  if ! command -v opencode &> /dev/null; then
    log "${RED}[Error]${NC} opencode not found"
    exit 1
  fi
  
  if check_done; then
    log "${GREEN}[Main]${NC} DONE.md already exists. Vision was achieved!"
    cat "$WORK_DIR/DONE.md"
    exit 0
  fi
  
  # Trap for cleanup
  trap 'log "${YELLOW}[Main]${NC} Shutting down..."; kill $(jobs -p) 2>/dev/null; exit 0' SIGINT SIGTERM
  
  log "${BLUE}[Main]${NC} Starting watchers..."
  
  # Start event watchers
  watch_plan &
  watch_vision &
  watch_commits &
  watch_signals &
  heartbeat 300 &
  
  # Initial kick-off if PLAN.md has tasks
  if grep -q '\- \[ \]' "$WORK_DIR/PLAN.md" 2>/dev/null; then
    log "${BLUE}[Main]${NC} Found pending tasks, starting executor..."
    sleep 2
    executor &
  else
    log "${BLUE}[Main]${NC} No pending tasks, starting planner..."
    sleep 2
    planner &
  fi
  
  log "${GREEN}[Main]${NC} All watchers running. Touch DONE.md to stop."
  log "${GREEN}[Main]${NC} Press Ctrl+C to shutdown manually."
  
  # Wait for all background jobs
  wait
}

# ============================================
# CLI COMMANDS
# ============================================

case "${1:-start}" in
  start)
    main
    ;;
  executor)
    init_workspace
    executor
    ;;
  planner)
    init_workspace
    planner
    ;;
  critic)
    init_workspace
    critic
    ;;
  gap)
    init_workspace
    gap_finder
    ;;
  oracle)
    init_workspace
    oracle
    ;;
  visionary)
    init_workspace
    visionary
    ;;
  status)
    echo "=== VISION ==="
    head -20 "$WORK_DIR/VISION.md" 2>/dev/null || echo "(empty)"
    echo ""
    echo "=== PLAN (pending) ==="
    grep '\- \[ \]' "$WORK_DIR/PLAN.md" 2>/dev/null || echo "(none)"
    echo ""
    echo "=== PLAN (completed) ==="
    grep '\- \[x\]' "$WORK_DIR/PLAN.md" 2>/dev/null | tail -10 || echo "(none)"
    echo ""
    echo "=== RECENT LOG ==="
    tail -20 "$LOG_FILE" 2>/dev/null || echo "(empty)"
    ;;
  *)
    echo "Usage: $0 {start|executor|planner|critic|gap|oracle|visionary|status}"
    exit 1
    ;;
esac
