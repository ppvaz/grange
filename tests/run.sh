#!/bin/bash
# tests/run.sh - Run grange's tests. Every test_* function in tests/*_test.sh
# runs in its own bash process with tests/lib.sh loaded.
#
# Usage: tests/run.sh [test-file...]

set -uo pipefail

cd "$(dirname "$0")"
files=("$@")
(( ${#files[@]} )) || files=(*_test.sh)

output=$(mktemp)
trap 'rm -f "$output"' EXIT

passed=0
failed=0
for file in "${files[@]}"; do
  file=$(basename "$file")
  for t in $(bash -c "source ./$file && declare -F" | awk '$3 ~ /^test_/ { print $3 }'); do
    # To a file, not $(...): a test that leaks a background process would
    # otherwise hang the runner until that process exits.
    if bash -c "source ./lib.sh && source ./$file && $t" > "$output" 2>&1; then
      echo "ok   $file $t"
      passed=$((passed + 1))
    else
      echo "FAIL $file $t"
      sed 's/^/     /' "$output"
      failed=$((failed + 1))
    fi
  done
done

echo "$passed passed, $failed failed"
(( failed == 0 ))
