package grow

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/ppvaz/grange/internal/workspace"
)

// ciScript returns the path of an executable ci.sh, or "".
func (g *Grow) ciScript() string {
	path := g.ws.Path("ci.sh")
	if info, err := os.Stat(path); err == nil && info.Mode()&0o111 != 0 {
		return path
	}
	return ""
}

// runCI runs ci.sh per ENABLE_CI and records the result in .locks/ci.log.
func (g *Grow) runCI() bool {
	if g.cfg.CI == "false" {
		return true
	}
	logFile, err := os.OpenFile(g.ws.LockPath("ci.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		g.log.Printf(workspace.Red, "CI", "%v", err)
		return false
	}
	defer logFile.Close()
	stamp := func() string { return time.Now().Format(time.UnixDate) }

	script := g.ciScript()
	if script == "" {
		if g.cfg.CI == "true" {
			g.log.Printf(workspace.Red, "CI", "ENABLE_CI=true but ci.sh is missing or not executable")
			fmt.Fprintf(logFile, "FAIL: ci.sh not found (%s)\n", stamp())
			return false
		}
		return true
	}

	g.log.Printf(workspace.Blue, "CI", "Running ci.sh...")
	fmt.Fprintf(logFile, "=== CI run at %s ===\n", stamp())
	cmd := exec.Command(script)
	cmd.Dir = g.ws.Dir
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Run(); err != nil {
		g.log.Printf(workspace.Red, "CI", "Failed (%v)", err)
		fmt.Fprintf(logFile, "FAIL %v (%s)\n", err, stamp())
		return false
	}
	g.log.Printf(workspace.Green, "CI", "Passed")
	fmt.Fprintf(logFile, "PASS (%s)\n", stamp())
	return true
}
