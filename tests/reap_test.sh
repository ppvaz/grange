# reap: six checkpointed IKE stages over a read-only target codebase

new_target() {
  TARGET="$TMP/target"
  mkdir -p "$TARGET"
  echo "def checkout(cart): return sum(cart)" > "$TARGET/shop.py"
}

test_stage_0a_runs_three_lenses_then_stops_for_review() {
  new_project
  new_target
  export STUB_BODY="$FAKE_AGENT"
  local output
  output=$(./reap.sh start "$TARGET" < /dev/null 2>&1) || fail "reap start failed: $output"
  local f
  for f in business-analyst product-manager qa-adversarial; do
    [[ -s "recon/lens-$f.md" ]] || fail "missing recon/lens-$f.md"
  done
  [[ "$(grep -c 'lens of Stage 0a' "$STUB_LOG")" == 3 ]] || fail "expected three lens runs"
  assert_contains "$(cat "$STUB_LOG")" "--add-dir $TARGET"
  assert_contains "$(git log --format=%s)" "recon: Stage 0a lens analyses"
  assert_contains "$(cat .ike-state)" "CURRENT_STAGE=0"
  assert_contains "$output" "Your task: Review lens files"
  assert_contains "$output" "When you're ready: ./reap.sh resume $PROJECT"
}

test_resume_runs_synthesis_next() {
  new_project
  new_target
  export STUB_BODY="$FAKE_AGENT"
  ./reap.sh start "$TARGET" < /dev/null > /dev/null 2>&1 || fail "reap start failed"
  local output
  output=$(./reap.sh resume < /dev/null 2>&1) || fail "reap resume failed: $output"
  [[ -s recon/VISION-stage1-extraction.md ]] || fail "0b should generate the extraction vision"
  assert_contains "$(cat .ike-state)" "CURRENT_STAGE=1"
  assert_contains "$(./reap.sh status)" "[▶] 2  1a   Deep Extraction"
}

test_stage_without_its_outputs_is_not_marked_done() {
  new_project
  new_target
  local output
  output=$(./reap.sh start "$TARGET" < /dev/null 2>&1) && fail "stage 0a without lens files should fail"
  assert_contains "$output" "Stage 0a is missing: recon/lens-business-analyst.md"
  assert_contains "$(cat .ike-state)" "CURRENT_STAGE=-1"
}

test_grow_stage_needs_its_artifacts_not_just_done_md() {
  new_project
  new_target
  export STUB_BODY="$FAKE_AGENT"
  ./reap.sh start "$TARGET" < /dev/null > /dev/null 2>&1 || fail "reap start failed"
  ./reap.sh resume < /dev/null > /dev/null 2>&1 || fail "0b failed"
  local output
  # 1a's fake loop reaches DONE.md without writing knowledge/
  output=$(./reap.sh resume < /dev/null 2>&1) && fail "1a without knowledge/ should fail"
  [[ -f DONE.md ]] || fail "the fake Oracle should have declared done"
  assert_contains "$output" "Stage 1a is missing: knowledge/entities/"
  assert_contains "$(cat .ike-state)" "CURRENT_STAGE=1"
}

test_build_stage_asks_for_a_stack_without_a_terminal() {
  new_project
  new_target
  export STUB_BODY="$FAKE_AGENT"
  ./reap.sh start "$TARGET" < /dev/null > /dev/null 2>&1 || fail "reap start failed"
  ./reap.sh reset 4 > /dev/null
  local output
  output=$(./reap.sh resume < /dev/null 2>&1) && fail "2a without a stack should fail"
  assert_contains "$output" 'needs a target stack: ./reap.sh resume --stack'
}
