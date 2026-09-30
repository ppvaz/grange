package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestParseSpec(t *testing.T) {
	tests := []struct {
		in   string
		want Spec
	}{
		{"claude", Spec{"claude", ""}},
		{"codex:gpt-5.5", Spec{"codex", "gpt-5.5"}},
		{" agy:gemini-3.1-pro-high ", Spec{"agy", "gemini-3.1-pro-high"}},
		{"opencode:ollama/qwen3:32b", Spec{"opencode", "ollama/qwen3:32b"}},
	}
	for _, tt := range tests {
		got, err := ParseSpec(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("ParseSpec(%q) = %v, %v; want %v", tt.in, got, err, tt.want)
		}
	}
	if _, err := ParseSpec("claude-cheap"); err == nil || !strings.Contains(err.Error(), "want one of: agy, claude, codex, opencode") {
		t.Errorf("unknown backend: got %v", err)
	}
}

func TestHeadlessArgs(t *testing.T) {
	inv := Invocation{Prompt: "do it", AddDirs: []string{"/target"}}
	tests := []struct {
		spec Spec
		want []string
	}{
		{Spec{"claude", "claude-sonnet-5-5"}, []string{"-p", "do it", "--dangerously-skip-permissions", "--model", "claude-sonnet-5-5", "--add-dir", "/target"}},
		{Spec{"codex", "gpt-5.5"}, []string{"exec", "--dangerously-bypass-approvals-and-sandbox", "--skip-git-repo-check", "--model", "gpt-5.5", "--add-dir", "/target", "do it"}},
		{Spec{"agy", ""}, []string{"--print", "do it", "--dangerously-skip-permissions", "--add-dir", "/target"}},
		{Spec{"opencode", "anthropic/claude-sonnet-5-5"}, []string{"run", "--auto", "--model", "anthropic/claude-sonnet-5-5", "do it"}},
	}
	for _, tt := range tests {
		if got := tt.spec.Args(inv); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s headless:\n got %q\nwant %q", tt.spec, got, tt.want)
		}
	}
}

func TestInteractiveArgs(t *testing.T) {
	normal := Invocation{Prompt: "pair", Interactive: true}
	plan := Invocation{Prompt: "pair", Interactive: true, PlanMode: true}
	tests := []struct {
		backend      string
		normal, plan []string
	}{
		{"claude", []string{"--dangerously-skip-permissions", "pair"}, []string{"--permission-mode", "plan", "pair"}},
		{"codex", []string{"--dangerously-bypass-approvals-and-sandbox", "pair"}, []string{"pair"}},
		{"agy", []string{"--dangerously-skip-permissions", "--prompt-interactive", "pair"}, []string{"--mode", "plan", "--prompt-interactive", "pair"}},
		{"opencode", []string{"--auto", "--prompt", "pair"}, []string{"--agent", "plan", "--prompt", "pair"}},
	}
	for _, tt := range tests {
		spec := Spec{Backend: tt.backend}
		if got := spec.Args(normal); !reflect.DeepEqual(got, tt.normal) {
			t.Errorf("%s interactive: got %q, want %q", tt.backend, got, tt.normal)
		}
		if got := spec.Args(plan); !reflect.DeepEqual(got, tt.plan) {
			t.Errorf("%s plan: got %q, want %q", tt.backend, got, tt.plan)
		}
	}
}

// stubBackend puts a fake `claude` first on PATH that records the PIDs of
// itself and a background child in pidFile, then runs body.
func stubBackend(t *testing.T, body string) (pidFile string) {
	t.Helper()
	dir := t.TempDir()
	pidFile = filepath.Join(dir, "pids")
	script := "#!/bin/sh\necho $$ >> " + pidFile + "\nsleep 300 &\necho $! >> " + pidFile + "\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return pidFile
}

func assertAllDead(t *testing.T, pidFile string) {
	t.Helper()
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range strings.Fields(string(data)) {
		pid, _ := strconv.Atoi(field)
		for i := 0; syscall.Kill(pid, 0) == nil; i++ {
			if i == 50 {
				t.Errorf("pid %d still running", pid)
				syscall.Kill(pid, syscall.SIGKILL)
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func TestExecTimeoutStopsWholeTree(t *testing.T) {
	pidFile := stubBackend(t, "sleep 300")
	err := Exec(context.Background(), Run{Spec: Spec{Backend: "claude"}, Dir: t.TempDir(), Timeout: 300 * time.Millisecond, Log: os.Stderr})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("got %v, want ErrTimeout", err)
	}
	assertAllDead(t, pidFile)
}

func TestExecCancelStopsWholeTree(t *testing.T) {
	pidFile := stubBackend(t, "sleep 300")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := Exec(ctx, Run{Spec: Spec{Backend: "claude"}, Dir: t.TempDir(), Log: os.Stderr}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want context cancellation", err)
	}
	assertAllDead(t, pidFile)
}

func TestExecCleansUpLeftoversAfterNormalExit(t *testing.T) {
	pidFile := stubBackend(t, "exit 0")
	if err := Exec(context.Background(), Run{Spec: Spec{Backend: "claude"}, Dir: t.TempDir(), Log: os.Stderr}); err != nil {
		t.Fatal(err)
	}
	assertAllDead(t, pidFile)
}

func TestExecMissingBackend(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := Exec(context.Background(), Run{Spec: Spec{Backend: "codex"}, Dir: t.TempDir()})
	if err == nil || err.Error() != "codex: not found on PATH" {
		t.Fatalf("got %v", err)
	}
}
