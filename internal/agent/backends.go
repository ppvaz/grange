// Package agent runs coding-agent CLIs (Claude Code, Codex, Antigravity,
// OpenCode) behind one interface, headless or attached to the terminal.
package agent

import (
	"fmt"
	"sort"
	"strings"
)

// Spec names a backend and, optionally, the model it should use. It is
// written "backend" or "backend:model", e.g. "codex:gpt-5.5" or
// "opencode:anthropic/claude-sonnet-5-5".
type Spec struct {
	Backend string
	Model   string
}

// Invocation is what one agent run should do, independent of the backend.
type Invocation struct {
	Prompt string
	// AddDirs are extra directories the agent may read, e.g. the codebase
	// reap is analysing. Backends without such a flag get full access anyway.
	AddDirs []string
	// Interactive attaches the agent's own UI to the terminal (pair mode).
	Interactive bool
	// PlanMode asks an interactive agent to propose changes before making them.
	PlanMode bool
}

type backend struct {
	headless    func(model string, inv Invocation) []string
	interactive func(model string, inv Invocation) []string
}

// Headless runs auto-approve everything: grange agents work unattended and
// already need shell access, so a per-tool allowlist buys nothing.
var backends = map[string]backend{
	"claude": {
		headless: func(model string, inv Invocation) []string {
			args := []string{"-p", inv.Prompt, "--dangerously-skip-permissions"}
			return append(args, common(model, "--model", inv.AddDirs, "--add-dir")...)
		},
		interactive: func(model string, inv Invocation) []string {
			args := planOr(inv, []string{"--permission-mode", "plan"}, []string{"--dangerously-skip-permissions"})
			args = append(args, common(model, "--model", inv.AddDirs, "--add-dir")...)
			return append(args, inv.Prompt)
		},
	},
	"codex": {
		headless: func(model string, inv Invocation) []string {
			args := []string{"exec", "--dangerously-bypass-approvals-and-sandbox", "--skip-git-repo-check"}
			args = append(args, common(model, "--model", inv.AddDirs, "--add-dir")...)
			return append(args, inv.Prompt)
		},
		// Codex has no plan mode: keep its own approval prompts on and let the
		// pair-mode prompt ask for a plan first
		interactive: func(model string, inv Invocation) []string {
			args := planOr(inv, nil, []string{"--dangerously-bypass-approvals-and-sandbox"})
			args = append(args, common(model, "--model", inv.AddDirs, "--add-dir")...)
			return append(args, inv.Prompt)
		},
	},
	"agy": {
		headless: func(model string, inv Invocation) []string {
			args := []string{"--print", inv.Prompt, "--dangerously-skip-permissions"}
			return append(args, common(model, "--model", inv.AddDirs, "--add-dir")...)
		},
		interactive: func(model string, inv Invocation) []string {
			args := planOr(inv, []string{"--mode", "plan"}, []string{"--dangerously-skip-permissions"})
			args = append(args, common(model, "--model", inv.AddDirs, "--add-dir")...)
			return append(args, "--prompt-interactive", inv.Prompt)
		},
	},
	"opencode": {
		headless: func(model string, inv Invocation) []string {
			args := []string{"run", "--auto"}
			args = append(args, common(model, "--model", nil, "")...)
			return append(args, inv.Prompt)
		},
		interactive: func(model string, inv Invocation) []string {
			args := planOr(inv, []string{"--agent", "plan"}, []string{"--auto"})
			args = append(args, common(model, "--model", nil, "")...)
			return append(args, "--prompt", inv.Prompt)
		},
	},
}

func planOr(inv Invocation, plan, normal []string) []string {
	if inv.PlanMode {
		return append([]string(nil), plan...)
	}
	return append([]string(nil), normal...)
}

func common(model, modelFlag string, dirs []string, dirFlag string) []string {
	var args []string
	if model != "" {
		args = append(args, modelFlag, model)
	}
	for _, d := range dirs {
		args = append(args, dirFlag, d)
	}
	return args
}

// Backends lists the supported backend names.
func Backends() []string {
	names := make([]string, 0, len(backends))
	for name := range backends {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ParseSpec parses "backend" or "backend:model". Only the first colon splits,
// so models may contain colons (e.g. "opencode:ollama/qwen3:32b").
func ParseSpec(s string) (Spec, error) {
	name, model, _ := strings.Cut(strings.TrimSpace(s), ":")
	if _, ok := backends[name]; !ok {
		return Spec{}, fmt.Errorf("unknown agent backend %q in %q (want one of: %s)", name, s, strings.Join(Backends(), ", "))
	}
	return Spec{Backend: name, Model: model}, nil
}

func (s Spec) String() string {
	if s.Model == "" {
		return s.Backend
	}
	return s.Backend + ":" + s.Model
}

// Args returns the command-line arguments (without the binary) for inv.
func (s Spec) Args(inv Invocation) []string {
	b := backends[s.Backend]
	if inv.Interactive {
		return b.interactive(s.Model, inv)
	}
	return b.headless(s.Model, inv)
}
