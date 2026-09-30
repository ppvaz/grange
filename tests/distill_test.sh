# distill.sh: cross-project pattern discovery

test_distill_fails_fast_when_its_backend_is_missing() {
  new_project
  rm "$TMP/bin/opencode"
  grange agent check > /dev/null  # build the binary while go is still on PATH
  local output
  output=$(PATH="$TMP/bin:/usr/bin:/bin" GRANGE_CHEAP=opencode ./distill.sh 2>&1) && fail "distill.sh should have refused to run"
  assert_contains "$output" "Distill    opencode"
  assert_contains "$output" "NOT FOUND on PATH"
}

test_distill_clusters_through_the_configured_backend() {
  new_project
  mkdir -p specs/shop specs/blog
  printf '# Product listing\n\n## Business Requirement\nList products\n' > specs/shop/listing.md
  printf '# Post listing\n\n## Business Requirement\nList posts\n' > specs/blog/posts.md
  # Answer the clustering call with no clusters, which ends the run cleanly
  export STUB_BODY="$(answer_with '{"clusters": []}')"
  local output
  output=$(GRANGE_AGENT_DISTILL=codex:test-model WORK_DIR="$PROJECT" ./distill.sh --dry-run 2>&1) || fail "distill failed: $output"
  assert_contains "$(cat "$STUB_LOG")" "codex exec --dangerously-bypass-approvals-and-sandbox --skip-git-repo-check --model test-model"
  assert_contains "$output" "Cataloged 2 specs"
}
