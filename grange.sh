#!/bin/bash
# grange - Global CLI for the Grange toolkit
#
# Install globally:
#   ln -s /path/to/grange/grange.sh /usr/local/bin/grange
#
# Usage:
#   grange init <directory>   Create a new project scaffold
#   grange adopt [directory]  Bring grange into an existing project
#   grange eject [directory]  Remove grange from a completed project
#   grange dashboard [directory]  Launch the dashboard for a project
#   grange help               Show this help

set -euo pipefail

# Resolve grange home from this script's real location
GRANGE_HOME="$(cd "$(dirname "$(readlink -f "$0")")" && pwd)"

usage() {
  cat <<'EOF'
Usage: grange <command> [options]

Commands:
  init <dir>        Scaffold a new project directory with symlinks to grange
  adopt [dir]       Bring grange into an existing project (default: current dir)
  eject [dir]       Remove grange from a completed project (default: current dir)
  dashboard [dir]   Launch the dashboard for a project (default: current dir)
  dashboard --all [dir]  Launch multi-project overview dashboard
  status [dir]      Show status of all grange projects in a directory
  help              Show this help

Examples:
  grange init my-app
  grange init ~/Projects/client-rebuild
  cd ~/Projects/existing-app && grange adopt
  grange adopt ~/Projects/existing-app
  grange eject ~/Projects/completed-app
  grange dashboard
  grange dashboard --all ~/Projects
  grange status
  grange status ~/Projects
EOF
}

# --- Utilities ---

port_in_use() {
  if command -v ss &>/dev/null; then
    ss -tlnp 2>/dev/null | grep -q ":$1 "
  elif command -v lsof &>/dev/null; then
    lsof -iTCP:"$1" -sTCP:LISTEN &>/dev/null
  else
    return 1
  fi
}

launch_dashboard() {
  local target="$1"
  local dashboard_bin="$GRANGE_HOME/dashboard/grange-dashboard"

  if [[ ! -x "$dashboard_bin" ]]; then
    echo "  dashboard binary not found, skipping"
    return
  fi

  # Find a free port starting from 3000
  local port=3000
  while port_in_use "$port"; do
    ((port++))
    if [[ $port -gt 3099 ]]; then
      echo "  no free port found (3000-3099), skipping dashboard"
      return
    fi
  done

  "$dashboard_bin" -w "$target" -g "$GRANGE_HOME" -p "$port" &
  local pid=$!
  disown "$pid" 2>/dev/null

  # Wait briefly for server to start
  sleep 0.3

  local url="http://localhost:$port"
  echo "  dashboard running at $url (pid $pid)"

  # Open in browser
  if command -v xdg-open &>/dev/null; then
    xdg-open "$url" 2>/dev/null &
  elif command -v open &>/dev/null; then
    open "$url" &
  fi
}

# --- Multi-project discovery & status ---

discover_projects() {
  local scan_dir="$1"
  local projects=()
  for d in "$scan_dir"/*/; do
    [[ -d "$d" ]] || continue
    local grow="$d/grow.sh"
    if [[ -L "$grow" ]]; then
      local target
      target="$(readlink -f "$grow" 2>/dev/null)" || continue
      if [[ "$target" == "$GRANGE_HOME/grow.sh" ]]; then
        projects+=("$(cd "$d" && pwd)")
      fi
    fi
  done
  printf '%s\n' "${projects[@]}"
}

project_status() {
  local dir="$1"
  local name
  name="$(basename "$dir")"

  # Plan progress
  local done=0 total=0
  if [[ -f "$dir/PLAN.md" ]]; then
    done=$(grep -c '^\s*- \[x\]' "$dir/PLAN.md" 2>/dev/null || true)
    local pending
    pending=$(grep -c '^\s*- \[ \]' "$dir/PLAN.md" 2>/dev/null || true)
    total=$((done + pending))
  fi

  # Git stats
  local commits=0 last_activity="never" last_epoch=0
  if [[ -d "$dir/.git" ]]; then
    commits=$(git -C "$dir" rev-list --count HEAD 2>/dev/null || echo 0)
    local git_epoch
    git_epoch=$(git -C "$dir" log -1 --format='%ct' 2>/dev/null || echo 0)
    if [[ "$git_epoch" -gt 0 ]]; then
      last_epoch=$git_epoch
      local now
      now=$(date +%s)
      local diff=$((now - git_epoch))
      if [[ $diff -lt 3600 ]]; then
        last_activity="$((diff / 60))m ago"
      elif [[ $diff -lt 86400 ]]; then
        last_activity="$((diff / 3600))h ago"
      else
        last_activity="$((diff / 86400))d ago"
      fi
    fi
  fi

  # Goal line
  local goal="-"
  if [[ -f "$dir/VISION.md" ]]; then
    goal=$(awk '/^## Goal/{getline; while(/^[[:space:]]*$/){getline}; print; exit}' "$dir/VISION.md" 2>/dev/null)
    [[ -z "$goal" ]] && goal="-"
    # Truncate long goals
    if [[ ${#goal} -gt 50 ]]; then
      goal="${goal:0:47}..."
    fi
  fi

  # Signal files count
  local signals=0
  for f in BLOCKERS.md CUTS.md DRIFT.md VISION_REVIEW.md; do
    if [[ -f "$dir/$f" ]] && [[ -s "$dir/$f" ]]; then
      ((signals++))
    fi
  done

  # Status logic
  local status
  if [[ -f "$dir/DONE.md" ]]; then
    status="done"
  elif [[ $last_epoch -gt 0 ]] && [[ $(($(date +%s) - last_epoch)) -lt 604800 ]]; then
    status="active"
  elif [[ $total -gt 0 ]]; then
    status="stalled"
  else
    status="new"
  fi

  # Progress string
  local progress="-"
  if [[ $total -gt 0 ]]; then
    progress="$done/$total"
  fi

  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$name" "$progress" "$commits" "$last_activity" "$status" "$goal"
}

cmd_status() {
  local scan_dir="$1"

  # Default scan dir: parent of cwd if inside a grange project, else ~/Projects
  if [[ -z "$scan_dir" ]]; then
    if [[ -L "./grow.sh" ]]; then
      scan_dir="$(cd .. && pwd)"
    elif [[ -d "$HOME/Projects" ]]; then
      scan_dir="$HOME/Projects"
    else
      scan_dir="$(pwd)"
    fi
  fi
  scan_dir="$(cd "$scan_dir" && pwd)"

  local projects
  projects=$(discover_projects "$scan_dir")
  if [[ -z "$projects" ]]; then
    echo "No grange projects found in $scan_dir"
    return
  fi

  # Collect project data
  local data=()
  while IFS= read -r dir; do
    data+=("$(project_status "$dir")")
  done <<< "$projects"

  # Sort: active first, then stalled, new, done
  local sorted=()
  for status_filter in active stalled new done; do
    for line in "${data[@]}"; do
      local s
      s=$(echo "$line" | cut -f5)
      [[ "$s" == "$status_filter" ]] && sorted+=("$line")
    done
  done

  # Color codes
  local green='\033[32m' amber='\033[33m' red='\033[31m' dim='\033[2m' reset='\033[0m' bold='\033[1m'

  # Header
  echo
  printf "${bold}%-20s %-10s %-8s %-14s %-8s %s${reset}\n" "PROJECT" "PROGRESS" "COMMITS" "LAST ACTIVITY" "STATUS" "GOAL"
  printf '%.0s─' {1..100}; echo

  for line in "${sorted[@]}"; do
    local name progress commits last_activity status goal
    IFS=$'\t' read -r name progress commits last_activity status goal <<< "$line"

    local status_colored
    case "$status" in
      active)  status_colored="${green}active${reset}" ;;
      stalled) status_colored="${amber}stalled${reset}" ;;
      new)     status_colored="${dim}new${reset}" ;;
      done)    status_colored="${green}done${reset}" ;;
    esac

    printf "%-20s %-10s %-8s %-14s " "$name" "$progress" "$commits" "$last_activity"
    printf "${status_colored}"
    printf "%-*s" $((8 - ${#status})) ""
    printf " %s\n" "$goal"
  done
  echo
  printf "${dim}Scan: %s  |  %d projects${reset}\n" "$scan_dir" "${#sorted[@]}"
  echo
}

launch_dashboard_all() {
  local scan_dir="$1"
  local dashboard_bin="$GRANGE_HOME/dashboard/grange-dashboard"

  if [[ ! -x "$dashboard_bin" ]]; then
    echo "  dashboard binary not found, skipping"
    return
  fi

  local port=3000
  while port_in_use "$port"; do
    ((port++))
    if [[ $port -gt 3099 ]]; then
      echo "  no free port found (3000-3099), skipping dashboard"
      return
    fi
  done

  "$dashboard_bin" -a -s "$scan_dir" -g "$GRANGE_HOME" -p "$port" &
  local pid=$!
  disown "$pid" 2>/dev/null

  sleep 0.3

  local url="http://localhost:$port"
  echo "  dashboard running at $url (pid $pid)"
  echo "  scanning: $scan_dir"

  if command -v xdg-open &>/dev/null; then
    xdg-open "$url" 2>/dev/null &
  elif command -v open &>/dev/null; then
    open "$url" &
  fi
}

cmd_init() {
  local target="$1"

  if [[ -e "$target" ]]; then
    echo "Error: '$target' already exists." >&2
    exit 1
  fi

  echo "Scaffolding new project at: $target"
  echo "Grange home: $GRANGE_HOME"
  echo

  mkdir -p "$target"

  # Symlink scripts
  local scripts=(grow.sh reap.sh harvest.sh distill.sh digest.sh)
  for s in "${scripts[@]}"; do
    if [[ -f "$GRANGE_HOME/$s" ]]; then
      ln -s "$GRANGE_HOME/$s" "$target/$s"
      echo "  linked $s"
    fi
  done

  # Symlink visions directory
  ln -s "$GRANGE_HOME/visions" "$target/visions"
  echo "  linked visions/"

  # Symlink specs directory
  ln -s "$GRANGE_HOME/specs" "$target/specs"
  echo "  linked specs/"

  # Copy config files (these are project-specific, not symlinked)
  cp "$GRANGE_HOME/.env.example" "$target/.env.example"
  echo "  copied .env.example"

  # Copy .env from grange home if it exists (so API keys carry over)
  if [[ -f "$GRANGE_HOME/.env" ]]; then
    cp "$GRANGE_HOME/.env" "$target/.env"
    echo "  copied .env (from grange home)"
  fi

  # Create project-specific .gitignore
  cat > "$target/.gitignore" <<'GITIGNORE'
# Grange toolkit (symlinks back to grange install)
grow.sh
reap.sh
harvest.sh
distill.sh
digest.sh
visions
specs

# Secrets
.env
.env.local

# Runtime state
.locks/
.ike-state
.git-commit-signal

# Logs
LOG.md
*.log

# Agent working files (review before committing)
# PLAN.md
# BLOCKERS.md
# CUTS.md
# DRIFT.md
# VISION_REVIEW.md
# HUMAN_DIGEST.md
# DONE.md
GITIGNORE
  echo "  created .gitignore"

  # Create placeholder VISION.md
  cat > "$target/VISION.md" <<'VISION'
# Vision

<!-- Write your project vision here. This is the contract agents work toward. -->
<!-- See visions/ike-v3/ for multi-stage pipeline templates. -->
VISION
  echo "  created VISION.md"

  # Initialize git repo
  git -C "$target" init -q
  git -C "$target" add -A
  git -C "$target" commit -q -m "Initial scaffold (grange init)"
  echo "  initialized git repo"

  # Resolve to absolute path for dashboard
  target="$(cd "$target" && pwd)"
  launch_dashboard "$target"

  echo
  echo "Done. Next steps:"
  echo "  cd $target"
  if [[ ! -f "$target/.env" ]]; then
    echo "  cp .env.example .env    # add your API keys"
  fi
  echo "  edit VISION.md           # define what you're building"
  echo "  ./grow.sh start          # let agents work"
}

cmd_adopt() {
  local target="${1:-.}"
  target="$(cd "$target" && pwd)"

  if [[ ! -d "$target" ]]; then
    echo "Error: '$target' is not a directory." >&2
    exit 1
  fi

  echo "Adopting existing project: $target"
  echo "Grange home: $GRANGE_HOME"
  echo

  # Symlink scripts (skip if already present)
  local scripts=(grow.sh reap.sh harvest.sh distill.sh digest.sh)
  for s in "${scripts[@]}"; do
    if [[ -e "$target/$s" ]]; then
      echo "  skip $s (already exists)"
    elif [[ -f "$GRANGE_HOME/$s" ]]; then
      ln -s "$GRANGE_HOME/$s" "$target/$s"
      echo "  linked $s"
    fi
  done

  # Symlink visions directory
  if [[ -e "$target/visions" ]]; then
    echo "  skip visions/ (already exists)"
  else
    ln -s "$GRANGE_HOME/visions" "$target/visions"
    echo "  linked visions/"
  fi

  # Symlink specs directory
  if [[ -e "$target/specs" ]]; then
    echo "  skip specs/ (already exists)"
  else
    ln -s "$GRANGE_HOME/specs" "$target/specs"
    echo "  linked specs/"
  fi

  # Copy config files (don't overwrite existing)
  if [[ -e "$target/.env.example" ]]; then
    echo "  skip .env.example (already exists)"
  else
    cp "$GRANGE_HOME/.env.example" "$target/.env.example"
    echo "  copied .env.example"
  fi

  if [[ -e "$target/.env" ]]; then
    echo "  skip .env (already exists)"
  elif [[ -f "$GRANGE_HOME/.env" ]]; then
    cp "$GRANGE_HOME/.env" "$target/.env"
    echo "  copied .env (from grange home)"
  fi

  # Append grange entries to .gitignore (create if missing)
  local grange_marker="# Grange runtime"
  if [[ -f "$target/.gitignore" ]] && grep -qF "$grange_marker" "$target/.gitignore"; then
    echo "  skip .gitignore (grange entries already present)"
  else
    cat >> "$target/.gitignore" <<'GITIGNORE'

# Grange toolkit (symlinks back to grange install)
grow.sh
reap.sh
harvest.sh
distill.sh
digest.sh
visions
specs

# Grange runtime
.env
.env.local
.locks/
.ike-state
.git-commit-signal
LOG.md
GITIGNORE
    echo "  updated .gitignore"
  fi

  # Create placeholder VISION.md if missing
  if [[ -e "$target/VISION.md" ]]; then
    echo "  skip VISION.md (already exists)"
  else
    cat > "$target/VISION.md" <<'VISION'
# Vision

<!-- Write your project vision here. This is the contract agents work toward. -->
<!-- See visions/ike-v3/ for multi-stage pipeline templates. -->
VISION
    echo "  created VISION.md"
  fi

  # Initialize git repo if not already one
  if [[ -d "$target/.git" ]]; then
    echo "  skip git init (already a repo)"
  else
    git -C "$target" init -q
    echo "  initialized git repo"
  fi

  launch_dashboard "$target"

  echo
  echo "Done. Next steps:"
  if [[ ! -f "$target/.env" ]]; then
    echo "  cp .env.example .env    # add your API keys"
  fi
  echo "  edit VISION.md           # define what agents should work toward"
  echo "  ./grow.sh start          # let agents work"
}

cmd_eject() {
  local target="${1:-.}"
  target="$(cd "$target" && pwd)"

  if [[ ! -d "$target" ]]; then
    echo "Error: '$target' is not a directory." >&2
    exit 1
  fi

  echo "Ejecting grange from: $target"
  echo

  # Run harvest to collect IKE specs and raw .md files
  "$GRANGE_HOME/harvest.sh" "$target"

  # Remove grange symlinks
  local scripts=(grow.sh reap.sh harvest.sh distill.sh digest.sh)
  for s in "${scripts[@]}"; do
    if [[ -L "$target/$s" ]]; then
      rm "$target/$s"
      echo "  removed $s"
    fi
  done

  for d in visions specs; do
    if [[ -L "$target/$d" ]]; then
      rm "$target/$d"
      echo "  removed $d"
    fi
  done

  # Remove runtime state
  local runtime=(.locks .ike-state .git-commit-signal LOG.md)
  for f in "${runtime[@]}"; do
    if [[ -e "$target/$f" ]]; then
      rm -rf "$target/$f"
      echo "  removed $f"
    fi
  done

  # Remove agent working files
  local workfiles=(PLAN.md BLOCKERS.md CUTS.md DRIFT.md VISION_REVIEW.md HUMAN_DIGEST.md DONE.md)
  for f in "${workfiles[@]}"; do
    if [[ -f "$target/$f" ]]; then
      rm "$target/$f"
      echo "  removed $f"
    fi
  done

  # Clean grange entries from .gitignore
  if [[ -f "$target/.gitignore" ]]; then
    # Remove the grange toolkit and runtime blocks
    sed -i '/^# Grange toolkit/,/^$/d; /^# Grange runtime/,/^$/d' "$target/.gitignore"
    # Remove any remaining grange-specific lines from init-style gitignore
    sed -i '/^grow\.sh$/d; /^reap\.sh$/d; /^harvest\.sh$/d; /^distill\.sh$/d; /^digest\.sh$/d' "$target/.gitignore"
    sed -i '/^visions$/d; /^specs$/d' "$target/.gitignore"
    sed -i '/^\.ike-state$/d; /^\.git-commit-signal$/d; /^\.locks\/$/d; /^LOG\.md$/d' "$target/.gitignore"
    # Clean up consecutive blank lines
    sed -i '/^$/N;/^\n$/d' "$target/.gitignore"
    echo "  cleaned .gitignore"
  fi

  echo
  echo "Done. Grange has been removed."
}

# --- Main ---

if [[ $# -eq 0 ]]; then
  usage
  exit 0
fi

case "${1:-}" in
  init)
    if [[ $# -lt 2 ]]; then
      echo "Error: 'init' requires a directory name." >&2
      echo "Usage: grange init <directory>" >&2
      exit 1
    fi
    cmd_init "$2"
    ;;
  adopt)
    cmd_adopt "${2:-.}"
    ;;
  eject)
    cmd_eject "${2:-.}"
    ;;
  dashboard)
    if [[ "${2:-}" == "--all" || "${2:-}" == "-a" ]]; then
      local scan_dir="${3:-}"
      if [[ -z "$scan_dir" ]]; then
        if [[ -L "./grow.sh" ]]; then
          scan_dir="$(cd .. && pwd)"
        elif [[ -d "$HOME/Projects" ]]; then
          scan_dir="$HOME/Projects"
        else
          scan_dir="$(pwd)"
        fi
      fi
      launch_dashboard_all "$(cd "$scan_dir" && pwd)"
    else
      launch_dashboard "$(cd "${2:-.}" && pwd)"
    fi
    ;;
  status)
    cmd_status "${2:-}"
    ;;
  help|--help|-h)
    usage
    ;;
  *)
    echo "Unknown command: $1" >&2
    usage
    exit 1
    ;;
esac
