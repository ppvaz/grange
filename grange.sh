#!/bin/bash
# grange - Global CLI for the Grange toolkit
#
# Install globally:
#   ln -s /path/to/grange/grange.sh /usr/local/bin/grange
#
# Usage:
#   grange init <directory>   Create a new project scaffold
#   grange adopt [directory]  Bring grange into an existing project
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
  dashboard [dir]   Launch the dashboard for a project (default: current dir)
  help              Show this help

Examples:
  grange init my-app
  grange init ~/Projects/client-rebuild
  cd ~/Projects/existing-app && grange adopt
  grange adopt ~/Projects/existing-app
  grange dashboard
EOF
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
  port_in_use() {
    if command -v ss &>/dev/null; then
      ss -tlnp 2>/dev/null | grep -q ":$1 "
    elif command -v lsof &>/dev/null; then
      lsof -iTCP:"$1" -sTCP:LISTEN &>/dev/null
    else
      return 1
    fi
  }
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
  dashboard)
    launch_dashboard "$(cd "${2:-.}" && pwd)"
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
