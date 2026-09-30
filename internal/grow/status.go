package grow

import (
	"fmt"
	"io"

	"strings"
)

// Status prints the vision, plan, CI, gates and recent log.
func (g *Grow) Status(w io.Writer) {
	vision := strings.Split(g.ws.Read("VISION.md"), "\n")
	fmt.Fprintln(w, "=== VISION ===")
	fmt.Fprintln(w, strings.Join(vision[:min(20, len(vision))], "\n"))

	pending, done := g.ws.Tasks()
	fmt.Fprintln(w, "\n=== PLAN (pending) ===")
	printTasks(w, " ", pending)
	fmt.Fprintln(w, "\n=== PLAN (completed) ===")
	printTasks(w, "x", done[max(0, len(done)-10):])

	fmt.Fprintln(w, "\n=== CI STATUS ===")
	fmt.Fprintln(w, orNone(tail(g.ws.Read(".locks/ci.log"), 3), "(no CI runs yet)"))

	fmt.Fprintln(w, "\n=== CONFIG ===")
	fmt.Fprintf(w, "Mode: %s | CI: %s | Approval gate: %v | Review gate: %v | TDD: %v\n",
		g.cfg.Mode, g.cfg.CI, g.cfg.ApprovalGate, g.cfg.ReviewGate, g.cfg.TDD)
	for _, role := range []string{Executor, Planner, Gap, Oracle, Visionary, Digest} {
		fmt.Fprintf(w, "%-10s %s\n", role, g.cfg.SpecFor(role))
	}

	fmt.Fprintln(w, "\n=== GATES ===")
	switch {
	case !g.cfg.ApprovalGate:
		fmt.Fprintln(w, "Approval: disabled")
	case g.ws.Exists(".plan-approved"):
		fmt.Fprintln(w, "Approval: APPROVED (pending consumption)")
	default:
		fmt.Fprintln(w, "Approval: WAITING (touch .plan-approved to approve)")
	}
	switch {
	case !g.cfg.ReviewGate:
		fmt.Fprintln(w, "Review: disabled")
	case !g.ws.Exists(".vision-review-pending"):
		fmt.Fprintln(w, "Review: CLEAR (no pending review)")
	case g.ws.Exists(".vision-reviewed"):
		fmt.Fprintln(w, "Review: ACKNOWLEDGED (pending consumption)")
	default:
		fmt.Fprintln(w, "Review: PENDING (touch .vision-reviewed to acknowledge)")
	}

	fmt.Fprintln(w, "\n=== RECENT LOG ===")
	fmt.Fprintln(w, orNone(tail(g.ws.Read("LOG.md"), 20), "(empty)"))
}

func printTasks(w io.Writer, mark string, tasks []string) {
	if len(tasks) == 0 {
		fmt.Fprintln(w, "(none)")
	}
	for _, t := range tasks {
		fmt.Fprintf(w, "- [%s] %s\n", mark, t)
	}
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return strings.Join(lines[max(0, len(lines)-n):], "\n")
}

func orNone(s, none string) string {
	if strings.TrimSpace(s) == "" {
		return none
	}
	return s
}
