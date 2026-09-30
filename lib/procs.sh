# lib/procs.sh - Stop everything a script started, and nothing else.
# Sourced by grow.sh and reap.sh for their Ctrl+C/SIGTERM cleanup.
#
# Killing our own process group isn't enough, and is too much: `timeout` puts
# each agent in a group of its own, out of reach; and when reap.sh runs
# grow.sh, both share a group, so grow's cleanup would SIGKILL reap.

# Prints every PID descended from PID $1, plus the process groups those
# descendants lead as negative PIDs, ready for `kill`. Take the snapshot before
# signalling anything: orphaned processes get reparented and stop looking ours.
descendants_of() {
  ps -A -o pid= -o ppid= -o pgid= | awk -v root="$1" '
    { parent[$1] = $2; group[$1] = $3 }
    END {
      for (pid in parent)
        for (p = parent[pid]; p > 1; p = parent[p])
          if (p == root) {
            print pid
            if (group[pid] == pid) print -pid
            break
          }
    }'
}

# TERM everything this shell started, then KILL whatever is left a second later
stop_descendants() {
  local targets
  targets=$(descendants_of $$)
  [[ -n "$targets" ]] || return 0
  # Some snapshot PIDs (the ps/awk above) are already gone; not an error
  kill -s TERM -- $targets 2>/dev/null || true
  sleep 1
  kill -s KILL -- $targets 2>/dev/null || true
  return 0
}
