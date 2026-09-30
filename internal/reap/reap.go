// Package reap walks a work directory through the six IKE stages, extracting
// a target codebase's implicit knowledge, with a human checkpoint after each.
package reap

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ppvaz/grange/internal/config"
	"github.com/ppvaz/grange/internal/grow"
	"github.com/ppvaz/grange/internal/workspace"
)

var (
	// ErrInterrupted: the run was cancelled mid-stage; resume picks it up.
	ErrInterrupted = errors.New("interrupted")
	// ErrIncomplete: a stage finished without its required outputs.
	ErrIncomplete = errors.New("stage incomplete")
)

// Options can be passed on start/resume instead of answering prompts.
type Options struct {
	Stack    string // target stack for stage 2a
	BuildDir string // where stages 2a/2b build; default: the work dir
}

type Reap struct {
	dir     string
	visions string
	cfg     config.Settings
	st      State
	opts    Options
	in      *bufio.Reader
	out     io.Writer
	tty     bool
}

func newReap(dir, visions string, cfg config.Settings, opts Options) *Reap {
	return &Reap{dir: dir, visions: visions, cfg: cfg, opts: opts,
		in: bufio.NewReader(os.Stdin), out: os.Stdout, tty: workspace.IsTerminal(os.Stdin)}
}

// Start begins a pipeline in dir for the codebase at target.
func Start(ctx context.Context, dir, target, visions string, cfg config.Settings, opts Options) error {
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("target codebase not found: %s", target)
	}
	if target, err = filepath.Abs(target); err != nil {
		return err
	}
	r := newReap(dir, visions, cfg, opts)
	if st, err := loadState(dir); err == nil && st.CurrentStage >= 0 {
		r.printf("Existing pipeline state found (completed through Stage %s)", stages[st.CurrentStage].ID)
		if !r.tty {
			return fmt.Errorf("use `./reap.sh resume %s`, or `./reap.sh reset 0 %s` to start over", dir, dir)
		}
		if r.confirm(fmt.Sprintf("Continue from Stage %s?", stages[min(st.CurrentStage+1, len(stages)-1)].ID)) {
			return Resume(ctx, dir, visions, cfg, opts)
		}
		if !r.confirm("Start fresh? This will reset all progress.") {
			r.printf("Cancelled")
			return nil
		}
	}
	r.st = State{CurrentStage: -1, TargetPath: target, StartedAt: time.Now().Format(time.RFC3339)}
	if err := r.st.save(dir); err != nil {
		return err
	}
	r.printf("IKE v3 — Institutional Knowledge Extractor")
	r.printf("Target: %s", target)
	return r.runFrom(ctx, 0)
}

// Resume continues after the last completed stage.
func Resume(ctx context.Context, dir, visions string, cfg config.Settings, opts Options) error {
	r := newReap(dir, visions, cfg, opts)
	st, err := loadState(dir)
	if err != nil {
		return fmt.Errorf("no .ike-state in %s; start a pipeline with ./reap.sh start /path/to/target", dir)
	}
	r.st = st
	next := st.CurrentStage + 1
	if next >= len(stages) {
		r.printf("Pipeline already complete!")
		return nil
	}
	r.printf("Resuming from Stage %s — %s", stages[next].ID, stages[next].Name)
	return r.runFrom(ctx, next)
}

func (r *Reap) runFrom(ctx context.Context, first int) error {
	for idx := first; idx < len(stages); idx++ {
		force := false
		for {
			complete, err := r.runStage(ctx, idx, force)
			if ctx.Err() != nil {
				r.printf("Interrupted. Resume with: %s", r.resumeHint())
				return ErrInterrupted
			}
			if err != nil {
				return err
			}
			if !complete {
				r.printf("Rerun it with: %s", r.resumeHint())
				return ErrIncomplete
			}
			r.st.CurrentStage = idx
			if err := r.st.save(r.dir); err != nil {
				return err
			}
			switch r.checkpoint(idx) {
			case "rerun":
				force = true
				r.printf("Rerunning Stage %s...", stages[idx].ID)
				continue
			case "stop":
				return nil
			}
			break
		}
	}
	r.printf("IKE v3 Pipeline Complete!")
	return nil
}

// runStage runs one stage and reports whether its outputs are all there.
// Without force, a stage whose outputs already exist is skipped.
func (r *Reap) runStage(ctx context.Context, idx int, force bool) (bool, error) {
	s := stages[idx]
	r.printf("══════════════════════════════════════════════════════")
	r.printf("  Stage %s: %s  (%d/%d)", s.ID, s.Name, idx+1, len(stages))
	r.printf("══════════════════════════════════════════════════════")
	r.printf("%s", s.Description)

	if err := r.ensureBuildSetup(idx); err != nil {
		return false, err
	}
	dir := r.stageDir(idx)
	if !force && len(s.Artifacts(dir)) == 0 {
		r.printf("Stage %s outputs already exist — skipping", s.ID)
		return true, nil
	}
	if err := r.prepare(idx, dir); err != nil {
		return false, err
	}
	g, err := grow.New(dir, r.cfg)
	if err != nil {
		return false, err
	}
	g.AddDirs = []string{r.st.TargetPath}

	switch s.Kind {
	case lenses:
		err = r.runLenses(ctx, g, force)
	case oneShot:
		err = r.runOneShot(ctx, g, s)
	case growLoop:
		err = g.Run(ctx)
		if errors.Is(err, grow.ErrStalled) {
			err = nil // reported below as missing outputs
		}
		if err == nil && !g.Workspace().Done() {
			r.printf("Stage %s ended before the Oracle confirmed it (no DONE.md)", s.ID)
			return false, nil
		}
	}
	if err != nil || ctx.Err() != nil {
		return false, err
	}
	if missing := s.Artifacts(dir); len(missing) > 0 {
		r.printf("Stage %s is missing: %s", s.ID, strings.Join(missing, ", "))
		return false, nil
	}
	return true, nil
}

func (r *Reap) stageDir(idx int) string {
	if idx >= buildStage && r.st.BuildDir != "" {
		return r.st.BuildDir
	}
	return r.dir
}

// ensureBuildSetup collects the target stack and build dir before stage 2a.
func (r *Reap) ensureBuildSetup(idx int) error {
	if idx != buildStage {
		return nil
	}
	if r.opts.Stack != "" {
		r.st.TargetStack = r.opts.Stack
	}
	if r.opts.BuildDir != "" {
		r.st.BuildDir = r.opts.BuildDir
	}
	if r.st.TargetStack == "" {
		if !r.tty {
			return errors.New(`stage 2a needs a target stack: ./reap.sh resume --stack "Node.js + TypeScript + PostgreSQL" [--build-dir DIR]`)
		}
		r.st.TargetStack = r.ask("Target stack (e.g. 'Node.js + TypeScript + PostgreSQL'): ")
		if r.st.TargetStack == "" {
			return errors.New("stage 2a needs a target stack")
		}
		if r.st.BuildDir == "" {
			r.st.BuildDir = r.ask(fmt.Sprintf("Build directory [Enter for %s]: ", r.dir))
		}
	}
	if r.st.BuildDir == "" || r.st.BuildDir == r.dir {
		r.st.BuildDir = ""
		return r.st.save(r.dir)
	}
	abs, err := filepath.Abs(r.st.BuildDir)
	if err != nil {
		return err
	}
	r.st.BuildDir = abs
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(r.dir, "knowledge")); err == nil {
		r.printf("Copying knowledge/ to %s", abs)
		if out, err := exec.Command("cp", "-R", filepath.Join(r.dir, "knowledge"), abs+"/").CombinedOutput(); err != nil {
			return fmt.Errorf("copying knowledge/: %v: %s", err, out)
		}
	}
	return r.st.save(r.dir)
}

// prepare resets per-stage files and writes the stage's VISION.md.
func (r *Reap) prepare(idx int, dir string) error {
	for _, f := range []string{"PLAN.md", "BLOCKERS.md", "CUTS.md", "DRIFT.md"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o644); err != nil {
			return err
		}
	}
	for _, f := range []string{"DONE.md", "VISION_REVIEW.md", "LOG.md", ".locks"} {
		_ = os.RemoveAll(filepath.Join(dir, f))
	}

	vision, err := os.ReadFile(filepath.Join(r.visions, stages[idx].Vision))
	if err != nil {
		return fmt.Errorf("vision template: %w", err)
	}
	if idx == 2 {
		if generated, err := os.ReadFile(filepath.Join(r.dir, "recon/VISION-stage1-extraction.md")); err == nil {
			r.printf("Using the extraction spec generated in Stage 0b")
			vision = generated
		} else {
			r.printf("No recon/VISION-stage1-extraction.md from Stage 0b; using the template")
		}
	}
	text := strings.ReplaceAll(string(vision), "[TARGET_CODEBASE_PATH]", r.st.TargetPath)
	text = strings.ReplaceAll(text, "[DEFINE YOUR TARGET STACK]", r.st.TargetStack)
	return os.WriteFile(filepath.Join(dir, "VISION.md"), []byte(text), 0o644)
}

func (r *Reap) runLenses(ctx context.Context, g *grow.Grow, force bool) error {
	if err := g.Preflight("Lens"); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(r.dir, "recon"), 0o755); err != nil {
		return err
	}
	todo := lensJobs
	if !force {
		todo = r.missingLenses()
	}
	// One retry for lenses that didn't produce their file
	for attempt := 0; attempt < 2 && len(todo) > 0 && ctx.Err() == nil; attempt++ {
		var wg sync.WaitGroup
		for _, l := range todo {
			wg.Add(1)
			go func(l lens) {
				defer wg.Done()
				_ = g.RunJob(ctx, grow.Job{
					Name:    "Lens-" + strings.ReplaceAll(l.Name, " ", ""),
					Role:    "Lens",
					Prompt:  lensPrompt(l, r.st.TargetPath),
					Timeout: r.cfg.JobTimeout,
				})
			}(l)
		}
		wg.Wait()
		todo = r.missingLenses()
	}
	if len(todo) > 0 || ctx.Err() != nil {
		return nil // runStage reports what's missing
	}
	ws := g.Workspace()
	if _, err := ws.Git("add", "recon"); err != nil {
		return nil // not a git repo: nothing to commit
	}
	if out, err := ws.Git("commit", "--quiet", "--message", "recon: Stage 0a lens analyses"); err != nil && !strings.Contains(out, "nothing to commit") {
		r.printf("Couldn't commit the lens files (%v); they're still in recon/", err)
	}
	return nil
}

func (r *Reap) missingLenses() []lens {
	var missing []lens
	for _, l := range lensJobs {
		if len(missingFiles(r.dir, l.File)) > 0 {
			missing = append(missing, l)
		}
	}
	return missing
}

func lensPrompt(l lens, target string) string {
	return fmt.Sprintf(`You are the %[1]s lens of Stage 0a in an institutional knowledge extraction pipeline.

Read VISION.md in this directory and do ONLY task %[2]s, the %[1]s Lens. Two other agents are doing the other lenses in parallel.

The target codebase is at %[3]s. It is READ-ONLY: never create, modify or delete anything there.

Write your analysis to %[4]s, following the task's format. Don't create or change any other file, and don't commit: grange commits all three lenses together.
`, l.Name, l.Task, target, l.File)
}

func (r *Reap) runOneShot(ctx context.Context, g *grow.Grow, s stage) error {
	if err := g.Preflight("Synthesis"); err != nil {
		return err
	}
	var missing []string
	for attempt := 0; attempt < 2 && ctx.Err() == nil; attempt++ {
		prompt := fmt.Sprintf(`You are doing Stage %s (%s) of an institutional knowledge extraction pipeline, in one pass.

Read VISION.md in this directory and complete every task in it, producing every output it lists.

The target codebase is at %s. It is READ-ONLY: never create, modify or delete anything there.

When you're done, commit your outputs.
`, s.ID, s.Name, r.st.TargetPath)
		if len(missing) > 0 {
			prompt += "\nA previous attempt didn't produce: " + strings.Join(missing, ", ") + ". Make sure these exist.\n"
		}
		_ = g.RunJob(ctx, grow.Job{Name: "Synthesis", Role: "Synthesis", Prompt: prompt, Timeout: r.cfg.JobTimeout})
		if missing = s.Artifacts(r.dir); len(missing) == 0 {
			return nil
		}
	}
	return nil
}

// checkpoint shows the stage's outputs and what the human should review, and
// returns "next", "rerun" or "stop". Without a terminal it always stops.
func (r *Reap) checkpoint(idx int) string {
	dir := r.stageDir(idx)
	r.printf("[Artifacts] Key outputs from Stage %s:", stages[idx].ID)
	for _, a := range artifactSummary(idx, dir) {
		r.printf("  ✓ %s", a)
	}
	r.printf("CHECKPOINT — Human Review Required")
	r.printf("Your task: %s", stages[idx].Checkpoint)
	if idx+1 < len(stages) {
		r.printf("Next: Stage %s — %s", stages[idx+1].ID, stages[idx+1].Name)
	} else {
		r.printf("This was the final stage!")
		return "next"
	}
	if !r.tty {
		r.printf("When you're ready: %s", r.resumeHint())
		return "stop"
	}
	for {
		switch strings.ToLower(r.ask("  [Enter] continue  |  [r] rerun stage  |  [q] quit > ")) {
		case "":
			return "next"
		case "r":
			return "rerun"
		case "q":
			r.printf("Paused. Resume with: %s", r.resumeHint())
			return "stop"
		}
	}
}

func (r *Reap) resumeHint() string { return "./reap.sh resume " + r.dir }

func (r *Reap) printf(format string, args ...any) {
	fmt.Fprintf(r.out, "%s %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}

func (r *Reap) ask(prompt string) string {
	fmt.Fprint(r.out, prompt)
	line, _ := r.in.ReadString('\n')
	return strings.TrimSpace(line)
}

func (r *Reap) confirm(question string) bool {
	answer := strings.ToLower(r.ask("  " + question + " [y/N] "))
	return answer == "y" || answer == "yes"
}

// Status prints pipeline progress.
func Status(dir string, w io.Writer) error {
	st, err := loadState(dir)
	if err != nil {
		return fmt.Errorf("no .ike-state in %s", dir)
	}
	fmt.Fprintf(w, "IKE v3 Pipeline Status\nWork directory: %s\n", dir)
	if st.TargetPath != "" {
		fmt.Fprintf(w, "Target codebase: %s\n", st.TargetPath)
	}
	if st.StartedAt != "" {
		fmt.Fprintf(w, "Started: %s\n", st.StartedAt)
	}
	fmt.Fprintln(w)
	for i, s := range stages {
		mark := " "
		switch {
		case i <= st.CurrentStage:
			mark = "✓"
		case i == st.CurrentStage+1:
			mark = "▶"
		}
		fmt.Fprintf(w, "  [%s] %d  %-4s %s\n", mark, i, s.ID, s.Name)
	}
	fmt.Fprintln(w)
	if next := st.CurrentStage + 1; next < len(stages) {
		fmt.Fprintf(w, "Next: Stage %s — %s\nRun: ./reap.sh resume %s\n", stages[next].ID, stages[next].Name, dir)
	} else {
		fmt.Fprintln(w, "Pipeline complete!")
	}
	return nil
}

// Reset makes the next resume start at stage n.
func Reset(dir string, n int) error {
	if n < 0 || n >= len(stages) {
		return fmt.Errorf("stage must be 0-%d", len(stages)-1)
	}
	st, err := loadState(dir)
	if err != nil {
		return fmt.Errorf("no .ike-state in %s", dir)
	}
	st.CurrentStage = n - 1
	if err := st.save(dir); err != nil {
		return err
	}
	fmt.Printf("Pipeline will resume from Stage %s — %s\nRun: ./reap.sh resume %s\n", stages[n].ID, stages[n].Name, dir)
	return nil
}
