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

test_dashboard_all_parses_its_arguments() {
  # With a built dashboard this would start a server; only the argument handling is under test
  [[ -x "$GRANGE/dashboard/grange-dashboard" ]] && return 0
  new_project
  local output
  output=$("$GRANGE/grange.sh" dashboard --all "$TMP" 2>&1) || fail "dashboard --all failed: $output"
  assert_contains "$output" "dashboard binary not found"
}
