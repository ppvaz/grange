package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
// backend is configured: the answer goes through a file, which every backend
// can write, so no CLI's output format has to be parsed.
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

	tmp, err := os.MkdirTemp("", "grange-answer-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	answer := filepath.Join(tmp, "answer.md")

	log := io.Discard
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		log = f
	}

	spec := settings.SpecFor(*role)
	err = agent.Exec(ctx, agent.Run{
		Spec:    spec,
		Inv:     agent.Invocation{Prompt: string(prompt) + answerInstruction(answer)},
		Dir:     workDir,
		Timeout: settings.AgentTimeout,
		Log:     log,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", spec, err)
	}
	out, err := os.ReadFile(answer)
	if err != nil {
		return fmt.Errorf("%s finished without writing an answer to %s", spec, answer)
	}
	_, err = os.Stdout.Write(out)
	return err
}

func answerInstruction(path string) string {
	return fmt.Sprintf("\n\nWrite your final answer, and nothing else, to the file %s. Don't create or modify any other file.\n", path)
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
