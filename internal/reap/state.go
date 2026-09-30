package reap

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// State is .ike-state: plain KEY=value lines, unquoted, because the
// dashboard reads them as-is.
type State struct {
	CurrentStage int // last completed stage, -1 before any
	TargetPath   string
	TargetStack  string
	BuildDir     string
	StartedAt    string
}

func statePath(dir string) string { return filepath.Join(dir, ".ike-state") }

func loadState(dir string) (State, error) {
	st := State{CurrentStage: -1}
	data, err := os.ReadFile(statePath(dir))
	if err != nil {
		return st, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		value = strings.Trim(value, `"'`)
		switch key {
		case "CURRENT_STAGE":
			if n, err := strconv.Atoi(value); err == nil {
				st.CurrentStage = n
			}
		case "TARGET_PATH":
			st.TargetPath = value
		case "TARGET_STACK":
			st.TargetStack = value
		case "BUILD_DIR":
			st.BuildDir = value
		case "STARTED_AT":
			st.StartedAt = value
		}
	}
	return st, nil
}

func (st State) save(dir string) error {
	content := fmt.Sprintf("CURRENT_STAGE=%d\nTARGET_PATH=%s\nTARGET_STACK=%s\nBUILD_DIR=%s\nSTARTED_AT=%s\n",
		st.CurrentStage, st.TargetPath, st.TargetStack, st.BuildDir, st.StartedAt)
	return os.WriteFile(statePath(dir), []byte(content), 0o644)
}
