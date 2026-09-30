# grow: the plan → execute → review → verify loop, and which backend runs what

test_loop_runs_until_oracle_confirms() {
  new_project
  export STUB_BODY="$FAKE_AGENT"
  timeout 60 ./grow.sh start > /dev/null 2>&1 || fail "grow.sh start failed: $(tail -5 LOG.md)"
  assert_contains "$(cat DONE.md)" "All shipped."
  assert_contains "$(cat DONE.md)" "- [x] Write hello.txt"
  assert_contains "$(cat work.txt)" "Write hello.txt"
  assert_contains "$(cat LOG.md)" "Vision achieved"
}

test_empty_plan_is_populated_first() {
  new_project
  : > PLAN.md
  export STUB_BODY="$FAKE_AGENT"
  timeout 60 ./grow.sh start > /dev/null 2>&1 || fail "grow.sh start failed: $(tail -5 LOG.md)"
  assert_contains "$(cat PLAN.md)" "- [x] Write bye.txt"
  [[ -f DONE.md ]] || fail "expected DONE.md"
}

test_oracle_failures_become_fix_tasks() {
  new_project
  export STUB_BODY="$FAKE_AGENT" ORACLE_FAILS_ONCE=1
  timeout 60 ./grow.sh start > /dev/null 2>&1 || fail "grow.sh start failed: $(tail -5 LOG.md)"
  assert_contains "$(cat PLAN.md)" "- [x] Fix: make the tests pass"
  [[ "$(cat .locks/verify_cycles)" == 1 ]] || fail "expected one failed verify cycle"
  [[ -f DONE.md ]] || fail "expected DONE.md after the fix"
}

test_loop_stops_when_nothing_moves() {
  new_project
  local output
  output=$(timeout 60 ./grow.sh start 2>&1) && fail "a loop with no progress should stop with an error"
  assert_contains "$output" "no progress in 3 consecutive turns"
  [[ ! -f DONE.md ]] || fail "nothing was done"
}

test_approval_gate_holds_the_executor() {
  new_project
  export STUB_BODY="$FAKE_AGENT"
  ENABLE_APPROVAL_GATE=true timeout 60 ./grow.sh start > /dev/null 2>&1 &
  local grow=$!
  wait_for 20 grep -q "Waiting for approval" LOG.md
  [[ ! -f work.txt ]] || fail "Executor ran before approval"
  touch .plan-approved
  wait "$grow" || fail "grow.sh start failed: $(tail -5 LOG.md)"
  [[ -f DONE.md ]] || fail "expected DONE.md once approved"
}

test_backends_follow_tiers_and_overrides() {
  new_project
  printf '%s\n' 'GRANGE_SMART=agy:model-a' 'GRANGE_CHEAP=codex:model-b@low' \
    'GRANGE_AGENT_GAP=opencode:provider/model-c@high' > .env
  export STUB_BODY="$FAKE_AGENT"
  timeout 60 ./grow.sh start > /dev/null 2>&1 || fail "grow.sh start failed: $(tail -5 LOG.md)"
  local log
  log=$(cat "$STUB_LOG")
  assert_contains "$log" "agy --print You are running non-interactively"
  assert_contains "$log" 'codex exec --dangerously-bypass-approvals-and-sandbox --skip-git-repo-check --model model-b -c model_reasoning_effort="low"'
  assert_contains "$log" "opencode run --auto --model provider/model-c --variant high"
  [[ "$(grep -c -- '--model model-a' "$STUB_LOG")" -ge 2 ]] || fail "Visionary and Oracle should run on the smart tier"
}

test_start_fails_fast_when_a_backend_is_missing() {
  new_project
  rm "$TMP/bin/codex"
  grange agent check > /dev/null  # build the binary while go is still on PATH
  local output timeout_bin
  timeout_bin=$(command -v timeout)
  output=$(PATH="$TMP/bin:/usr/bin:/bin" GRANGE_CHEAP=codex "$timeout_bin" 20 ./grow.sh start 2>&1) && fail "should refuse to start"
  assert_contains "$output" "not found on PATH: codex (for Executor)"
  [[ ! -s "$STUB_LOG" ]] || fail "no agent should have run"
}

test_pair_mode_runs_the_executor_interactively() {
  new_project
  export STUB_BODY="$FAKE_AGENT"
  GROW_MODE=pair ENABLE_APPROVAL_GATE=false ENABLE_REVIEW_GATE=false \
    in_pty timeout 60 ./grow.sh start < /dev/null > /dev/null 2>&1
  # The stub logs "<cli> <args>"; a headless run would start "claude -p"
  assert_contains "$(cat "$STUB_LOG")" "claude --dangerously-skip-permissions You are pair programming"
  ! grep -q "^claude -p You are pair programming" "$STUB_LOG" || fail "pair mode ran the Executor headless"
}

test_digest_runs_on_the_digest_backend() {
  new_project
  echo "Blocked on credentials" > BLOCKERS.md
  export STUB_BODY="$(answer_with '# Human Digest')"
  GRANGE_AGENT_DIGEST=codex:model-d ./digest.sh > /dev/null 2>&1 || fail "digest.sh failed"
  assert_contains "$(cat "$STUB_LOG")" "codex exec"
  assert_contains "$(cat HUMAN_DIGEST.md)" "# Human Digest"
  assert_contains "$(cat .locks/digest_markers)" "BLOCKERS.md:1"
}
