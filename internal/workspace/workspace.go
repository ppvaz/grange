// Package workspace is a grange project on disk: VISION.md, PLAN.md, the
// observation files, and the .locks/ state the dashboard reads.
package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Observation files agents append to; the Visionary and digest read them.
var ObservationFiles = []string{"BLOCKERS.md", "CUTS.md", "DRIFT.md", "VISION_REVIEW.md"}

type Workspace struct {
	Dir   string
	Locks string
	Log   *Logger
}

// Open prepares dir for a run: required files, .locks/, and no stale state
// left by a previous run that was killed.
func Open(dir string) (*Workspace, error) {
	w := &Workspace{Dir: dir, Locks: filepath.Join(dir, ".locks")}
	for _, d := range []string{w.Locks, filepath.Join(w.Locks, "checkpoints"), filepath.Join(dir, "specs")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	for _, name := range []string{"VISION.md", "PLAN.md", "LOG.md", "BLOCKERS.md", "CUTS.md", "DRIFT.md"} {
		f, err := os.OpenFile(w.Path(name), os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		f.Close()
	}
	log, err := NewLogger(w.Path("LOG.md"))
	if err != nil {
		return nil, err
	}
	w.Log = log
	w.clearStaleMarkers()
	w.removeLegacyHook()
	return w, nil
}

func (w *Workspace) Path(name string) string { return filepath.Join(w.Dir, name) }

func (w *Workspace) LockPath(name string) string { return filepath.Join(w.Locks, name) }

func (w *Workspace) Exists(name string) bool {
	_, err := os.Stat(w.Path(name))
	return err == nil
}

func (w *Workspace) Read(name string) string {
	data, _ := os.ReadFile(w.Path(name))
	return string(data)
}

// LineCount counts lines the way `wc -l` does.
func (w *Workspace) LineCount(name string) int {
	return strings.Count(w.Read(name), "\n")
}

func (w *Workspace) Done() bool { return w.Exists("DONE.md") }

var taskLine = regexp.MustCompile(`^\s*- \[([ xX])\] (.+)$`)

// Tasks returns PLAN.md's unchecked and checked tasks, in order.
func (w *Workspace) Tasks() (pending, done []string) {
	for _, line := range strings.Split(w.Read("PLAN.md"), "\n") {
		m := taskLine.FindStringSubmatch(line)
		switch {
		case m == nil:
		case m[1] == " ":
			pending = append(pending, m[2])
		default:
			done = append(done, m[2])
		}
	}
	return pending, done
}

// AddTasks appends tasks to PLAN.md, skipping ones already pending.
func (w *Workspace) AddTasks(tasks []string) (int, error) {
	pending, _ := w.Tasks()
	seen := map[string]bool{}
	for _, t := range pending {
		seen[t] = true
	}
	var b strings.Builder
	for _, t := range tasks {
		if t = strings.TrimSpace(t); t != "" && !seen[t] {
			seen[t] = true
			fmt.Fprintf(&b, "- [ ] %s\n", t)
		}
	}
	if b.Len() == 0 {
		return 0, nil
	}
	plan := w.Read("PLAN.md")
	if plan != "" && !strings.HasSuffix(plan, "\n") {
		plan += "\n"
	}
	return strings.Count(b.String(), "\n"), os.WriteFile(w.Path("PLAN.md"), []byte(plan+b.String()), 0o644)
}

// Counter reads an integer from .locks/<name> (0 when missing).
func (w *Workspace) Counter(name string) int {
	data, _ := os.ReadFile(w.LockPath(name))
	n, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return n
}

func (w *Workspace) SetCounter(name string, n int) {
	_ = os.WriteFile(w.LockPath(name), []byte(strconv.Itoa(n)+"\n"), 0o644)
}

func (w *Workspace) AddCounter(name string, delta int) int {
	n := w.Counter(name) + delta
	w.SetCounter(name, n)
	return n
}

// MarkRunning records that agent name is running, in the layout the
// dashboard reads (.locks/running_<name>/pid, <name>.last, agent_count).
// The returned func clears the marker.
func (w *Workspace) MarkRunning(name string) func() {
	marker := w.LockPath("running_" + name)
	_ = os.MkdirAll(marker, 0o755)
	_ = os.WriteFile(filepath.Join(marker, "pid"), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
	_ = os.WriteFile(w.LockPath(name+".last"), []byte(strconv.FormatInt(time.Now().Unix(), 10)+"\n"), 0o644)
	w.AddCounter("agent_count", 1)
	return func() {
		_ = os.RemoveAll(marker)
		if w.AddCounter("agent_count", -1) < 0 {
			w.SetCounter("agent_count", 0)
		}
	}
}

func (w *Workspace) clearStaleMarkers() {
	entries, _ := os.ReadDir(w.Locks)
	running := 0
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "running_") {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(w.Locks, e.Name(), "pid"))
		pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		if pid > 0 && syscall.Kill(pid, 0) == nil {
			running++
			continue
		}
		_ = os.RemoveAll(filepath.Join(w.Locks, e.Name()))
	}
	w.SetCounter("agent_count", running)
}

// legacyHook is the post-commit hook the bash grow.sh appended. The Go loop
// sees commits directly, and the hook's background ci.sh run would race the
// loop's own CI step.
const legacyHook = `
# GROW_HOOK
# Signal grow.sh about new commits
touch ".git-commit-signal" 2>/dev/null || true

# Run CI in background if available and not disabled
if [[ "${ENABLE_CI:-auto}" != "false" ]] && [[ -x "./ci.sh" ]]; then
  (
    echo "=== Post-commit CI at $(date) ===" >> .locks/ci-hook.log
    ./ci.sh >> .locks/ci-hook.log 2>&1 && echo "PASS ($(date))" >> .locks/ci-hook.log || echo "FAIL ($(date))" >> .locks/ci-hook.log
  ) &
fi
`

func (w *Workspace) removeLegacyHook() {
	gitDir, err := w.Git("rev-parse", "--git-dir")
	if err != nil {
		return
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(w.Dir, gitDir)
	}
	hook := filepath.Join(gitDir, "hooks", "post-commit")
	data, err := os.ReadFile(hook)
	if err != nil || !strings.Contains(string(data), legacyHook) {
		return
	}
	_ = os.WriteFile(hook, []byte(strings.Replace(string(data), legacyHook, "", 1)), 0o755)
	_ = os.Remove(w.Path(".git-commit-signal"))
}

// Git runs git in the workspace and returns trimmed stdout.
func (w *Workspace) Git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = w.Dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// Head is the current commit, or "" outside a repo / before the first commit.
func (w *Workspace) Head() string {
	head, _ := w.Git("rev-parse", "HEAD")
	return head
}
