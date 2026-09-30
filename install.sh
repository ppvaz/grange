#!/bin/bash
# Install the grange CLI globally
set -euo pipefail

GRANGE_HOME="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$GRANGE_HOME/grange.sh"

chmod +x "$SCRIPT"

# Try /usr/local/bin first, fall back to ~/.local/bin
if [[ -w /usr/local/bin ]]; then
  ln -sf "$SCRIPT" /usr/local/bin/grange
  echo "Installed: /usr/local/bin/grange -> $SCRIPT"
elif command -v sudo &>/dev/null && sudo -n true 2>/dev/null; then
  sudo ln -sf "$SCRIPT" /usr/local/bin/grange
  echo "Installed: /usr/local/bin/grange -> $SCRIPT"
else
  mkdir -p "$HOME/.local/bin"
  ln -sf "$SCRIPT" "$HOME/.local/bin/grange"
  echo "Installed: $HOME/.local/bin/grange -> $SCRIPT"
  if ! echo "$PATH" | tr ':' '\n' | grep -q "$HOME/.local/bin"; then
    echo
    echo "Add ~/.local/bin to your PATH:"
    echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc"
  fi
fi

# Build the orchestrator now rather than on the first grow/reap run
if command -v go &>/dev/null; then
  (cd "$GRANGE_HOME" && go build -o bin/grange ./cmd/grange)
  echo "Built: $GRANGE_HOME/bin/grange"
else
  echo
  echo "Go not found: grow.sh and reap.sh need it to build bin/grange (https://go.dev/dl)"
fi
