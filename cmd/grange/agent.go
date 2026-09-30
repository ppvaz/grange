package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/ppvaz/grange/internal/agent"
	"github.com/ppvaz/grange/internal/config"
)

func agentCmd(ctx context.Context, workDir string, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usage)
		return exitError{2}
	}
	settings, err := config.Load()
	if err != nil {
		return err
	}
	switch args[0] {
	case "run":
		return agentRun(ctx, workDir, settings, args[1:])
	case "check":
		roles := args[1:]
		if len(roles) == 0 {
			roles = []string{"Executor", "Planner", "Gap", "Oracle", "Visionary", "Lens", "Digest", "Distill"}
		}
		return agentCheck(settings, roles)
	default:
		fmt.Fprintln(os.Stderr, usage)
		return exitError{2}
	}
}

// agentRun is how scripts (distill.sh) call a model without knowing which
// backend is configured.
func agentRun(ctx context.Context, workDir string, settings config.Settings, args []string) error {
	fs := flag.NewFlagSet("agent run", flag.ContinueOnError)
	role := fs.String("role", "Distill", "role whose backend to use (see GRANGE_AGENT_<ROLE>)")
	logPath := fs.String("log", "", "file for the agent's own output (default: discarded)")
	if err := fs.Parse(args); err != nil {
		return exitError{2}
	}
	prompt, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(prompt)) == "" {
		return fmt.Errorf("agent run: empty prompt on stdin")
	}

	log := io.Discard
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		log = f
	}

	answer, err := agent.Ask(ctx, agent.Run{
		Spec:    settings.SpecFor(*role),
		Inv:     agent.Invocation{Prompt: string(prompt)},
		Dir:     workDir,
		Timeout: settings.AgentTimeout,
		Log:     log,
	})
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString(answer)
	return err
}

func agentCheck(settings config.Settings, roles []string) error {
	missing := 0
	for _, role := range roles {
		spec := settings.SpecFor(role)
		status := "ok"
		if _, err := exec.LookPath(spec.Backend); err != nil {
			status = "NOT FOUND on PATH"
			missing++
		}
		fmt.Printf("%-10s %-40s %s\n", role, spec, status)
	}
	if missing > 0 {
		return exitError{1}
	}
	return nil
}
