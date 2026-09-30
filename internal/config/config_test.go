package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ppvaz/grange/internal/agent"
)

func TestParseLine(t *testing.T) {
	t.Setenv("HOME_DIR", "/home/x")
	tests := []struct {
		line, key, value string
		ok               bool
	}{
		{"GRANGE_CHEAP=codex:gpt-5.5", "GRANGE_CHEAP", "codex:gpt-5.5", true},
		{`export GRANGE_SMART="claude:claude-opus-5-5"`, "GRANGE_SMART", "claude:claude-opus-5-5", true},
		{"GROW_MODE=pair   # interactive", "GROW_MODE", "pair", true},
		{`PATH_X="$HOME_DIR/bin"`, "PATH_X", "/home/x/bin", true},
		{`LITERAL='$HOME_DIR # not a comment'`, "LITERAL", "$HOME_DIR # not a comment", true},
		{"# GROW_MODE=auto", "", "", false},
		{"", "", "", false},
		{"not a pair", "", "", false},
	}
	for _, tt := range tests {
		key, value, ok := parseLine(tt.line)
		if key != tt.key || value != tt.value || ok != tt.ok {
			t.Errorf("parseLine(%q) = %q, %q, %v; want %q, %q, %v", tt.line, key, value, ok, tt.key, tt.value, tt.ok)
		}
	}
}

func TestLoadDotEnvPrefersProjectAndExistingEnv(t *testing.T) {
	project, home := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(project, ".env"), []byte("GT_A=project\nGT_B=project\n"), 0o644)
	os.WriteFile(filepath.Join(home, ".env"), []byte("GT_C=home\n"), 0o644)
	t.Setenv("GT_B", "from-env")
	os.Unsetenv("GT_A")
	os.Unsetenv("GT_C")
	t.Cleanup(func() { os.Unsetenv("GT_A"); os.Unsetenv("GT_C") })

	if err := LoadDotEnv(project, home); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("GT_A"); got != "project" {
		t.Errorf("GT_A = %q", got)
	}
	if got := os.Getenv("GT_B"); got != "from-env" {
		t.Errorf("GT_B = %q, existing env should win", got)
	}
	if got := os.Getenv("GT_C"); got != "" {
		t.Errorf("GT_C = %q, home .env should be skipped when the project has one", got)
	}
}

func TestSpecFor(t *testing.T) {
	t.Setenv("GRANGE_SMART", "claude:claude-opus-5-5")
	t.Setenv("GRANGE_CHEAP", "codex:gpt-5.5")
	t.Setenv("GRANGE_AGENT_GAP", "opencode:anthropic/claude-sonnet-5-5")
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]agent.Spec{
		"Oracle":   {Backend: "claude", Model: "claude-opus-5-5"},
		"Executor": {Backend: "codex", Model: "gpt-5.5"},
		"Gap":      {Backend: "opencode", Model: "anthropic/claude-sonnet-5-5"},
	}
	for role, want := range tests {
		if got := s.SpecFor(role); got != want {
			t.Errorf("SpecFor(%s) = %v, want %v", role, got, want)
		}
	}
}

func TestLoadRejectsBadConfig(t *testing.T) {
	t.Setenv("GRANGE_CHEAP", "claude-cheap")
	if _, err := Load(); err == nil {
		t.Error("unknown backend should fail")
	}
	t.Setenv("GRANGE_CHEAP", "")
	t.Setenv("GROW_MODE", "turbo")
	if _, err := Load(); err == nil {
		t.Error("unknown GROW_MODE should fail")
	}
}

func TestPairModePresets(t *testing.T) {
	t.Setenv("GROW_MODE", "pair")
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !s.ApprovalGate || !s.ReviewGate || !s.AlwaysVisionary {
		t.Errorf("pair mode should turn gates and startup Visionary on: %+v", s)
	}
	t.Setenv("ENABLE_APPROVAL_GATE", "false")
	if s, _ = Load(); s.ApprovalGate {
		t.Error("ENABLE_APPROVAL_GATE should override the pair preset")
	}
}
