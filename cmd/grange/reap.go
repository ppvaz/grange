package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ppvaz/grange/internal/config"
	"github.com/ppvaz/grange/internal/reap"
)

const reapUsage = `Usage: reap.sh <command> [args]

  start  /path/to/target [--stack S] [--build-dir D]   Begin at Stage 0a (work dir: cwd)
  resume [work-dir] [--stack S] [--build-dir D]       Continue after the last completed stage
  status [work-dir]                                   Show pipeline progress
  reset  <stage> [work-dir]                           Make resume start at stage 0-5

--stack / --build-dir answer Stage 2a's questions up front (needed without a terminal).`

func reapCmd(ctx context.Context, workDir string, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, reapUsage)
		return exitError{2}
	}
	var opts reap.Options
	var positional []string
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--stack", "--build-dir":
			if i+1 >= len(args) {
				return fmt.Errorf("%s needs a value", args[i])
			}
			if args[i] == "--stack" {
				opts.Stack = args[i+1]
			} else {
				opts.BuildDir = args[i+1]
			}
			i++
		default:
			positional = append(positional, args[i])
		}
	}
	dirArg := func(n int) (string, error) {
		if len(positional) > n {
			return filepath.Abs(positional[n])
		}
		return workDir, nil
	}

	settings, err := config.Load()
	if err != nil {
		return err
	}
	visions := filepath.Join(grangeHome(), "visions", "ike-v3")

	switch args[0] {
	case "start":
		if len(positional) == 0 {
			return errors.New("missing target codebase path: reap.sh start /path/to/target")
		}
		err = reap.Start(ctx, workDir, positional[0], visions, settings, opts)
	case "resume":
		dir, derr := dirArg(0)
		if derr != nil {
			return derr
		}
		err = reap.Resume(ctx, dir, visions, settings, opts)
	case "status":
		dir, derr := dirArg(0)
		if derr != nil {
			return derr
		}
		return reap.Status(dir, os.Stdout)
	case "reset":
		if len(positional) == 0 {
			return errors.New("missing stage index: reap.sh reset <stage> [work-dir]")
		}
		n, perr := strconv.Atoi(positional[0])
		if perr != nil {
			return fmt.Errorf("stage must be a number 0-5, got %q", positional[0])
		}
		dir, derr := dirArg(1)
		if derr != nil {
			return derr
		}
		return reap.Reset(dir, n)
	default:
		fmt.Fprintln(os.Stderr, reapUsage)
		return exitError{2}
	}

	switch {
	case errors.Is(err, reap.ErrInterrupted):
		return exitError{130}
	case errors.Is(err, reap.ErrIncomplete):
		return exitError{1}
	}
	return err
}
