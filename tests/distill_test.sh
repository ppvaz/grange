# distill.sh: cross-project pattern discovery

test_distill_fails_fast_when_cheap_cmd_is_not_executable() {
  new_project
  local output
  output=$(./distill.sh 2>&1) && fail "distill.sh should have refused to run"
  assert_contains "$output" "'claude-cheap' (from CLAUDE_CHEAP_CMD) is not an executable on PATH"
}
