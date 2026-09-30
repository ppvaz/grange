# grange agent: backend-neutral model calls for scripts

test_agent_run_prints_the_answer_file() {
  new_project
  export STUB_BODY="$(answer_with 'forty-two')"
  local output
  output=$(echo "What is the answer?" | grange agent run --role Distill) || fail "agent run failed: $output"
  [[ "$output" == "forty-two" ]] || fail "expected the answer file's contents, got: $output"
  assert_contains "$(cat "$STUB_LOG")" "What is the answer?"
}

test_agent_run_fails_without_an_answer() {
  new_project
  local output
  output=$(echo "hi" | grange agent run 2>&1) && fail "should fail when no answer is written"
  assert_contains "$output" "finished without writing an answer"
}

test_agent_check_reports_missing_backend() {
  new_project
  rm "$TMP/bin/codex"
  grange agent check > /dev/null  # build the binary while go is still on PATH
  local output
  output=$(PATH="$TMP/bin:/usr/bin:/bin" GRANGE_CHEAP=codex:model-b@low grange agent check Executor Oracle) && fail "codex should be missing"
  assert_contains "$output" "Executor   codex:model-b@low"
  assert_contains "$output" "NOT FOUND on PATH"
  assert_contains "$output" "Oracle     claude"
}
