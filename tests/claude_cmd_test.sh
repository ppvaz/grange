# Which claude command each agent tier runs (lib/claude-cmd.sh)

test_start_fails_fast_when_cheap_cmd_is_not_executable() {
  new_project
  local output
  output=$(timeout 20 ./grow.sh start 2>&1) && fail "grow.sh start should have refused to run"
  assert_contains "$output" "'claude-cheap' (from CLAUDE_CHEAP_CMD) is not an executable on PATH"
  [[ ! -s "$STUB_LOG" ]] || fail "no agent should have run"
}

test_cheap_agents_run_cheap_cmd_from_env_file() {
  new_project
  echo 'CLAUDE_CHEAP_CMD="claude --model cheap-model"' > .env
  timeout 60 ./grow.sh executor > /dev/null 2>&1
  assert_contains "$(cat "$STUB_LOG")" "--model cheap-model --allowedTools"
}

test_smart_agents_run_smart_cmd() {
  new_project
  CLAUDE_SMART_CMD="claude --model smart-model" timeout 60 ./grow.sh oracle > /dev/null 2>&1
  assert_contains "$(cat "$STUB_LOG")" "--model smart-model --allowedTools"
}

test_cmd_can_be_an_absolute_path() {
  new_project
  CLAUDE_CHEAP_CMD="$TMP/bin/claude --model abs-model" timeout 60 ./grow.sh planner > /dev/null 2>&1
  assert_contains "$(cat "$STUB_LOG")" "--model abs-model --allowedTools"
}

test_digest_runs_cheap_cmd() {
  new_project
  echo "Blocked on credentials" > BLOCKERS.md
  CLAUDE_CHEAP_CMD="claude --model cheap-model" ./digest.sh > /dev/null 2>&1
  assert_contains "$(cat "$STUB_LOG")" "--model cheap-model -p"
}

test_distill_fails_fast_when_cheap_cmd_is_not_executable() {
  new_project
  local output
  output=$(./distill.sh 2>&1) && fail "distill.sh should have refused to run"
  assert_contains "$output" "'claude-cheap' (from CLAUDE_CHEAP_CMD) is not an executable on PATH"
}
