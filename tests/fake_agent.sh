# tests/fake_agent.sh - Sourced by the stub agent CLI (STUB_BODY=$FAKE_AGENT):
# plays each grange role just well enough to drive grow's loop. "$*" is the
# CLI's argv, prompt included. ORACLE_FAILS_ONCE makes the first verdict fail.
prompt="$*"
case "$prompt" in
  *"You are the Planner agent"*"PLAN.md is empty"*)
    printf -- '- [ ] Write hello.txt\n- [ ] Write bye.txt\n' > PLAN.md
    ;;
  *"You are the Executor agent"*)
    task=$(grep -m1 -- '- \[ \]' PLAN.md | sed 's/^- \[ \] //')
    [[ -n "$task" ]] || exit 0
    echo "$task" >> work.txt
    perl -pi -e 's/- \[ \]/- [x]/ && $done++ unless $done' PLAN.md
    git add -A
    git commit --quiet --message "Do: $task"
    ;;
  *"You are the Oracle agent"*)
    verdict=$(printf '%s' "$prompt" | sed -E -e 's/.*write it as JSON to ([^,]+), with exactly.*/\1/' -e t -e d)
    if [[ -n "${ORACLE_FAILS_ONCE:-}" && ! -f .locks/oracle-failed ]]; then
      touch .locks/oracle-failed
      echo '{"done": false, "summary": "Tests fail.", "failures": ["make the tests pass"]}' > "$verdict"
    else
      echo '{"done": true, "summary": "All shipped.", "build_and_tests": "stub tests pass", "failures": [], "unaddressed_reviews": []}' > "$verdict"
    fi
    ;;
esac
