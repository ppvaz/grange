#!/bin/bash
# grange - Global CLI for the Grange toolkit
#
# Install globally:
#   ln -s /path/to/grange/grange.sh /usr/local/bin/grange
#
# Usage:
#   grange init <directory>   Create a new project scaffold
#   grange help               Show this help

set -euo pipefail

# Resolve grange home from this script's real location
GRANGE_HOME="$(cd "$(dirname "$(readlink -f "$0")")" && pwd)"

usage() {
  cat <<'EOF'
Usage: grange <command> [options]

Commands:
  init <dir>    Scaffold a new project directory with symlinks to grange
  help          Show this help

Examples:
  grange init my-app
  grange init ~/Projects/client-rebuild
EOF
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

  echo
  echo "Done. Next steps:"
  echo "  cd $target"
  if [[ ! -f "$target/.env" ]]; then
    echo "  cp .env.example .env    # add your API keys"
  fi
  echo "  edit VISION.md           # define what you're building"
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
  help|--help|-h)
    usage
    ;;
  *)
    echo "Unknown command: $1" >&2
    usage
    exit 1
    ;;
esac
