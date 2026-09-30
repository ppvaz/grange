// Package grow drives a project toward its VISION.md with a fixed loop:
// plan, execute, CI, review, and verify, until the Oracle confirms it's done.
package grow

import (
	"bufio"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ppvaz/grange/internal/agent"
	"github.com/ppvaz/grange/internal/config"
	"github.com/ppvaz/grange/internal/workspace"
)

// Roles, named as the dashboard expects in .locks/running_<role>.
const (
	Executor  = "Executor"
	Planner   = "Planner"
	Gap       = "Gap"
	Oracle    = "Oracle"
	Visionary = "Visionary"
	Digest    = "Digest"
)

// maxStalls is how many loop turns in a row may make no progress before
// grow stops instead of spending more tokens going in circles.
const maxStalls = 3

// ErrStalled means the loop stopped because nothing was moving.
var ErrStalled = errors.New("no progress in 3 consecutive turns; see BLOCKERS.md and LOG.md")

// errPaused ends the loop quietly: the human chose not to continue.
var errPaused = errors.New("paused")

type Grow struct {
	ws  *workspace.Workspace
	cfg config.Settings
	log *workspace.Logger
	// AddDirs are extra directories agents may read (reap's target codebase).
	AddDirs []string

	lastLines map[string]int // observation file sizes at the last check
}

func New(dir string, cfg config.Settings) (*Grow, error) {
	ws, err := workspace.Open(dir)
	if err != nil {
		return nil, err
	}
	return &Grow{ws: ws, cfg: cfg, log: ws.Log}, nil
}

func (g *Grow) Workspace() *workspace.Workspace { return g.ws }

// Preflight fails fast when a backend needed by these roles isn't installed.
func (g *Grow) Preflight(roles ...string) error {
	var missing []string
	for _, role := range roles {
		spec := g.cfg.SpecFor(role)
		if _, err := exec.LookPath(spec.Backend); err != nil {
			missing = append(missing, fmt.Sprintf("%s (for %s)", spec.Backend, role))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("not found on PATH: %s; install it or change GRANGE_SMART / GRANGE_CHEAP / GRANGE_AGENT_<ROLE>", strings.Join(missing, ", "))
	}
	return nil
}

// Run loops until the vision is done, ctx is cancelled, the human pauses, or
// the loop stalls.
func (g *Grow) Run(ctx context.Context) error {
	if g.ws.Done() {
		g.log.Printf(workspace.Green, "Main", "DONE.md already exists. Vision was achieved!")
		return nil
	}
	if err := g.Preflight(Executor, Planner, Gap, Oracle, Visionary, Digest); err != nil {
		return err
	}
	g.log.Printf(workspace.Green, "Main", "grange grow | mode %s | CI %s | approval gate %v | review gate %v | TDD %v",
		g.cfg.Mode, g.cfg.CI, g.cfg.ApprovalGate, g.cfg.ReviewGate, g.cfg.TDD)

	if g.cfg.AlwaysVisionary || !g.visionValidated() {
		g.log.Printf(workspace.Blue, "Main", "Validating vision...")
		g.visionary(ctx, "STARTUP CHECK: No work has begun yet. Focus on whether VISION.md is specific, measurable, and actionable. Flag any issues that would cause agents to struggle.")
		g.saveVisionHash()
	} else {
		g.log.Printf(workspace.Green, "Main", "Vision unchanged since last validation, skipping startup Visionary")
	}
	g.lastLines = g.lineCounts()

	stalls := 0
	for ctx.Err() == nil && !g.ws.Done() {
		progressed, err := g.turn(ctx)
		if errors.Is(err, errPaused) {
			g.log.Printf(workspace.Blue, "Main", "Paused. Run ./grow.sh start to continue.")
			return nil
		}
		if err != nil {
			return err
		}
		if ctx.Err() != nil || g.ws.Done() {
			break
		}
		if progressed {
			stalls = 0
		} else if stalls++; stalls >= maxStalls {
			g.log.Printf(workspace.Red, "Main", "Stopping: %v", ErrStalled)
			return ErrStalled
		}
		g.observe(ctx)
	}
	if g.ws.Done() {
		g.log.Printf(workspace.Green, "Main", "Vision achieved. See DONE.md.")
	}
	return ctx.Err()
}

// turn does one step of work and reports whether anything moved.
func (g *Grow) turn(ctx context.Context) (bool, error) {
	pending, done := g.ws.Tasks()
	switch {
	case len(pending) == 0 && len(done) == 0:
		g.log.Printf(workspace.Blue, "Main", "Empty plan, populating tasks from vision...")
		g.planner(ctx)
		pending, _ = g.ws.Tasks()
		return len(pending) > 0, nil
	case len(pending) == 0:
		g.log.Printf(workspace.Blue, "Main", "All tasks complete, invoking Oracle...")
		return g.verify(ctx), nil
	}

	if !g.waitForGates(ctx) {
		return false, nil
	}
	before := g.ws.Head()
	interactive := g.executor(ctx)
	g.runCI()
	after := g.ws.Head()
	stillPending, _ := g.ws.Tasks()

	progressed := after != before || len(stillPending) < len(pending)
	if after != before && ctx.Err() == nil {
		g.afterCommits(ctx, before, after)
	}
	if !progressed && ctx.Err() == nil {
		g.log.Printf(workspace.Yellow, "Main", "Executor made no progress; asking the Planner to re-check the plan")
		g.planner(ctx)
	}
	if interactive && ctx.Err() == nil && !g.askContinue() {
		return progressed, errPaused
	}
	return progressed, nil
}

// afterCommits reviews new work and keeps the plan topped up.
func (g *Grow) afterCommits(ctx context.Context, before, after string) {
	n := 1
	if before != "" {
		if out, err := g.ws.Git("rev-list", "--count", before+".."+after); err == nil {
			n, _ = strconv.Atoi(out)
		}
	}
	total := g.ws.AddCounter("commit_count", n)
	g.gap(ctx, before, after)

	if pending, _ := g.ws.Tasks(); len(pending) < g.cfg.MaxPendingTasks {
		g.planner(ctx)
	}
	// Every REFACTOR_INTERVAL commits, nudge the Visionary to look for rot
	if g.cfg.RefactorInterval > 0 && total/g.cfg.RefactorInterval > (total-n)/g.cfg.RefactorInterval {
		g.log.Printf(workspace.Blue, "Main", "%d commits so far, signalling Visionary for a refactoring check", total)
		g.ws.AddCounter("signal_count", 1)
	}
}

// Job is one agent run. Name labels its log and running marker; Role picks
// the backend. They differ when one role runs several jobs (reap's lenses).
type Job struct {
	Name, Role, Prompt string
	Timeout            time.Duration
	Interactive        bool
}

// Agent runs one role and records the run the way the dashboard reads it.
func (g *Grow) Agent(ctx context.Context, role, prompt string, timeout time.Duration, interactive bool) error {
	return g.RunJob(ctx, Job{Name: role, Role: role, Prompt: prompt, Timeout: timeout, Interactive: interactive})
}

func (g *Grow) RunJob(ctx context.Context, j Job) error {
	role, prompt, timeout, interactive := j.Name, j.Prompt, j.Timeout, j.Interactive
	spec := g.cfg.SpecFor(j.Role)
	defer g.ws.MarkRunning(role)()

	logFile, err := g.agentLog(role)
	if err != nil {
		return err
	}
	defer logFile.Close()
	fmt.Fprintf(logFile, "=== Run started at %s ===\n", time.Now().Format(time.UnixDate))

	preamble := render("preamble", promptData{Pair: interactive, PlanFirst: interactive && g.cfg.PairStyle == "plan"})
	if interactive {
		g.log.Printf(workspace.Blue, role, "Running %s interactively (human in the loop)...", spec)
	} else {
		g.log.Printf(workspace.Blue, role, "Running %s (timeout %s)...", spec, timeout)
	}

	err = agent.Exec(ctx, agent.Run{
		Spec: spec,
		Inv: agent.Invocation{
			Prompt:      preamble + prompt,
			AddDirs:     g.AddDirs,
			Interactive: interactive,
			PlanMode:    interactive && g.cfg.PairStyle == "plan",
		},
		Dir:     g.ws.Dir,
		Timeout: timeout,
		Log:     g.log.Output(logFile),
	})
	switch {
	case err == nil:
		g.log.Printf(workspace.Green, role, "Completed")
		g.ws.SetCounter("timeout_count", 0)
	case errors.Is(err, agent.ErrTimeout):
		g.log.Printf(workspace.Red, role, "TIMEOUT after %s", timeout)
		g.ws.AddCounter("timeout_count", 1)
	case ctx.Err() != nil:
		g.log.Printf(workspace.Yellow, role, "Stopped")
	default:
		g.log.Printf(workspace.Red, role, "Failed: %v", err)
	}
	return err
}

// agentLog opens .locks/<role>.log for appending, rotating it past 1MB.
func (g *Grow) agentLog(role string) (*os.File, error) {
	path := g.ws.LockPath(role + ".log")
	if info, err := os.Stat(path); err == nil && info.Size() > 1<<20 {
		_ = os.Rename(path, path+".old")
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}

func (g *Grow) minutes(d time.Duration) int { return int(d.Minutes()) }

// executor runs the next task and reports whether it ran interactively.
func (g *Grow) executor(ctx context.Context) bool {
	interactive := g.cfg.Mode == "pair" && workspace.IsTerminal(os.Stdin)
	if g.cfg.Mode == "pair" && !interactive {
		g.log.Printf(workspace.Yellow, Executor, "Pair mode without a terminal; running headless")
	}
	timeout := g.cfg.AgentTimeout
	if interactive {
		timeout = g.cfg.PairTimeout
	}
	checkpointFile := g.ws.LockPath("checkpoints/executor.md")
	checkpoint, _ := os.ReadFile(checkpointFile)
	_ = g.Agent(ctx, Executor, render("executor", promptData{
		TDD:            g.cfg.TDD,
		CI:             g.cfg.CI != "false" && g.ciScript() != "",
		Checkpoint:     strings.TrimSpace(string(checkpoint)),
		CheckpointFile: checkpointFile,
		Minutes:        g.minutes(timeout),
	}), timeout, interactive)
	return interactive
}

func (g *Grow) planner(ctx context.Context) {
	pending, done := g.ws.Tasks()
	populate := len(pending) == 0 && len(done) == 0
	if !populate && len(pending) >= g.cfg.MaxPendingTasks {
		g.log.Printf(workspace.Yellow, Planner, "Skipped: %d pending tasks (max %d)", len(pending), g.cfg.MaxPendingTasks)
		return
	}
	entries, _ := os.ReadDir(g.ws.Path("specs"))
	_ = g.Agent(ctx, Planner, render("planner", promptData{Populate: populate, Specs: len(entries) > 0}), g.cfg.AgentTimeout, false)
}

// gap reviews the commits in (before, after].
func (g *Grow) gap(ctx context.Context, before, after string) {
	rng, diff := after+" -1", after+"~1 "+after
	if before != "" {
		rng, diff = before+".."+after, before+" "+after
	}
	_ = g.Agent(ctx, Gap, render("gap", promptData{
		Range:        rng,
		DiffRange:    diff,
		Minutes:      g.minutes(g.cfg.AgentTimeout),
		MaxFileLines: g.cfg.MaxFileLines,
		Date:         time.Now().Format("2006-01-02"),
	}), g.cfg.AgentTimeout, false)
}

func (g *Grow) visionary(ctx context.Context, trigger string) {
	_ = g.Agent(ctx, Visionary, render("visionary", promptData{
		Trigger: trigger,
		History: g.previousObservations(),
		Date:    time.Now().Format("2006-01-02"),
	}), g.cfg.AgentTimeout, false)
	g.checkReviewGate()
}

// previousObservations is the tail of VISION_REVIEW.md's observations, so the
// Visionary doesn't repeat itself.
func (g *Grow) previousObservations() string {
	lines := strings.Split(g.ws.Read("VISION_REVIEW.md"), "\n")
	var picked []string
	for i, line := range lines {
		if strings.HasPrefix(line, "## Observation") {
			end := min(i+4, len(lines))
			picked = append(picked, lines[i:end]...)
		}
	}
	if len(picked) > 20 {
		picked = picked[len(picked)-20:]
	}
	return strings.Join(picked, "\n")
}

// observe reacts to what agents wrote down: enough new blockers, cuts and
// drift wake the Visionary, and a backlog of observations triggers a digest.
func (g *Grow) observe(ctx context.Context) {
	now := g.lineCounts()
	growth := 0
	for _, f := range []string{"BLOCKERS.md", "CUTS.md", "DRIFT.md"} {
		growth += now[f] - g.lastLines[f]
	}
	g.lastLines = now
	if growth > 0 {
		total := g.ws.AddCounter("signal_count", growth)
		g.log.Printf(workspace.Yellow, "Main", "Observation files grew by %d lines (signals: %d)", growth, total)
	}
	if g.ws.Counter("signal_count") >= 3 {
		g.ws.SetCounter("signal_count", 0)
		g.log.Printf(workspace.Blue, "Main", "Signal threshold reached, running Visionary")
		g.visionary(ctx, "")
	}
	if g.ws.Counter("timeout_count") >= 3 {
		g.ws.SetCounter("timeout_count", 0)
		g.visionary(ctx, "TIMEOUT ALERT: Multiple agents timed out in a row. Consider if the vision is too vague or ambitious, or if tasks are too large to complete in the allotted time.")
	}
	if backlog := g.digestBacklog(); backlog >= 10 {
		g.log.Printf(workspace.Blue, "Main", "%d new observation lines, compiling a digest", backlog)
		if err := g.RunDigest(ctx); err != nil {
			g.log.Printf(workspace.Red, Digest, "%v", err)
		}
	}
}

func (g *Grow) lineCounts() map[string]int {
	counts := map[string]int{}
	for _, f := range workspace.ObservationFiles {
		counts[f] = g.ws.LineCount(f)
	}
	return counts
}

// checkReviewGate raises .vision-review-pending when the Visionary added to
// VISION_REVIEW.md and the review gate is on.
func (g *Grow) checkReviewGate() {
	now := g.ws.LineCount("VISION_REVIEW.md")
	if g.lastLines != nil && now > g.lastLines["VISION_REVIEW.md"] && g.cfg.ReviewGate {
		_ = os.WriteFile(g.ws.Path(".vision-review-pending"), nil, 0o644)
		g.log.Printf(workspace.Yellow, "ReviewGate", "VISION_REVIEW.md grew; Executor paused until review (touch .vision-reviewed)")
	}
	if g.lastLines != nil {
		g.lastLines["VISION_REVIEW.md"] = now
	}
}

// waitForGates blocks until the approval and review gates let the Executor
// run. It returns false if ctx is cancelled or DONE.md appears first.
func (g *Grow) waitForGates(ctx context.Context) bool {
	waiting := ""
	for {
		reason := g.gatesOpen()
		if reason == "" {
			return true
		}
		if reason != waiting {
			g.log.Printf(workspace.Yellow, "Gate", "%s", reason)
			waiting = reason
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(2 * time.Second):
		}
		if g.ws.Done() {
			return false
		}
	}
}

// gatesOpen consumes gate signals and returns "" when the Executor may run,
// or what it's waiting for.
func (g *Grow) gatesOpen() string {
	if g.cfg.ReviewGate && g.ws.Exists(".vision-review-pending") {
		if !g.ws.Exists(".vision-reviewed") {
			return "Waiting for vision review (read VISION_REVIEW.md, then touch .vision-reviewed)"
		}
		_ = os.Remove(g.ws.Path(".vision-review-pending"))
		_ = os.Remove(g.ws.Path(".vision-reviewed"))
		g.log.Printf(workspace.Green, "Gate", "Vision review acknowledged")
	}
	if g.cfg.ApprovalGate {
		if !g.ws.Exists(".plan-approved") {
			return "Waiting for approval (review PLAN.md, then touch .plan-approved)"
		}
		_ = os.Remove(g.ws.Path(".plan-approved"))
		g.log.Printf(workspace.Green, "Gate", "Plan approved, Executor may proceed")
	}
	return ""
}

// askContinue is pair mode's pause point between tasks.
func (g *Grow) askContinue() bool {
	pending, _ := g.ws.Tasks()
	if len(pending) == 0 {
		return true
	}
	fmt.Printf("\n%d tasks remaining:\n", len(pending))
	for i, t := range pending {
		if i == 5 {
			fmt.Printf("  ... and %d more\n", len(pending)-5)
			break
		}
		fmt.Printf("  - [ ] %s\n", t)
	}
	if g.ws.Exists(".vision-review-pending") {
		fmt.Println("\nThe Visionary raised concerns; review VISION_REVIEW.md")
	}
	fmt.Print("\nContinue to next task? [Y/n] ")
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "" || answer == "y" || answer == "yes"
}

// visionValidated reports whether VISION.md is unchanged since the Visionary
// last checked it, within the past hour.
func (g *Grow) visionValidated() bool {
	stored, err := os.ReadFile(g.ws.LockPath("vision_validated_hash"))
	if err != nil || strings.TrimSpace(string(stored)) != g.visionHash() {
		return false
	}
	last := g.ws.Counter("Visionary.last")
	return last > 0 && time.Since(time.Unix(int64(last), 0)) < time.Hour
}

func (g *Grow) saveVisionHash() {
	_ = os.WriteFile(g.ws.LockPath("vision_validated_hash"), []byte(g.visionHash()+"\n"), 0o644)
}

func (g *Grow) visionHash() string {
	return fmt.Sprintf("%x", md5.Sum([]byte(g.ws.Read("VISION.md"))))
}

// RunRole runs one agent once, as `grow.sh executor` and the dashboard's
// "run agent" do. Gates don't apply: a human asked for this run.
func (g *Grow) RunRole(ctx context.Context, role string) error {
	roles := map[string]string{"executor": Executor, "planner": Planner, "gap": Gap, "oracle": Oracle, "visionary": Visionary}
	name, ok := roles[role]
	if !ok {
		return fmt.Errorf("unknown agent %q", role)
	}
	if err := g.Preflight(name); err != nil {
		return err
	}
	switch name {
	case Executor:
		g.executor(ctx)
		g.runCI()
	case Planner:
		g.planner(ctx)
	case Gap:
		parent, _ := g.ws.Git("rev-parse", "HEAD~1")
		g.gap(ctx, parent, g.ws.Head())
	case Oracle:
		g.verify(ctx)
	case Visionary:
		g.lastLines = g.lineCounts()
		g.visionary(ctx, "")
	}
	return ctx.Err()
}
