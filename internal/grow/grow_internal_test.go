package grow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptsRender(t *testing.T) {
	data := promptData{TDD: true, CI: true, Checkpoint: "half done", CheckpointFile: "/c", Minutes: 10,
		MaxFileLines: 300, Date: "2026-09-30", Range: "a..b", DiffRange: "a b", VerifyCycles: 3, VerdictFile: "/v.json"}
	for _, name := range []string{"preamble", "executor", "planner", "gap", "oracle", "visionary"} {
		if out := render(name, data); strings.Contains(out, "<no value>") || len(out) < 50 {
			t.Errorf("%s rendered badly:\n%s", name, out)
		}
	}
	if out := render("oracle", data); !strings.Contains(out, "SKIP booting") || !strings.Contains(out, "/v.json") {
		t.Errorf("oracle prompt should skip booting after 3 cycles and name the verdict file:\n%s", out)
	}
	if out := render("executor", data); !strings.Contains(out, "RESUME FROM CHECKPOINT") || !strings.Contains(out, "~10 minutes") {
		t.Errorf("executor prompt missing checkpoint or timeout:\n%s", out)
	}
	if out := render("planner", promptData{Populate: true}); !strings.Contains(out, "PLAN.md is empty") || strings.Contains(out, "ALIGNMENT CHECK") {
		t.Errorf("populate planner prompt:\n%s", out)
	}
}

func TestReadVerdict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v.json")
	if _, err := readVerdict(path); err == nil || !strings.Contains(err.Error(), "didn't write") {
		t.Errorf("missing file: %v", err)
	}
	os.WriteFile(path, []byte(`{"summary": "no done field"}`), 0o644)
	if _, err := readVerdict(path); err == nil || !strings.Contains(err.Error(), `no "done" field`) {
		t.Errorf("missing done: %v", err)
	}
	os.WriteFile(path, []byte(`{"done": false, "failures": ["x"]}`), 0o644)
	if v, err := readVerdict(path); err != nil || *v.Done || v.Failures[0] != "x" {
		t.Errorf("valid verdict: %+v, %v", v, err)
	}
}
