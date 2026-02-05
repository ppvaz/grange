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
