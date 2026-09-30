package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/ppvaz/grange/internal/config"
	"github.com/ppvaz/grange/internal/grow"
	"github.com/ppvaz/grange/internal/workspace"
)

func growCmd(ctx context.Context, dir string, args []string) error {
	sub := "start"
	if len(args) > 0 {
		sub = args[0]
	}
	settings, err := config.Load()
	if err != nil {
		return err
	}
	g, err := grow.New(dir, settings)
	if err != nil {
		return err
	}
	switch sub {
	case "start":
		err = g.Run(ctx)
	case "executor", "planner", "gap", "oracle", "visionary":
		err = g.RunRole(ctx, sub)
	case "status":
		g.Status(os.Stdout)
		return nil
	default:
		fmt.Fprintln(os.Stderr, "Usage: grow.sh {start|executor|planner|gap|oracle|visionary|status}")
		return exitError{2}
	}
	if errors.Is(err, context.Canceled) {
		g.Workspace().Log.Printf(workspace.Yellow, "Main", "Shut down")
		return nil
	}
	return err
}

func digestCmd(ctx context.Context, dir string) error {
	settings, err := config.Load()
	if err != nil {
		return err
	}
	g, err := grow.New(dir, settings)
	if err != nil {
		return err
	}
	if err := g.Preflight(grow.Digest); err != nil {
		return err
	}
	return g.RunDigest(ctx)
}
