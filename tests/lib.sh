# tests/lib.sh - Helpers loaded into every test by tests/run.sh

GRANGE=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

# Creates a throwaway grange project at $PROJECT and cds into it. Every agent
# CLI (claude, codex, agy, opencode) on PATH is a stub that appends its PID to
# $STUB_PIDS and "<name> <args>" to $STUB_LOG, then evals $STUB_BODY.
# claude-cheap is kept off PATH so every machine sees the same default config.
new_project() {
  TMP=$(mktemp -d)
  trap cleanup_project EXIT
  PROJECT="$TMP/project"
  export STUB_LOG="$TMP/stub.log" STUB_PIDS="$TMP/stub.pids"
  mkdir -p "$TMP/bin" "$PROJECT"
  : > "$STUB_LOG"
  : > "$STUB_PIDS"

  cat > "$TMP/bin/claude" <<'EOF'
#!/bin/bash
echo $$ >> "$STUB_PIDS"
printf '%s %s\n' "$(basename "$0")" "$*" >> "$STUB_LOG"
eval "${STUB_BODY:-}"
EOF
  chmod +x "$TMP/bin/claude"
  for cli in codex agy opencode; do
    ln -s claude "$TMP/bin/$cli"
  done

  local dir path="$TMP/bin"
  local -a dirs
  IFS=: read -ra dirs <<< "$PATH"
  for dir in "${dirs[@]}"; do
    [[ -x "$dir/claude-cheap" ]] || path+=":$dir"
  done
  export PATH="$path"
  unset CLAUDE_SMART_CMD CLAUDE_CHEAP_CMD GROW_MODE SMART_AGENTS

  cd "$PROJECT"
  for f in grow.sh reap.sh digest.sh distill.sh visions; do
    ln -s "$GRANGE/$f" .
  done
  printf '# Vision\n\n## Done When\n- [ ] hello.txt exists\n' > VISION.md
  printf -- '- [ ] Write hello.txt\n' > PLAN.md
  : > .env  # keeps grow.sh from falling back to grange's own .env
  git init -q
  git add -A
  git -c user.name=test -c user.email=test@example.com commit -qm init
}

cleanup_project() {
  # Stubs outlive their callers when a test fails; don't leak them
  kill $(cat "$STUB_PIDS") 2>/dev/null
  rm -rf "$TMP"
}

fail() {
  echo "$*" >&2
  exit 1
}

assert_contains() {
  [[ "$1" == *"$2"* ]] || fail "expected to find: $2"$'\n'"in: $1"
}

# Usage: wait_for <seconds> <command...>
wait_for() {
  local deadline=$((SECONDS + $1))
  shift
  until "$@"; do
    (( SECONDS < deadline )) || fail "timed out waiting for: $*"
    sleep 0.2
  done
}

grange() {
  "$GRANGE/lib/launch.sh" "$@"
}

# Stub body that writes $1 to the answer file named in a `grange agent run` prompt
answer_with() {
  printf 'printf %%s %q > "$(printf %%s "$*" | sed -nE %q)"' "$1" 's/.*to the file (.+)\. Don.t create.*/\1/p'
}
