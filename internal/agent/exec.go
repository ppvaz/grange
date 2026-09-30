package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
)

// ErrTimeout is returned by Exec when the agent outlived its timeout.
var ErrTimeout = errors.New("agent timed out")

// gracePeriod is how long an agent gets to exit after SIGTERM before SIGKILL.
var gracePeriod = 10 * time.Second

var interactiveRuns atomic.Int32

// InteractiveActive reports whether an agent currently owns the terminal.
// Ctrl+C then belongs to the agent's UI, not to grange.
func InteractiveActive() bool { return interactiveRuns.Load() > 0 }

// Run describes one agent process.
type Run struct {
	Spec    Spec
	Inv     Invocation
	Dir     string
	Timeout time.Duration // 0 = no limit
	Log     io.Writer     // headless output; ignored for interactive runs
}

// Exec runs the agent to completion. Headless agents get a process group of
// their own; on timeout, cancellation or exit, that whole group is stopped,
// so nothing the agent started (MCP servers, dev servers) outlives the run.
func Exec(ctx context.Context, r Run) error {
	if _, err := exec.LookPath(r.Spec.Backend); err != nil {
		return fmt.Errorf("%s: not found on PATH", r.Spec.Backend)
	}
	cmd := exec.Command(r.Spec.Backend, r.Spec.Args(r.Inv)...)
	cmd.Dir = r.Dir

	if r.Inv.Interactive {
		// Same process group as grange: the agent's UI needs the terminal
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		interactiveRuns.Add(1)
		defer interactiveRuns.Add(-1)
	} else {
		cmd.Stdout, cmd.Stderr = r.Log, r.Log
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		// Don't wait on output pipes held open by processes the agent left behind
		cmd.WaitDelay = 5 * time.Second
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var timeout <-chan time.Time
	if r.Timeout > 0 {
		t := time.NewTimer(r.Timeout)
		defer t.Stop()
		timeout = t.C
	}

	select {
	case err := <-done:
		if !r.Inv.Interactive {
			signalGroup(cmd, syscall.SIGKILL) // leftovers only; the agent itself has exited
		}
		return err
	case <-timeout:
		stop(cmd, done)
		return ErrTimeout
	case <-ctx.Done():
		stop(cmd, done)
		return ctx.Err()
	}
}

func stop(cmd *exec.Cmd, done <-chan error) {
	signalGroup(cmd, syscall.SIGTERM)
	select {
	case <-done:
	case <-time.After(gracePeriod):
		signalGroup(cmd, syscall.SIGKILL)
		<-done
	}
	signalGroup(cmd, syscall.SIGKILL)
}

func signalGroup(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd.SysProcAttr != nil && cmd.SysProcAttr.Setpgid {
		_ = syscall.Kill(-cmd.Process.Pid, sig)
	} else {
		_ = cmd.Process.Signal(sig)
	}
}
