// Command grange is the orchestrator behind grow.sh, reap.sh and digest.sh.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/ppvaz/grange/internal/agent"
	"github.com/ppvaz/grange/internal/config"
)

const usage = `Usage: grange <command> [args]

Commands:
  grow [start|executor|planner|gap|oracle|visionary|status]
                                      Drive the project toward VISION.md (default: start)
  digest                              Summarise new observations into HUMAN_DIGEST.md
  agent run [--role R] [--log FILE]   Run stdin as a prompt; print the agent's answer
  agent check [ROLE...]               Show which backend each role uses, and that it's installed`

// exitError carries a specific exit code up to main.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("exit %d", e.code) }

func main() {
	if err := run(os.Args[1:]); err != nil {
		var exit exitError
		if errors.As(err, &exit) {
			os.Exit(exit.code)
		}
		fmt.Fprintln(os.Stderr, "grange:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usage)
		return exitError{2}
	}
	workDir, err := workDir()
	if err != nil {
		return err
	}
	if err := config.LoadDotEnv(workDir, grangeHome()); err != nil {
		return err
	}
	ctx, stop := shutdownContext()
	defer stop()

	switch args[0] {
	case "grow":
		return growCmd(ctx, workDir, args[1:])
	case "digest":
		return digestCmd(ctx, workDir)
	case "agent":
		return agentCmd(ctx, workDir, args[1:])
	default:
		fmt.Fprintln(os.Stderr, usage)
		return exitError{2}
	}
}

func workDir() (string, error) {
	if dir := os.Getenv("WORK_DIR"); dir != "" {
		return filepath.Abs(dir)
	}
	return os.Getwd()
}

// grangeHome is the grange checkout; the binary lives in its bin/.
func grangeHome() string {
	if home := os.Getenv("GRANGE_HOME"); home != "" {
		return home
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(filepath.Dir(exe))
}

// shutdownContext is cancelled on SIGTERM, and on SIGINT unless an
// interactive agent owns the terminal (its UI uses Ctrl+C to interrupt).
func shutdownContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range signals {
			if sig == syscall.SIGINT && agent.InteractiveActive() {
				continue
			}
			cancel()
		}
	}()
	return ctx, func() {
		signal.Stop(signals)
		cancel()
	}
}
