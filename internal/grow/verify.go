package grow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ppvaz/grange/internal/workspace"
)

// VerdictFile is where the Oracle writes its verdict, relative to the project.
const VerdictFile = ".locks/oracle-verdict.json"

type verdict struct {
	Done               *bool    `json:"done"`
	Summary            string   `json:"summary"`
	BuildAndTests      string   `json:"build_and_tests"`
	Failures           []string `json:"failures"`
	UnaddressedReviews []string `json:"unaddressed_reviews"`
}

// verify asks the Oracle whether the vision is achieved, then either writes
// DONE.md or turns the failures into fix tasks. It reports progress.
func (g *Grow) verify(ctx context.Context) bool {
	path := g.ws.Path(VerdictFile)
	_ = os.Remove(path)
	cycles := g.ws.Counter("verify_cycles")
	err := g.Agent(ctx, Oracle, render("oracle", promptData{
		VerifyCycles: cycles,
		VerdictFile:  path,
		Minutes:      g.minutes(g.cfg.OracleTimeout),
	}), g.cfg.OracleTimeout, false)
	if err != nil {
		return false
	}

	v, err := readVerdict(path)
	if err != nil {
		g.log.Printf(workspace.Red, Oracle, "No usable verdict: %v", err)
		return false
	}
	if *v.Done {
		if err := g.writeDone(v); err != nil {
			g.log.Printf(workspace.Red, Oracle, "Writing DONE.md: %v", err)
			return false
		}
		return true
	}

	var tasks []string
	for _, f := range v.Failures {
		if f = strings.TrimSpace(f); f != "" && !strings.HasPrefix(f, "Fix:") {
			f = "Fix: " + f
		}
		tasks = append(tasks, f)
	}
	added, err := g.ws.AddTasks(tasks)
	if err != nil {
		g.log.Printf(workspace.Red, Oracle, "Adding fix tasks: %v", err)
		return false
	}
	g.ws.SetCounter("verify_cycles", cycles+1)
	g.log.Printf(workspace.Yellow, Oracle, "Not done yet: %s (%d fix tasks added)", v.Summary, added)
	return added > 0
}

func readVerdict(path string) (verdict, error) {
	var v verdict
	data, err := os.ReadFile(path)
	if err != nil {
		return v, errors.New("the Oracle didn't write " + VerdictFile)
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return v, fmt.Errorf("%s isn't valid JSON: %w", VerdictFile, err)
	}
	if v.Done == nil {
		return v, fmt.Errorf(`%s has no "done" field`, VerdictFile)
	}
	return v, nil
}

func (g *Grow) writeDone(v verdict) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# DONE\n\n%s\n\n## Completed Tasks\n\n", strings.TrimSpace(v.Summary))
	_, done := g.ws.Tasks()
	for _, t := range done {
		fmt.Fprintf(&b, "- [x] %s\n", t)
	}
	fmt.Fprintf(&b, "\n## Build & Tests\n\n%s\n\n## Unaddressed Vision Reviews\n\n", strings.TrimSpace(v.BuildAndTests))
	if len(v.UnaddressedReviews) == 0 {
		b.WriteString("No unresolved observations remain.\n")
	}
	for _, r := range v.UnaddressedReviews {
		fmt.Fprintf(&b, "- %s\n", r)
	}
	return os.WriteFile(g.ws.Path("DONE.md"), []byte(b.String()), 0o644)
}
