#!/bin/bash
# grow.sh - Event-driven multi-agent system
# Agents react to file changes instead of polling on timers
#
# Environment variables:
#   SMART_AGENTS     - Agents using CLAUDE_SMART_CMD (default: "Oracle,Visionary")
#   CLAUDE_SMART_CMD - Command for smart agents (default: "claude")
#   CLAUDE_CHEAP_CMD - Command for other agents (default: "claude-cheap")
#   CLAUDE_DEBUG     - Enable debug logging (default: true)
#   CLAUDE_DEBUG     - Enable debug logging (default: true)
#   AGENT_TIMEOUT    - Default timeout in seconds (default: 600)
#   ORACLE_TIMEOUT   - Timeout for Oracle (default: 900)
#   DEBOUNCE_INTERVAL- Min seconds between triggers (default: 60)
#   MAX_AGENTS       - Max concurrent agents (default: 2)

set -euo pipefail

# Config
WORK_DIR="${WORK_DIR:-.}"
LOG_FILE="$WORK_DIR/LOG.md"
LOCK_DIR="$WORK_DIR/.locks"
SIGNAL_FILE="$WORK_DIR/.locks/signal_count"
TIMEOUT_COUNT_FILE="$LOCK_DIR/timeout_count"
GIT_SIGNAL="$WORK_DIR/.git-commit-signal"
MIN_INTERVAL=30  # Minimum seconds between runs of same agent

# Timeout configuration (in seconds)
DEFAULT_TIMEOUT=600                              # 10 minutes default
AGENT_TIMEOUT="${AGENT_TIMEOUT:-$DEFAULT_TIMEOUT}"
ORACLE_TIMEOUT="${ORACLE_TIMEOUT:-1800}"         # 30 minutes (includes boot + integration checks)
DEBOUNCE_INTERVAL="${DEBOUNCE_INTERVAL:-60}"     # 60 seconds between watcher triggers

# Concurrency control
MAX_AGENTS="${MAX_AGENTS:-2}"                    # Maximum concurrent agents
AGENT_COUNT_FILE="$LOCK_DIR/agent_count"
MAX_PENDING_TASKS="${MAX_PENDING_TASKS:-5}"      # Skip Planner if queue is full

# Grow mode — presets for common workflows (individual vars can still override)
GROW_MODE="${GROW_MODE:-auto}"

if [[ "$GROW_MODE" == "pair" ]]; then
  ENABLE_CI="${ENABLE_CI:-auto}"
  ENABLE_APPROVAL_GATE="${ENABLE_APPROVAL_GATE:-true}"
  ENABLE_REVIEW_GATE="${ENABLE_REVIEW_GATE:-true}"
  ENABLE_TDD="${ENABLE_TDD:-true}"
  MAX_FILE_LINES="${MAX_FILE_LINES:-300}"
  REFACTOR_INTERVAL="${REFACTOR_INTERVAL:-5}"
  _SKIP_STARTUP_VISIONARY=false   # Always validate in pair mode
elif [[ "$GROW_MODE" == "sleep" ]]; then
  ENABLE_CI="${ENABLE_CI:-auto}"
  ENABLE_APPROVAL_GATE="${ENABLE_APPROVAL_GATE:-false}"
  ENABLE_REVIEW_GATE="${ENABLE_REVIEW_GATE:-false}"
  ENABLE_TDD="${ENABLE_TDD:-true}"
  MAX_FILE_LINES="${MAX_FILE_LINES:-300}"
  REFACTOR_INTERVAL="${REFACTOR_INTERVAL:-5}"
  _SKIP_STARTUP_VISIONARY=auto    # Hash-based skip
else  # auto (default) — backwards-compatible
  ENABLE_CI="${ENABLE_CI:-auto}"
  ENABLE_APPROVAL_GATE="${ENABLE_APPROVAL_GATE:-false}"
  ENABLE_REVIEW_GATE="${ENABLE_REVIEW_GATE:-false}"
  ENABLE_TDD="${ENABLE_TDD:-true}"
  MAX_FILE_LINES="${MAX_FILE_LINES:-300}"
  REFACTOR_INTERVAL="${REFACTOR_INTERVAL:-5}"
  _SKIP_STARTUP_VISIONARY=auto
fi

# Pair mode execution style (only used when GROW_MODE=pair)
PAIR_STYLE="${PAIR_STYLE:-interactive}"  # interactive | plan
PAIR_TIMEOUT="${PAIR_TIMEOUT:-3600}"     # 1 hour default for interactive sessions

COMMIT_COUNT_FILE="$LOCK_DIR/commit_count"
APPROVAL_SIGNAL="$WORK_DIR/.plan-approved"
VISION_HASH_FILE="$LOCK_DIR/vision_validated_hash"
VISION_REVIEW_SIGNAL="$WORK_DIR/.vision-review-pending"
REVIEW_ACK_SIGNAL="$WORK_DIR/.vision-reviewed"

# Checkpointing
CHECKPOINT_DIR="$LOCK_DIR/checkpoints"

# Load environment variables from .env if present
# Check work dir first, then fall back to the script's own directory (for symlinked setups)
SCRIPT_DIR="$(cd "$(dirname "$(readlink -f "$0")")" && pwd)"
if [[ -f "$WORK_DIR/.env" ]]; then
  set -a
  source "$WORK_DIR/.env"
  set +a
elif [[ -f "$SCRIPT_DIR/.env" ]]; then
  set -a
  source "$SCRIPT_DIR/.env"
  set +a
fi

# Agent configuration
# SMART_AGENTS run CLAUDE_SMART_CMD, the rest run CLAUDE_CHEAP_CMD
source "$SCRIPT_DIR/lib/claude-cmd.sh"
SMART_AGENTS="${SMART_AGENTS:-Oracle,Visionary}"
CLAUDE_DEBUG="${CLAUDE_DEBUG:-false}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

if [[ -t 1 ]]; then
  log() { echo -e "$(date +%H:%M:%S) $1" | tee -a "$LOG_FILE"; }
  # tee to stdout + log files
  _agent_tee() { tee -a "$@"; }
else
  log() { echo -e "$(date +%H:%M:%S) $1" >> "$LOG_FILE"; }
  # log files only, no stdout
  _agent_tee() { tee -a "$@" > /dev/null; }
fi

# Timeout wrapper with graceful shutdown (SIGTERM first, then SIGKILL)
# Gives process 30 seconds to clean up after SIGTERM before SIGKILL
run_with_timeout() {
  local seconds=$1
  local grace_period=30  # Seconds to wait after SIGTERM before SIGKILL
  shift

  if command -v timeout &> /dev/null; then
    # Use timeout with --signal and --kill-after for graceful shutdown
    # SIGTERM first, then SIGKILL after grace period
    timeout --signal=TERM --kill-after="$grace_period" "$seconds" "$@"
  else
    # Fallback using background process + sleep + kill
    "$@" &
    local pid=$!

    (
      sleep "$seconds"
      if kill -0 "$pid" 2>/dev/null; then
        # Graceful shutdown: SIGTERM first
        kill -TERM "$pid" 2>/dev/null
        # Wait for grace period
        sleep "$grace_period"
        # Force kill if still running
        if kill -0 "$pid" 2>/dev/null; then
          kill -KILL "$pid" 2>/dev/null
        fi
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
  mkdir -p "$CHECKPOINT_DIR"
  mkdir -p "$WORK_DIR/specs"
  touch "$WORK_DIR/VISION.md" "$WORK_DIR/PLAN.md" "$LOG_FILE"
  [[ -f "$WORK_DIR/BLOCKERS.md" ]] || touch "$WORK_DIR/BLOCKERS.md"
  [[ -f "$WORK_DIR/CUTS.md" ]] || touch "$WORK_DIR/CUTS.md"
  [[ -f "$WORK_DIR/DRIFT.md" ]] || touch "$WORK_DIR/DRIFT.md"
  [[ -f "$SIGNAL_FILE" ]] || echo "0" > "$SIGNAL_FILE"
  # Initialize agent count file
  [[ -f "$AGENT_COUNT_FILE" ]] || echo "0" > "$AGENT_COUNT_FILE"
  # Initialize timeout count file
  [[ -f "$TIMEOUT_COUNT_FILE" ]] || echo "0" > "$TIMEOUT_COUNT_FILE"
  # Initialize commit count file (for refactoring interval)
  [[ -f "$COMMIT_COUNT_FILE" ]] || echo "0" > "$COMMIT_COUNT_FILE"

  # Clean up stale run markers from previous crashes/kills
  # Only remove markers where the owning process is no longer running
  local stale_count=0
  for marker in "$LOCK_DIR"/running_*; do
    [[ -d "$marker" ]] || continue
    local pid_file="$marker/pid"
    if [[ -f "$pid_file" ]]; then
      local pid=$(cat "$pid_file")
      if ! kill -0 "$pid" 2>/dev/null; then
        # Process is dead, marker is stale
        rm -rf "$marker"
        stale_count=$((stale_count + 1))
      fi
    else
      # No PID file = old format or corrupted, remove it
      rm -rf "$marker"
      stale_count=$((stale_count + 1))
    fi
  done

  # Clean up stale lock directories (*.lock.d) from previous crashes
  for lockdir in "$LOCK_DIR"/*.lock.d; do
    [[ -d "$lockdir" ]] || continue
    # Lock dirs are stale if no corresponding running_* marker exists
    local agent_name=$(basename "$lockdir" .lock.d)
    if [[ ! -d "$LOCK_DIR/running_${agent_name}" ]]; then
      rm -rf "$lockdir"
      stale_count=$((stale_count + 1))
    fi
  done

  if (( stale_count > 0 )); then
    log "${YELLOW}[Setup]${NC} Cleaned $stale_count stale run marker(s)/lock(s)"
    # Recalculate agent count based on remaining valid markers
    local active_count
    active_count=$(find "$LOCK_DIR" -maxdepth 1 -type d -name "running_*" 2>/dev/null | wc -l)
    echo "$active_count" > "$AGENT_COUNT_FILE"
  fi

  # Setup git hook if in a git repo
  setup_git_hook
}

# Setup git post-commit hook for event-driven commit detection
setup_git_hook() {
  local git_dir
  git_dir=$(git rev-parse --git-dir 2>/dev/null) || return 0
  
  local hooks_dir="$git_dir/hooks"
  local hook_file="$hooks_dir/post-commit"
  local marker="# GROW_HOOK"
  
  mkdir -p "$hooks_dir"
  
  # Check if our hook is already installed
  if [[ -f "$hook_file" ]] && grep -q "$marker" "$hook_file"; then
    return 0
  fi
  
  # Append our hook (preserve existing hooks)
  cat >> "$hook_file" << 'HOOKEOF'

# GROW_HOOK
# Signal grow.sh about new commits
touch ".git-commit-signal" 2>/dev/null || true

# Run CI in background if available and not disabled
if [[ "${ENABLE_CI:-auto}" != "false" ]] && [[ -x "./ci.sh" ]]; then
  (
    echo "=== Post-commit CI at $(date) ===" >> .locks/ci-hook.log
    ./ci.sh >> .locks/ci-hook.log 2>&1 && echo "PASS ($(date))" >> .locks/ci-hook.log || echo "FAIL ($(date))" >> .locks/ci-hook.log
  ) &
fi
HOOKEOF
  
  chmod +x "$hook_file"
  log "${GREEN}[Setup]${NC} Installed git post-commit hook"
}

# Check if DONE.md exists (vision achieved)
check_done() {
  [[ -f "$WORK_DIR/DONE.md" ]] && return 0 || return 1
}

# Check if all tasks in PLAN.md are complete (no remaining [ ] tasks)
all_tasks_complete() {
  # Returns 0 (true) if no incomplete tasks exist
  ! grep -q '\- \[ \]' "$WORK_DIR/PLAN.md" 2>/dev/null
}

# Atomic signal counter operations (fixes subshell scope issue)
# Uses mkdir-based locking for macOS compatibility (flock not available)
get_signal_count() {
  cat "$SIGNAL_FILE" 2>/dev/null || echo "0"
}

_atomic_lock() {
  local lockdir="$1"
  local max_wait=10
  local waited=0
  while ! mkdir "$lockdir" 2>/dev/null; do
    sleep 0.1
    waited=$((waited + 1))
    if (( waited > max_wait * 10 )); then
      # Stale lock, force remove
      rm -rf "$lockdir"
    fi
  done
}

_atomic_unlock() {
  rmdir "$1" 2>/dev/null || true
}

add_signal_count() {
  local amount=${1:-1}
  local lockdir="$SIGNAL_FILE.lock.d"
  _atomic_lock "$lockdir"
  local current=$(cat "$SIGNAL_FILE" 2>/dev/null || echo "0")
  echo $((current + amount)) > "$SIGNAL_FILE"
  _atomic_unlock "$lockdir"
}

reset_signal_count() {
  local lockdir="$SIGNAL_FILE.lock.d"
  _atomic_lock "$lockdir"
  echo "0" > "$SIGNAL_FILE"
  _atomic_unlock "$lockdir"
}

# Timeout counter operations (tracks consecutive agent timeouts)
get_timeout_count() {
  cat "$TIMEOUT_COUNT_FILE" 2>/dev/null || echo "0"
}

add_timeout_count() {
  local amount=${1:-1}
  local lockdir="$TIMEOUT_COUNT_FILE.lock.d"
  _atomic_lock "$lockdir"
  local current=$(cat "$TIMEOUT_COUNT_FILE" 2>/dev/null || echo "0")
  echo $((current + amount)) > "$TIMEOUT_COUNT_FILE"
  _atomic_unlock "$lockdir"
}

reset_timeout_count() {
  local lockdir="$TIMEOUT_COUNT_FILE.lock.d"
  _atomic_lock "$lockdir"
  echo "0" > "$TIMEOUT_COUNT_FILE"
  _atomic_unlock "$lockdir"
}

# Global concurrency control - limit total running agents
acquire_agent_slot() {
  local max_wait=300  # Wait up to 5 minutes for a slot
  local waited=0
  local lockdir="$AGENT_COUNT_FILE.lock.d"

  while (( waited < max_wait )); do
    _atomic_lock "$lockdir"
    local count=$(cat "$AGENT_COUNT_FILE" 2>/dev/null || echo 0)
    if (( count < MAX_AGENTS )); then
      echo $((count + 1)) > "$AGENT_COUNT_FILE"
      _atomic_unlock "$lockdir"
      return 0  # Got slot
    fi
    _atomic_unlock "$lockdir"

    sleep 5
    waited=$((waited + 5))
  done

  return 1  # Timed out waiting for slot
}

release_agent_slot() {
  local lockdir="$AGENT_COUNT_FILE.lock.d"
  _atomic_lock "$lockdir"
  local count=$(cat "$AGENT_COUNT_FILE" 2>/dev/null || echo 1)
  if (( count > 0 )); then
    echo $((count - 1)) > "$AGENT_COUNT_FILE"
  fi
  _atomic_unlock "$lockdir"
}

# Run CI script (ci.sh) if available/enabled
# Returns 0 on pass (or CI disabled/not applicable), 1 on failure
run_ci() {
  local ci_log="$LOCK_DIR/ci.log"

  if [[ "$ENABLE_CI" == "false" ]]; then
    return 0
  fi

  if [[ "$ENABLE_CI" == "true" ]]; then
    if [[ ! -x "$WORK_DIR/ci.sh" ]]; then
      log "${RED}[CI]${NC} ENABLE_CI=true but ci.sh not found or not executable"
      echo "FAIL: ci.sh not found ($(date))" >> "$ci_log"
      return 1
    fi
  fi

  # auto mode: only run if ci.sh exists and is executable
  if [[ ! -x "$WORK_DIR/ci.sh" ]]; then
    return 0
  fi

  log "${BLUE}[CI]${NC} Running ci.sh..."
  local ci_result=0
  {
    echo "=== CI run at $(date) ==="
    "$WORK_DIR/ci.sh" 2>&1
  } >> "$ci_log" || ci_result=$?

  if [[ $ci_result -eq 0 ]]; then
    log "${GREEN}[CI]${NC} Passed"
    echo "PASS ($(date))" >> "$ci_log"
  else
    log "${RED}[CI]${NC} Failed (exit code $ci_result)"
    echo "FAIL exit=$ci_result ($(date))" >> "$ci_log"
  fi
  return $ci_result
}

# Check approval gate — returns 0 if Executor may proceed
check_approval_gate() {
  if [[ "$ENABLE_APPROVAL_GATE" != "true" ]]; then
    return 0  # Gate disabled, always approved
  fi

  if [[ -f "$APPROVAL_SIGNAL" ]]; then
    rm -f "$APPROVAL_SIGNAL"
    log "${GREEN}[Gate]${NC} Plan approved, Executor may proceed"
    return 0
  fi

  log "${YELLOW}[Gate]${NC} Waiting for approval (touch .plan-approved to continue)"
  return 1
}

# Check if startup Visionary can be skipped (hash-based + time-based)
# Returns 0 (skip) if VISION.md unchanged AND Visionary ran within the last hour
should_skip_startup_visionary() {
  # Check if stored hash exists
  if [[ ! -f "$VISION_HASH_FILE" ]]; then
    return 1  # No stored hash, must run
  fi

  # Compute current hash
  local current_hash
  if command -v md5sum &>/dev/null; then
    current_hash=$(md5sum "$WORK_DIR/VISION.md" 2>/dev/null | cut -d' ' -f1)
  elif command -v md5 &>/dev/null; then
    current_hash=$(md5 -q "$WORK_DIR/VISION.md" 2>/dev/null)
  else
    return 1  # No hash tool available, must run
  fi

  local stored_hash
  stored_hash=$(cat "$VISION_HASH_FILE" 2>/dev/null)

  if [[ "$current_hash" != "$stored_hash" ]]; then
    return 1  # Vision changed, must run
  fi

  # Check if Visionary ran within the last hour
  local time_file="$LOCK_DIR/Visionary.last"
  if [[ ! -f "$time_file" ]]; then
    return 1  # Never ran, must run
  fi

  local last_run
  last_run=$(cat "$time_file" 2>/dev/null || echo 0)
  local now
  now=$(date +%s)
  local elapsed=$((now - last_run))

  if (( elapsed > 3600 )); then
    return 1  # Ran more than 1 hour ago, must run
  fi

  return 0  # Safe to skip
}

# Store current VISION.md hash for skip comparison
save_vision_hash() {
  local hash
  if command -v md5sum &>/dev/null; then
    hash=$(md5sum "$WORK_DIR/VISION.md" 2>/dev/null | cut -d' ' -f1)
  elif command -v md5 &>/dev/null; then
    hash=$(md5 -q "$WORK_DIR/VISION.md" 2>/dev/null)
  else
    return 0  # No hash tool, silently skip
  fi
  echo "$hash" > "$VISION_HASH_FILE"
}

# Check review gate — returns 0 if Executor may proceed
check_review_gate() {
  if [[ "$ENABLE_REVIEW_GATE" != "true" ]]; then
    return 0  # Gate disabled, always pass
  fi

  if [[ ! -f "$VISION_REVIEW_SIGNAL" ]]; then
    return 0  # No pending review, pass
  fi

  if [[ -f "$REVIEW_ACK_SIGNAL" ]]; then
    # Human acknowledged the review — consume both signals
    rm -f "$VISION_REVIEW_SIGNAL" "$REVIEW_ACK_SIGNAL"
    log "${GREEN}[ReviewGate]${NC} Vision review acknowledged, Executor may proceed"
    return 0
  fi

  log "${YELLOW}[ReviewGate]${NC} Waiting for vision review (touch .vision-reviewed to continue)"
  return 1
}

# Rate-limited agent runner
# Prevents same agent from running more than once per MIN_INTERVAL
# Args: name, prompt, [timeout_seconds]
run_agent() {
  local name=$1
  local raw_prompt="$2"
  local timeout=${3:-$AGENT_TIMEOUT}  # Optional timeout, defaults to AGENT_TIMEOUT

  # Build prompt preamble based on execution mode
  local prompt
  if [[ "$GROW_MODE" == "pair" && "$name" == "Executor" ]]; then
    prompt="You are pair programming with a human navigator. They can see your work and may interrupt to redirect, provide context, or adjust the approach. Explain your reasoning briefly before acting. Ask when genuinely uncertain.

$raw_prompt"
    timeout="$PAIR_TIMEOUT"  # Override timeout for interactive sessions
  else
    prompt="You are running non-interactively (no human in the loop). Use your tools (Read, Write, Edit, Bash, Glob, Grep) directly to accomplish tasks. Do NOT output text asking for permission — just act.

$raw_prompt"
  fi
  local lock_file="$LOCK_DIR/${name}.lock"
  local time_file="$LOCK_DIR/${name}.last"
  local run_marker="$LOCK_DIR/running_${name}"
  local agent_log="$LOCK_DIR/${name}.log"  # Per-agent log file

  # Atomic check for DONE + acquire run marker (fixes race condition)
  if check_done || ! mkdir "$run_marker" 2>/dev/null; then
    if check_done; then
      log "${GREEN}[$name]${NC} Vision achieved. Skipping."
    else
      log "${YELLOW}[$name]${NC} Already starting, skipping"
    fi
    return 2  # Distinct code for "skipped" - prevents self-chaining loop
  fi

  # Write PID to marker for stale detection
  echo $$ > "$run_marker/pid"

  # Cleanup run marker on exit (slot release added later after acquisition)
  trap "rm -rf '$run_marker' 2>/dev/null || true" RETURN

  # Check rate limit
  if [[ -f "$time_file" ]]; then
    local last_run=$(cat "$time_file")
    local now=$(date +%s)
    # Validate last_run is numeric (handles stale ISO date format)
    if ! [[ "$last_run" =~ ^[0-9]+$ ]]; then
      last_run=0
    fi
    local elapsed=$((now - last_run))
    if (( elapsed < MIN_INTERVAL )); then
      log "${YELLOW}[$name]${NC} Rate limited (${elapsed}s < ${MIN_INTERVAL}s)"
      return 2  # Distinct code for "skipped"
    fi
  fi

  # Acquire global concurrency slot
  log "${YELLOW}[$name]${NC} Waiting for agent slot..."
  if ! acquire_agent_slot; then
    log "${RED}[$name]${NC} Timed out waiting for agent slot"
    return 1
  fi

  # Acquire lock (non-blocking) using mkdir for macOS compatibility
  local lock_dir="${lock_file}.d"
  if ! mkdir "$lock_dir" 2>/dev/null; then
    log "${YELLOW}[$name]${NC} Already running, skipping"
    release_agent_slot
    return 2  # Distinct code for "skipped"
  fi

  # Cleanup lock dir on exit (update trap)
  trap "rm -rf '$run_marker' '$lock_dir' 2>/dev/null || true; release_agent_slot" RETURN

  date +%s > "$time_file"

  # Determine if this is a "smart" agent (claude) or "iterative" agent (claude-cheap)
  local is_smart_agent=false
  if [[ ",${SMART_AGENTS}," == *",${name},"* ]]; then
    is_smart_agent=true
  fi

  # Rotate agent log if too large (>1MB)
  if [[ -f "$agent_log" ]] && (( $(stat -c%s "$agent_log" 2>/dev/null || stat -f%z "$agent_log" 2>/dev/null || echo 0) > 1048576 )); then
    mv "$agent_log" "${agent_log}.old"
  fi

  local cmd_result=0
  echo "=== Run started at $(date) ===" >> "$agent_log"

  # Determine base command and sleep-mode permissions
  local tier="cheap"
  local -a claude_cmd=("${CLAUDE_CHEAP[@]}")
  if [[ "$is_smart_agent" == true ]]; then
    tier="smart"
    claude_cmd=("${CLAUDE_SMART[@]}")
  fi

  local skip_perms=""
  if [[ "$GROW_MODE" == "sleep" ]]; then
    skip_perms="--dangerously-skip-permissions"
  fi

  if [[ "$GROW_MODE" == "pair" && "$name" == "Executor" ]]; then
    # INTERACTIVE PAIR MODE — human navigates, agent pilots
    if [[ "$PAIR_STYLE" == "plan" ]]; then
      log "${BLUE}[$name]${NC} Running interactively with --permission-mode plan..."
      "${claude_cmd[@]}" --allowedTools "Read,Edit,Write,Bash,Glob,Grep" \
        --permission-mode plan "$prompt" 2>&1 | tee -a "$agent_log" || cmd_result=$?
    else
      # Default: full interactive
      log "${BLUE}[$name]${NC} Running interactively (human in the loop)..."
      "${claude_cmd[@]}" --allowedTools "Read,Edit,Write,Bash,Glob,Grep" \
        "$prompt" 2>&1 | tee -a "$agent_log" || cmd_result=$?
    fi
  else
    log "${BLUE}[$name]${NC} Running on $tier tier (timeout: ${timeout}s)..."
    (
      run_with_timeout "$timeout" "${claude_cmd[@]}" --allowedTools "Read,Edit,Write,Bash,Glob,Grep" \
        $skip_perms -p "$prompt" < /dev/null
    ) 2>&1 | _agent_tee "$LOG_FILE" "$agent_log" || cmd_result=$?
  fi

  if [[ $cmd_result -eq 0 ]]; then
    log "${GREEN}[$name]${NC} Completed"
    reset_timeout_count  # Reset on success
  elif [[ $cmd_result -eq 124 ]]; then
    log "${RED}[$name]${NC} TIMEOUT after ${timeout}s"
    add_timeout_count 1
    local timeout_count
    timeout_count=$(get_timeout_count)
    if (( timeout_count >= 3 )); then
      log "${YELLOW}[System]${NC} Multiple timeouts ($timeout_count), triggering Visionary..."
      reset_timeout_count
      visionary "TIMEOUT ALERT: Multiple agents timed out ($timeout_count consecutive). Consider if the vision is too vague or ambitious, or if tasks are too large to complete in the allotted time." &
    fi
  else
    log "${RED}[$name]${NC} Failed with exit code $cmd_result"
  fi

  # Lock cleanup handled by trap
}

# ============================================
# AGENT DEFINITIONS
# ============================================

executor() {
  # In pair mode, only run executor from foreground (interactive).
  # Background watcher triggers should not launch interactive sessions.
  if [[ "$GROW_MODE" == "pair" ]] && [[ ! -t 0 ]]; then
    return 0
  fi

  # Build checkpoint context if a checkpoint exists
  local checkpoint_file="$CHECKPOINT_DIR/executor.md"
  local checkpoint_context=""
  if [[ -f "$checkpoint_file" ]]; then
    checkpoint_context="

IMPORTANT - RESUME FROM CHECKPOINT:
A previous Executor run was interrupted. Here is its progress:
---
$(cat "$checkpoint_file")
---
Continue from where it left off. Delete the checkpoint file ($checkpoint_file) once you've resumed and made progress.
"
  fi

  # Build TDD instructions if enabled
  local tdd_context=""
  if [[ "$ENABLE_TDD" == "true" ]]; then
    tdd_context="
TDD WORKFLOW (mandatory):
1. Write a failing test FIRST that captures the acceptance criteria
2. Run the test to confirm it fails (red)
3. Implement the minimum code to make the test pass (green)
4. Run ALL tests to ensure nothing broke
5. Only commit if all tests are green
If the project has no test framework yet, set one up as your first step.
"
  fi

  # Build CI instructions if enabled
  local ci_context=""
  if [[ "$ENABLE_CI" != "false" ]] && [[ -x "$WORK_DIR/ci.sh" ]]; then
    ci_context="
CI GATE: Before committing, run ./ci.sh and verify it passes. If it fails, fix the issues before committing.
"
  fi

  run_agent "Executor" "You are the Executor agent. Do the most important incomplete task (- [ ]) in PLAN.md that serves VISION.md. After completing, mark it done (- [x]) and commit with a descriptive message. If blocked, document in BLOCKERS.md. Focus on one task only.

COMMIT DISCIPLINE:
- Each commit must be atomic and production-ready. One logical change per commit.

SIMPLICITY:
- Prefer the simplest solution that works. If a task could be solved in 40 lines, do not write 200.
- Avoid state machines, over-abstraction, and premature generalization unless the task explicitly requires them.
- When in doubt, choose the boring, straightforward approach.
${tdd_context}${ci_context}${checkpoint_context}
TIMEOUT HANDLING:
You have ~10 minutes. If you're working on a complex task and can't complete it:
1. Save your progress to $checkpoint_file with:
   - Which task you were working on
   - What you've completed so far
   - What remains to be done
   - Any relevant file paths or context
2. The next Executor run will resume from your checkpoint."
  local agent_result=$?

  # Post-Executor CI safety net (belt-and-suspenders with prompt instruction)
  if [[ $agent_result -eq 0 ]]; then
    run_ci || log "${YELLOW}[CI]${NC} Post-Executor CI failed — Gap Finder will catch this"
  fi

  # Self-chaining: only if run_agent actually ran (not skipped)
  if [[ $agent_result -eq 0 ]]; then
    sleep 3  # Brief pause to let commits/file changes settle
    if grep -q '\- \[ \]' "$WORK_DIR/PLAN.md" 2>/dev/null; then
      if [[ "$GROW_MODE" == "pair" ]]; then
        # In pair mode, show remaining tasks and prompt to continue
        local remaining
        remaining=$(grep -c '\- \[ \]' "$WORK_DIR/PLAN.md" 2>/dev/null) || remaining=0
        echo ""
        log "${BLUE}[Executor]${NC} Session complete. $remaining tasks remaining:"
        grep '\- \[ \]' "$WORK_DIR/PLAN.md" 2>/dev/null | head -5 | while IFS= read -r task; do
          log "  $task"
        done
        (( remaining > 5 )) && log "  ... and $((remaining - 5)) more"
        # Mention vision review if Visionary raised concerns
        if [[ -f "$VISION_REVIEW_SIGNAL" ]]; then
          echo ""
          log "${YELLOW}[Executor]${NC} Visionary raised concerns — review VISION_REVIEW.md"
        fi
        echo ""
        local answer
        read -r -p "  Continue to next task? [Y/n] " answer < /dev/tty || answer="n"
        case "$answer" in
          [nN]*)
            log "${BLUE}[Executor]${NC} Paused."
            ;;
          *)
            executor  # Recurse — locks released via RETURN trap in run_agent
            ;;
        esac
      elif [[ ! -d "$LOCK_DIR/running_Executor" ]] && check_approval_gate && check_review_gate; then
        log "${BLUE}[Executor]${NC} More tasks pending, re-triggering..."
        executor &
      fi
    fi
  fi
}

planner() {
  # Skip if too many pending tasks already
  local pending_count
  pending_count=$(grep -c '\- \[ \]' "$WORK_DIR/PLAN.md" 2>/dev/null) || pending_count=0
  if (( pending_count >= MAX_PENDING_TASKS )); then
    log "${YELLOW}[Planner]${NC} Skipped: $pending_count pending tasks (max: $MAX_PENDING_TASKS)"
    return 0
  fi

  local spec_context=""
  if [[ -d "$WORK_DIR/specs" ]] && [[ -n "$(ls -A "$WORK_DIR/specs/" 2>/dev/null)" ]]; then
    spec_context="

SPEC LIBRARY: There are specs available in specs/. Before adding a task:
1. Run: ls specs/ to see available collections
2. Run: ls specs/patterns/ for reusable cross-domain patterns
3. Browse project-specific specs if relevant (specs/tim/, specs/memoji/, etc.)
4. Read any spec whose name seems relevant to the vision
5. If a matching spec exists, reference it: '- [ ] Implement specs/[path].md: [brief description]'
6. If no spec matches, add the task without a spec reference
Specs contain acceptance criteria — use them to make tasks more precise."
  fi

  run_agent "Planner" "You are the Planner agent. Review VISION.md and PLAN.md.

ALIGNMENT CHECK (do this first):
Before adding anything, review existing incomplete tasks. If any no longer serve the vision or are redundant, remove them and log what you cut to CUTS.md with reasoning. Be conservative — only cut what clearly doesn't fit.

THEN: Add ONE concrete next task that moves toward the vision. Tasks should be atomic and actionable. No duplicates. Format: '- [ ] <task description>'. Add to the most logical position in PLAN.md.

TASK DIVERSITY:
Not every task should be a new feature. Consider adding:
- Test improvements for under-tested areas
- Documentation gaps that Gap Finder may have missed (project-level README, onboarding guides)
- Security hardening tasks
- Refactoring of complex or overgrown files
A healthy plan balances features, tests, docs, fixes, and infrastructure.${spec_context}"

  # Kick off Executor immediately if Planner added tasks (don't wait for watcher debounce)
  if grep -q '\- \[ \]' "$WORK_DIR/PLAN.md" 2>/dev/null && [[ ! -d "$LOCK_DIR/running_Executor" ]]; then
    if check_approval_gate && check_review_gate; then
      executor &
    fi
  fi
}

gap_finder() {
  local commit_hash="${1:-}"
  local commit_context=""

  if [[ -n "$commit_hash" ]]; then
    commit_context="
FOCUS: Review commit $commit_hash
Run: git show --stat $commit_hash
Then read the changed files to understand what was done."
  else
    commit_context="
FOCUS: Review the most recent commit.
Run: git log -1 --stat
Then read the changed files to understand what was done."
  fi

  run_agent "Gap" "You are the Gap Finder agent.

TASK: Review the latest commit across 5 dimensions.
${commit_context}

STEPS (be quick - you have 10 minutes):
1. Get the commit diff/stats (git show --stat, git diff HEAD~1)
2. Read VISION.md to understand the goal

CHECK 1 — VISION DRIFT:
- Does this commit align with the vision?
- IF DRIFT: Add to DRIFT.md with commit hash, what drifted, why it matters. Add task to PLAN.md: '- [ ] Fix drift: <specific issue>'

CHECK 2 — FILE SIZE (>${MAX_FILE_LINES} lines):
- Run: git diff --name-only HEAD~1 HEAD
- For each changed file, run: wc -l <file>
- If any file exceeds ${MAX_FILE_LINES} lines, add to DRIFT.md: 'File size: <file> is <N> lines (threshold: ${MAX_FILE_LINES})'
- Add task to PLAN.md: '- [ ] Refactor: split <file> (<N> lines, max ${MAX_FILE_LINES})'

CHECK 3 — TEST EXISTENCE:
- For each NEW source file in the commit (not test files themselves), check if a corresponding test file exists
- Common patterns: src/foo.ts → tests/foo.test.ts, src/foo.py → tests/test_foo.py, pkg/foo.go → pkg/foo_test.go
- If no test file exists, add task to PLAN.md: '- [ ] Add tests for <file>'

CHECK 4 — SECURITY REVIEW:
- Review the diff for obvious security issues:
  - Hardcoded secrets, API keys, passwords, tokens
  - SQL injection (string concatenation in queries)
  - Path traversal (unsanitized user input in file paths)
  - Missing input validation at system boundaries
- If found, add to DRIFT.md: 'Security: <issue> in <file>' with severity (high/medium/low)
- Add task to PLAN.md: '- [ ] Security fix: <specific issue in file>'

CHECK 5 — DOCUMENTATION:
- Does this commit introduce new patterns, workarounds, or architectural decisions?
- Does it add a new dependency, integration, or non-obvious configuration?
- Does it establish a convention that future code should follow?
- If YES to any: generate a structured .md documentation file.
  PLACEMENT — choose the most contextually useful location:
  - For module/directory-specific patterns: create or append to a README.md in that directory
  - For cross-cutting architectural decisions: append to ARCHITECTURE.md at project root (create if needed)
  - For conventions that AI agents need to follow: append to CLAUDE.md
  FORMAT — each entry should include:
  - A descriptive heading (## Decision/Pattern/Convention: ...)
  - Date: $(date +%Y-%m-%d)
  - What: clear explanation of the pattern/decision
  - Why: reasoning or context behind it
  - Example: a brief code snippet or reference if helpful
  After writing, commit the documentation with message: 'docs: [brief description]'
- Only document genuinely useful knowledge — not trivial implementation details.

IF ALL CHECKS PASS: Do nothing. Don't log success."
}

oracle() {
  run_agent "Oracle" "You are the Oracle agent.

TASK: Determine if the vision is FULLY achieved.

CHECKLIST (all must be true to declare done):
1. Read VISION.md - understand the goal
2. Read PLAN.md - verify ALL tasks are marked [x] complete (no remaining [ ] tasks)
2b. VERIFY OUTPUT ARTIFACTS: Do NOT trust PLAN.md checkboxes alone. For each 'Done When'
   criterion in VISION.md that references files or directories, independently verify they
   exist and are non-empty using Bash (ls) or Glob. If VISION.md says a directory should
   contain files, verify it does. If artifacts are missing despite tasks being marked
   complete, uncheck those tasks in PLAN.md and add fix tasks.
3. Check BLOCKERS.md - must be empty or all issues resolved
4. If the project has a build command, run it - must pass with no errors
5. If the project has tests, run them - must pass
6. Read VISION_REVIEW.md - note any observations from the Visionary that were never addressed or resolved
7. Boot and verify the running project:
   - Look at the project structure (docker-compose.yml, package.json, Makefile, etc.)
     to figure out how to start it
   - Boot the project, seed any demo/test data if applicable
   - Verify key endpoints or pages respond (curl health checks, etc.)
   - If anything crashes or errors: add fix tasks, tear down, and stop
   - Always clean up (stop containers, kill dev servers) after checking
   - If .locks/verify_cycles exists and its count >= 3, skip this step
     and note remaining integration issues in DONE.md instead of looping

DECISION:
- If ALL checks pass: use the Write tool to create DONE.md containing:
  - Summary of what was achieved (2-3 sentences)
  - List of completed tasks from PLAN.md
  - Build/test status confirmation
  - Unaddressed Vision Reviews: list any VISION_REVIEW.md entries whose concerns
    were not resolved. For each, include the original Observation and a brief note
    on the potential risk or gap it may introduce. If all entries were addressed
    or VISION_REVIEW.md doesn't exist, note that no unresolved observations remain.
- If ANY check fails: use Edit to add a task to PLAN.md describing what needs to be fixed.
  Format: '- [ ] Fix: <specific issue found>'
  Be specific (e.g., '- [ ] Fix: test_auth failing - expected 200, got 401')
  After running integration checks (step 7), increment the number in .locks/verify_cycles
  (create the file with '1' if it doesn't exist). This prevents infinite fix loops.

Take your time. You have 30 minutes." "$ORACLE_TIMEOUT"
  local agent_result=$?

  # Fallback: if Oracle exited successfully and all tasks are complete but DONE.md
  # was not created (e.g. agent asked for permission instead of using Write tool),
  # generate a fallback DONE.md so grow.sh can terminate.
  if [[ $agent_result -eq 0 ]] && all_tasks_complete && ! check_done; then
    log "${YELLOW}[Oracle]${NC} Agent exited 0 with all tasks complete but no DONE.md — creating fallback"
    {
      echo "# DONE (fallback — auto-generated)"
      echo ""
      echo "**Note:** The Oracle agent completed without creating DONE.md."
      echo "This fallback was generated because all PLAN.md tasks are marked complete."
      echo "**Human review recommended** to verify vision fulfillment."
      echo ""
      echo "## Completed Tasks"
      grep '\- \[x\]' "$WORK_DIR/PLAN.md" 2>/dev/null || echo "(none found)"
      echo ""
      echo "## Unaddressed Vision Reviews"
      if [[ -f "$WORK_DIR/VISION_REVIEW.md" ]] && [[ -s "$WORK_DIR/VISION_REVIEW.md" ]]; then
        echo "VISION_REVIEW.md exists — please review manually for unresolved observations."
      else
        echo "No unresolved observations."
      fi
    } > "$WORK_DIR/DONE.md"
    log "${GREEN}[Oracle]${NC} Fallback DONE.md created"
  fi
}

visionary() {
  local trigger_context="${1:-}"  # Optional context about why Visionary was triggered

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

  # Build trigger context section if provided
  local trigger_section=""
  if [[ -n "$trigger_context" ]]; then
    trigger_section="

TRIGGER CONTEXT:
$trigger_context

"
  fi

  run_agent "Visionary" "You are the Visionary agent. Review VISION.md, PLAN.md, BLOCKERS.md, CUTS.md, and DRIFT.md for patterns.
${trigger_section}
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

# Platform detection for file watching
USE_FSWATCH=false
if [[ "$(uname)" == "Darwin" ]]; then
  USE_FSWATCH=true
fi

# Watch for file changes and trigger appropriate agents
watch_plan() {
  log "${GREEN}[Watcher]${NC} Monitoring PLAN.md for changes..."
  local last_trigger_file="$LOCK_DIR/watcher_plan.last"
  echo "0" > "$last_trigger_file"

  _handle_plan_change() {
    if check_done; then
      log "${GREEN}[Watcher]${NC} DONE.md exists, stopping plan watcher"
      return 1
    fi

    # Debounce: skip if triggered too recently
    local now=$(date +%s)
    local last_trigger=$(cat "$last_trigger_file" 2>/dev/null || echo 0)
    local elapsed=$((now - last_trigger))
    if (( elapsed < DEBOUNCE_INTERVAL )); then
      log "${YELLOW}[Watcher]${NC} PLAN.md change debounced (${elapsed}s < ${DEBOUNCE_INTERVAL}s)"
      return 0
    fi
    echo "$now" > "$last_trigger_file"

    log "${BLUE}[Watcher]${NC} PLAN.md changed, triggering Executor..."
    if check_approval_gate && check_review_gate; then
      executor &
    fi
  }

  if [[ "$USE_FSWATCH" == true ]]; then
    fswatch -o "$WORK_DIR/PLAN.md" 2>/dev/null | while read -r _; do
      _handle_plan_change || break
    done
  else
    while read -r dir event file; do
      _handle_plan_change || break
    done < <(inotifywait -m -e close_write,moved_to "$WORK_DIR/PLAN.md" 2>/dev/null)
  fi
}

watch_vision() {
  log "${GREEN}[Watcher]${NC} Monitoring VISION.md for changes..."
  local last_trigger_file="$LOCK_DIR/watcher_vision.last"
  echo "0" > "$last_trigger_file"

  _handle_vision_change() {
    if check_done; then
      log "${GREEN}[Watcher]${NC} DONE.md exists, stopping vision watcher"
      return 1
    fi

    # Debounce: skip if triggered too recently
    local now=$(date +%s)
    local last_trigger=$(cat "$last_trigger_file" 2>/dev/null || echo 0)
    local elapsed=$((now - last_trigger))
    if (( elapsed < DEBOUNCE_INTERVAL )); then
      log "${YELLOW}[Watcher]${NC} VISION.md change debounced (${elapsed}s < ${DEBOUNCE_INTERVAL}s)"
      return 0
    fi
    echo "$now" > "$last_trigger_file"

    # Invalidate vision hash so next startup re-validates
    rm -f "$VISION_HASH_FILE"

    log "${BLUE}[Watcher]${NC} VISION.md changed, re-evaluating plan..."
    planner &
  }

  if [[ "$USE_FSWATCH" == true ]]; then
    fswatch -o "$WORK_DIR/VISION.md" 2>/dev/null | while read -r _; do
      _handle_vision_change || break
    done
  else
    while read -r dir event file; do
      _handle_vision_change || break
    done < <(inotifywait -m -e close_write,moved_to "$WORK_DIR/VISION.md" 2>/dev/null)
  fi
}

# Watch for .plan-approved signal (only active when ENABLE_APPROVAL_GATE=true)
watch_approval() {
  if [[ "$ENABLE_APPROVAL_GATE" != "true" ]]; then
    return 0  # Gate disabled, don't watch
  fi

  log "${GREEN}[Watcher]${NC} Monitoring .plan-approved for approval signals..."

  _handle_approval() {
    if check_done; then
      log "${GREEN}[Watcher]${NC} DONE.md exists, stopping approval watcher"
      return 1
    fi

    if [[ -f "$APPROVAL_SIGNAL" ]]; then
      rm -f "$APPROVAL_SIGNAL"
      log "${GREEN}[Gate]${NC} Plan approved! Triggering Executor..."
      if grep -q '\- \[ \]' "$WORK_DIR/PLAN.md" 2>/dev/null && [[ ! -d "$LOCK_DIR/running_Executor" ]]; then
        executor &
      fi
    fi
  }

  if [[ "$USE_FSWATCH" == true ]]; then
    # Watch the directory for the signal file
    fswatch -o "$WORK_DIR" --include '.plan-approved' --exclude '.*' 2>/dev/null | while read -r _; do
      _handle_approval || break
    done
  else
    while read -r dir event file; do
      [[ "$file" == ".plan-approved" ]] || continue
      _handle_approval || break
    done < <(inotifywait -m -e create,moved_to "$WORK_DIR" 2>/dev/null)
  fi
}

# Watch for .vision-reviewed signal (only active when ENABLE_REVIEW_GATE=true)
watch_review() {
  if [[ "$ENABLE_REVIEW_GATE" != "true" ]]; then
    return 0  # Gate disabled, don't watch
  fi

  log "${GREEN}[Watcher]${NC} Monitoring .vision-reviewed for review acknowledgements..."

  _handle_review_ack() {
    if check_done; then
      log "${GREEN}[Watcher]${NC} DONE.md exists, stopping review watcher"
      return 1
    fi

    if [[ -f "$REVIEW_ACK_SIGNAL" ]]; then
      rm -f "$VISION_REVIEW_SIGNAL" "$REVIEW_ACK_SIGNAL"
      log "${GREEN}[ReviewGate]${NC} Vision review acknowledged! Triggering Executor..."
      if grep -q '\- \[ \]' "$WORK_DIR/PLAN.md" 2>/dev/null && [[ ! -d "$LOCK_DIR/running_Executor" ]]; then
        if check_approval_gate; then
          executor &
        fi
      fi
    fi
  }

  if [[ "$USE_FSWATCH" == true ]]; then
    fswatch -o "$WORK_DIR" --include '.vision-reviewed' --exclude '.*' 2>/dev/null | while read -r _; do
      _handle_review_ack || break
    done
  else
    while read -r dir event file; do
      [[ "$file" == ".vision-reviewed" ]] || continue
      _handle_review_ack || break
    done < <(inotifywait -m -e create,moved_to "$WORK_DIR" 2>/dev/null)
  fi
}

# Watch for signs the vision needs review
watch_signals() {
  log "${GREEN}[Watcher]${NC} Monitoring BLOCKERS.md, CUTS.md, DRIFT.md for patterns..."

  # Track file sizes to detect growth (file-based for subshell persistence)
  local sizes_file="$LOCK_DIR/watcher_signals.sizes"
  {
    echo "blockers:$(wc -l < "$WORK_DIR/BLOCKERS.md" 2>/dev/null || echo 0)"
    echo "cuts:$(wc -l < "$WORK_DIR/CUTS.md" 2>/dev/null || echo 0)"
    echo "drift:$(wc -l < "$WORK_DIR/DRIFT.md" 2>/dev/null || echo 0)"
    echo "review:$(wc -l < "$WORK_DIR/VISION_REVIEW.md" 2>/dev/null || echo 0)"
  } > "$sizes_file"

  _handle_signal_change() {
    if check_done; then
      log "${GREEN}[Watcher]${NC} DONE.md exists, stopping signal watcher"
      return 1
    fi

    # Read last sizes
    local last_blockers=$(grep "^blockers:" "$sizes_file" 2>/dev/null | cut -d: -f2 || echo 0)
    local last_cuts=$(grep "^cuts:" "$sizes_file" 2>/dev/null | cut -d: -f2 || echo 0)
    local last_drift=$(grep "^drift:" "$sizes_file" 2>/dev/null | cut -d: -f2 || echo 0)

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

      # Trigger digest on high accumulated observations (separate from Visionary threshold)
      # Uses digest markers to check for 10+ new lines across observation files
      if [[ -x "./digest.sh" ]]; then
        local digest_new=0
        local markers_file="$LOCK_DIR/digest_markers"
        for f in BLOCKERS.md CUTS.md DRIFT.md VISION_REVIEW.md; do
          local curr_lines=$(wc -l < "$WORK_DIR/$f" 2>/dev/null || echo 0)
          local marker=$(grep "^${f}:" "$markers_file" 2>/dev/null | cut -d: -f2 || echo 0)
          digest_new=$((digest_new + curr_lines - ${marker:-0}))
        done
        if (( digest_new >= 10 )); then
          log "${BLUE}[Watcher]${NC} High observation accumulation ($digest_new lines), triggering digest..."
          ./digest.sh &
        fi
      fi
    fi

    # Check if VISION_REVIEW.md grew — trigger review gate if enabled
    local last_review=$(grep "^review:" "$sizes_file" 2>/dev/null | cut -d: -f2 || echo 0)
    local curr_review=$(wc -l < "$WORK_DIR/VISION_REVIEW.md" 2>/dev/null || echo 0)
    if (( curr_review > last_review )) && [[ "$ENABLE_REVIEW_GATE" == "true" ]]; then
      touch "$VISION_REVIEW_SIGNAL"
      log "${YELLOW}[ReviewGate]${NC} VISION_REVIEW.md grew, Executor paused until review (touch .vision-reviewed)"
    fi

    # Update sizes file
    {
      echo "blockers:$curr_blockers"
      echo "cuts:$curr_cuts"
      echo "drift:$curr_drift"
      echo "review:$curr_review"
    } > "$sizes_file"
  }

  if [[ "$USE_FSWATCH" == true ]]; then
    fswatch -o "$WORK_DIR/BLOCKERS.md" "$WORK_DIR/CUTS.md" "$WORK_DIR/DRIFT.md" 2>/dev/null | while read -r _; do
      _handle_signal_change || break
    done
  else
    while read -r dir event file; do
      _handle_signal_change || break
    done < <(inotifywait -m -e close_write,moved_to "$WORK_DIR/BLOCKERS.md" "$WORK_DIR/CUTS.md" "$WORK_DIR/DRIFT.md" 2>/dev/null)
  fi
}

# Git commit watcher - event-driven via hook signal file
watch_commits() {
  log "${GREEN}[Watcher]${NC} Monitoring git commits via hook signal..."

  # Create signal file if it doesn't exist
  touch "$GIT_SIGNAL"

  _handle_commit_signal() {
    local file="$1"
    # Only react to our signal file
    [[ "$file" == "$(basename "$GIT_SIGNAL")" || "$file" == "$GIT_SIGNAL" ]] || return 0

    if check_done; then
      log "${GREEN}[Watcher]${NC} DONE.md exists, stopping commit watcher"
      return 1
    fi

    local current_commit
    current_commit=$(git rev-parse HEAD 2>/dev/null || echo "none")
    log "${BLUE}[Watcher]${NC} New commit detected: ${current_commit:0:8}"

    # After a commit: continue executing, check gaps, plan if needed
    if check_approval_gate && check_review_gate; then
      executor &
    fi
    sleep 3
    gap_finder "$current_commit" &
    sleep 3
    planner &

    # Refactoring signal: every REFACTOR_INTERVAL commits, nudge Visionary
    local lockdir="$COMMIT_COUNT_FILE.lock.d"
    _atomic_lock "$lockdir"
    local cc=$(cat "$COMMIT_COUNT_FILE" 2>/dev/null || echo 0)
    cc=$((cc + 1))
    echo "$cc" > "$COMMIT_COUNT_FILE"
    _atomic_unlock "$lockdir"
    if (( cc % REFACTOR_INTERVAL == 0 )); then
      log "${BLUE}[Watcher]${NC} $cc commits since last refactoring check, signaling Visionary..."
      add_signal_count 1
    fi

    # Only invoke Oracle when all tasks are complete (saves expensive Opus calls)
    if all_tasks_complete; then
      log "${BLUE}[Watcher]${NC} All tasks complete, invoking Oracle..."
      sleep 5
      oracle &
    fi
  }

  if [[ "$USE_FSWATCH" == true ]]; then
    fswatch -o "$GIT_SIGNAL" 2>/dev/null | while read -r _; do
      _handle_commit_signal "$GIT_SIGNAL" || break
    done
  else
    while read -r dir event file; do
      _handle_commit_signal "$file" || break
    done < <(inotifywait -m -e close_write,moved_to,create "$(dirname "$GIT_SIGNAL")" 2>/dev/null)
  fi
}

# Periodic heartbeat for agents that need regular checks
heartbeat() {
  local interval=${1:-600}  # Default 10 minutes (was 5, too aggressive)

  while true; do
    if check_done; then
      log "${GREEN}[Heartbeat]${NC} DONE.md exists, stopping heartbeat"
      break
    fi

    sleep "$interval"
    log "${BLUE}[Heartbeat]${NC} Periodic check..."

    # Primary: Run Executor if pending tasks and not already running
    if grep -q '\- \[ \]' "$WORK_DIR/PLAN.md" && [[ ! -d "$LOCK_DIR/running_Executor" ]]; then
      if check_approval_gate && check_review_gate; then
        executor &
        sleep 5
      fi
    fi

    # Secondary: Run Planner if Executor not running (auto-skips if queue full)
    if [[ ! -d "$LOCK_DIR/running_Executor" ]]; then
      planner &
    fi

    # Safety net: if all tasks complete, also invoke Oracle
    if all_tasks_complete; then
      log "${BLUE}[Heartbeat]${NC} All tasks complete, invoking Oracle..."
      sleep 5
      oracle &
    fi
  done
}

# ============================================
# CLEANUP
# ============================================

cleanup_and_exit() {
  # Ignore signals during cleanup to prevent re-entry
  trap '' SIGINT SIGTERM
  log "${YELLOW}[Main]${NC} Shutting down..."
  # Clean up lock state before force-killing (traps won't fire after SIGKILL)
  rm -rf "$LOCK_DIR"/running_* "$LOCK_DIR"/agent_slot_* 2>/dev/null || true
  # Kill all processes in our process group (graceful)
  kill 0 2>/dev/null || true
  # Brief grace period then force-kill stragglers (skip ourselves)
  sleep 1
  local pgid
  pgid=$(ps -o pgid= -p $$ 2>/dev/null | tr -d ' ') || true
  if [[ -n "$pgid" ]]; then
    local pids
    pids=$(pgrep -g "$pgid" 2>/dev/null | grep -v "^$$$" || true)
    if [[ -n "$pids" ]]; then
      echo "$pids" | xargs kill -9 2>/dev/null || true
    fi
  fi
  exit 0
}

# ============================================
# MAIN
# ============================================

main() {
  init_workspace
  
  log "${GREEN}========================================${NC}"
  log "${GREEN}  Grow.sh - Event-Driven Agent System${NC}"
  if [[ "$GROW_MODE" == "pair" ]]; then
    log "${GREEN}  Mode: ${GROW_MODE} (${PAIR_STYLE}) | CI:${ENABLE_CI} Gate:${ENABLE_APPROVAL_GATE} ReviewGate:${ENABLE_REVIEW_GATE} TDD:${ENABLE_TDD}${NC}"
  else
    log "${GREEN}  Mode: ${GROW_MODE} | CI:${ENABLE_CI} Gate:${ENABLE_APPROVAL_GATE} ReviewGate:${ENABLE_REVIEW_GATE} TDD:${ENABLE_TDD}${NC}"
  fi
  log "${GREEN}========================================${NC}"

  # Check dependencies (platform-specific file watcher)
  if [[ "$USE_FSWATCH" == true ]]; then
    if ! command -v fswatch &> /dev/null; then
      log "${RED}[Error]${NC} fswatch not found. Install with: brew install fswatch"
      exit 1
    fi
  else
    if ! command -v inotifywait &> /dev/null; then
      log "${RED}[Error]${NC} inotifywait not found. Install with: apt install inotify-tools"
      exit 1
    fi
  fi
  
  # Both tiers: digest.sh always uses the cheap one, even if SMART_AGENTS covers every agent
  require_claude_cmd CLAUDE_SMART_CMD
  require_claude_cmd CLAUDE_CHEAP_CMD
  
  if check_done; then
    log "${GREEN}[Main]${NC} DONE.md already exists. Vision was achieved!"
    cat "$WORK_DIR/DONE.md"
    exit 0
  fi

  # Trap for cleanup — set early so Ctrl+C works during startup checks too
  trap 'cleanup_and_exit' SIGINT SIGTERM

  # Validate vision before starting work (background + wait so trap fires immediately)
  if [[ "$_SKIP_STARTUP_VISIONARY" == "false" ]]; then
    # Pair mode: always validate
    log "${BLUE}[Main]${NC} Validating vision (pair mode — always validates)..."
    visionary "STARTUP CHECK: No work has begun yet. Focus on whether VISION.md is specific, measurable, and actionable. Flag any issues that would cause agents to struggle." &
    wait $! 2>/dev/null || true
    save_vision_hash
  elif should_skip_startup_visionary; then
    log "${GREEN}[Main]${NC} Vision unchanged since last validation, skipping startup Visionary"
  else
    log "${BLUE}[Main]${NC} Validating vision..."
    visionary "STARTUP CHECK: No work has begun yet. Focus on whether VISION.md is specific, measurable, and actionable. Flag any issues that would cause agents to struggle." &
    wait $! 2>/dev/null || true
    save_vision_hash
  fi

  log "${BLUE}[Main]${NC} Starting watchers..."
  
  # Start event watchers
  watch_plan &
  watch_vision &
  watch_commits &
  watch_signals &
  watch_approval &
  watch_review &
  heartbeat 600 &
  
  # Initial kick-off
  local task_count
  task_count=$(grep -c '\- \[.\]' "$WORK_DIR/PLAN.md" 2>/dev/null) || task_count=0

  if (( task_count > 0 )); then
    log "${BLUE}[Main]${NC} Found pending tasks, starting executor..."
    sleep 2
    if check_approval_gate && check_review_gate; then
      if [[ "$GROW_MODE" == "pair" ]]; then
        executor   # Foreground — interactive session
      else
        executor &
      fi
    fi
  else
    # Empty plan: populate all tasks from vision in one shot
    log "${BLUE}[Main]${NC} Empty plan, populating initial tasks from vision..."
    sleep 2
    run_agent "Planner" "You are the Planner agent. PLAN.md is empty. Review VISION.md and populate PLAN.md with ALL tasks needed to fulfill the vision. Break the vision into concrete, atomic, actionable tasks. Format each as '- [ ] <task description>'. Order them logically." &
    wait $! 2>/dev/null || true
    # Chain into executor now that plan is populated
    if grep -q '\- \[ \]' "$WORK_DIR/PLAN.md" 2>/dev/null && [[ ! -d "$LOCK_DIR/running_Executor" ]]; then
      if check_approval_gate && check_review_gate; then
        if [[ "$GROW_MODE" == "pair" ]]; then
          executor   # Foreground — interactive session
        else
          executor &
        fi
      fi
    fi
  fi

  if [[ "$GROW_MODE" == "pair" ]]; then
    local final_remaining
    final_remaining=$(grep -c '\- \[ \]' "$WORK_DIR/PLAN.md" 2>/dev/null) || final_remaining=0
    if (( final_remaining > 0 )); then
      echo ""
      log "${GREEN}[Main]${NC} $final_remaining tasks remaining. Watchers still active."
      log "${GREEN}[Main]${NC} Run './grow.sh executor' to resume, or Ctrl+C to stop."
    fi
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
    echo "=== CI STATUS ==="
    if [[ -f "$LOCK_DIR/ci.log" ]]; then
      tail -3 "$LOCK_DIR/ci.log"
    else
      echo "(no CI runs yet)"
    fi
    echo ""
    echo "=== GROW MODE ==="
    echo "Mode: $GROW_MODE"
    echo "CI: $ENABLE_CI | Gate: $ENABLE_APPROVAL_GATE | ReviewGate: $ENABLE_REVIEW_GATE | TDD: $ENABLE_TDD"
    echo ""
    echo "=== APPROVAL GATE ==="
    if [[ "$ENABLE_APPROVAL_GATE" == "true" ]]; then
      if [[ -f "$APPROVAL_SIGNAL" ]]; then
        echo "Status: APPROVED (pending consumption)"
      else
        echo "Status: WAITING (touch .plan-approved to approve)"
      fi
    else
      echo "Status: DISABLED"
    fi
    echo ""
    echo "=== REVIEW GATE ==="
    if [[ "$ENABLE_REVIEW_GATE" == "true" ]]; then
      if [[ -f "$VISION_REVIEW_SIGNAL" ]]; then
        if [[ -f "$REVIEW_ACK_SIGNAL" ]]; then
          echo "Status: ACKNOWLEDGED (pending consumption)"
        else
          echo "Status: PENDING (touch .vision-reviewed to acknowledge)"
        fi
      else
        echo "Status: CLEAR (no pending review)"
      fi
    else
      echo "Status: DISABLED"
    fi
    echo ""
    echo "=== RECENT LOG ==="
    tail -20 "$LOG_FILE" 2>/dev/null || echo "(empty)"
    ;;
  *)
    echo "Usage: $0 {start|executor|planner|gap|oracle|visionary|status}"
    exit 1
    ;;
esac
