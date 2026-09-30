package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func open(t *testing.T, plan string) *Workspace {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "PLAN.md"), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestTasks(t *testing.T) {
	w := open(t, "# Plan\n- [x] Setup\n- [ ] Build API\n  - [ ] Nested step\nnot a task - [ ] here\n- [X] Docs\n")
	pending, done := w.Tasks()
	if want := []string{"Build API", "Nested step"}; !reflect.DeepEqual(pending, want) {
		t.Errorf("pending = %q, want %q", pending, want)
	}
	if want := []string{"Setup", "Docs"}; !reflect.DeepEqual(done, want) {
		t.Errorf("done = %q, want %q", done, want)
	}
}

func TestAddTasksSkipsDuplicatesAndKeepsNewline(t *testing.T) {
	w := open(t, "- [ ] Fix: tests fail")
	added, err := w.AddTasks([]string{"Fix: tests fail", "Fix: lint", "", "Fix: lint"})
	if err != nil || added != 1 {
		t.Fatalf("added %d, %v; want 1", added, err)
	}
	if got, want := w.Read("PLAN.md"), "- [ ] Fix: tests fail\n- [ ] Fix: lint\n"; got != want {
		t.Errorf("PLAN.md = %q, want %q", got, want)
	}
}

func TestOpenClearsStaleMarkers(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, ".locks", "running_Executor")
	os.MkdirAll(stale, 0o755)
	os.WriteFile(filepath.Join(stale, "pid"), []byte("999999\n"), 0o644)
	w, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale running marker should be removed")
	}
	if n := w.Counter("agent_count"); n != 0 {
		t.Errorf("agent_count = %d", n)
	}
}

func TestMarkRunning(t *testing.T) {
	w := open(t, "")
	done := w.MarkRunning("Oracle")
	if _, err := os.Stat(w.LockPath("running_Oracle/pid")); err != nil {
		t.Error("expected running marker with pid")
	}
	if w.Counter("agent_count") != 1 || w.Counter("Oracle.last") == 0 {
		t.Error("expected agent_count=1 and Oracle.last")
	}
	done()
	if _, err := os.Stat(w.LockPath("running_Oracle")); !os.IsNotExist(err) || w.Counter("agent_count") != 0 {
		t.Error("marker should be cleared")
	}
}

func TestOpenRemovesLegacyHookOnly(t *testing.T) {
	dir := t.TempDir()
	hooks := filepath.Join(dir, ".git", "hooks")
	os.MkdirAll(hooks, 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".git", "refs"), 0o755)
	own := "#!/bin/sh\necho mine\n"
	os.WriteFile(filepath.Join(hooks, "post-commit"), []byte(own+legacyHook), 0o755)
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(hooks, "post-commit"))
	if string(got) != own {
		t.Errorf("hook = %q, want only the user's part %q", got, own)
	}
}
