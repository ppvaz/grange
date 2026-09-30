# Ctrl+C must stop running agents, not just the scripts that launched them

# Runs a command the way a terminal runs a foreground job: leading its own
# process group, with SIGINT restored (bash starts async commands with it
# ignored, and an ignored signal can't be trapped).
start_as_foreground_job() {
  perl -e '$SIG{INT} = "DEFAULT"; setpgrp(0, 0); exec @ARGV or die' "$@" > "$TMP/job.log" 2>&1 &
  JOB=$!
}

# What the terminal does on Ctrl+C: SIGINT to the whole foreground group
ctrl_c() {
  kill -INT -"$JOB"
  perl -e 'sleep 20; kill "KILL", -$ARGV[0]' "$JOB" &
  local watchdog=$!
  wait "$JOB"
  kill "$watchdog" 2>/dev/null
  wait "$watchdog" 2>/dev/null
}

agent_started() {
  [[ -s "$STUB_PIDS" ]]
}

agents_stopped() {
  local pid
  for pid in $(cat "$STUB_PIDS"); do
    kill -0 "$pid" 2>/dev/null && return 1
  done
  return 0
}

test_ctrl_c_on_grow_stops_running_agents() {
  new_project
  export STUB_BODY='exec sleep 300'
  start_as_foreground_job ./grow.sh start
  wait_for 30 agent_started
  ctrl_c
  wait_for 5 agents_stopped
}

test_ctrl_c_on_reap_stops_running_agents() {
  new_project
  export STUB_BODY='exec sleep 300'
  start_as_foreground_job ./reap.sh start "$PROJECT"
  wait_for 30 agent_started
  ctrl_c
  wait_for 5 agents_stopped
  # reap.sh got to finish its own cleanup rather than being killed by grow.sh's
  assert_contains "$(cat "$TMP/job.log")" "Resume with: ./reap.sh resume"
}
