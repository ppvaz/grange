# grange.sh: the global CLI

test_grange_forwards_grow_and_agent_commands() {
  new_project
  local output
  output=$("$GRANGE/grange.sh" grow status) || fail "grange grow status failed: $output"
  assert_contains "$output" "=== PLAN (pending) ==="
  assert_contains "$output" "- [ ] Write hello.txt"
  output=$(GRANGE_CHEAP=codex:test-model "$GRANGE/grange.sh" agent check Executor) || fail "agent check failed: $output"
  assert_contains "$output" "Executor   codex:test-model"
}
