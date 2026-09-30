// Package agent runs coding-agent CLIs (Claude Code, Codex, Antigravity,
// OpenCode) behind one interface, headless or attached to the terminal.
package agent

import (
	"fmt"
	"sort"
	"strings"
)

// Spec names a backend and, optionally, the model and reasoning effort it
// should use, written "backend[:model][@effort]", e.g. "codex:gpt-6.1-sol@low"
// or "claude:claude-opus-5-5@high".
type Spec struct {
	Backend string
	Model   string
	Effort  string
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
	effort  func(level string) []string
	addDirs bool // has --add-dir
	// opts are the model, effort and extra-dir flags
	headless    func(inv Invocation, opts []string) []string
	interactive func(inv Invocation, opts []string) []string
}

// Headless runs auto-approve everything: grange agents work unattended and
// already need shell access, so a per-tool allowlist buys nothing.
var backends = map[string]backend{
	"claude": {
		effort:  func(level string) []string { return []string{"--effort", level} },
		addDirs: true,
		headless: func(inv Invocation, opts []string) []string {
			return append([]string{"-p", inv.Prompt, "--dangerously-skip-permissions"}, opts...)
		},
		interactive: func(inv Invocation, opts []string) []string {
			args := planOr(inv, []string{"--permission-mode", "plan"}, []string{"--dangerously-skip-permissions"})
			return append(append(args, opts...), inv.Prompt)
		},
	},
	"codex": {
		// No effort flag; the config key can be overridden per run
		effort:  func(level string) []string { return []string{"-c", fmt.Sprintf("model_reasoning_effort=%q", level)} },
		addDirs: true,
		headless: func(inv Invocation, opts []string) []string {
			args := []string{"exec", "--dangerously-bypass-approvals-and-sandbox", "--skip-git-repo-check"}
			return append(append(args, opts...), inv.Prompt)
		},
		// Codex has no plan mode: keep its own approval prompts on and let the
		// pair-mode prompt ask for a plan first
		interactive: func(inv Invocation, opts []string) []string {
			args := planOr(inv, nil, []string{"--dangerously-bypass-approvals-and-sandbox"})
			return append(append(args, opts...), inv.Prompt)
		},
	},
	"agy": {
		effort:  func(level string) []string { return []string{"--effort", level} },
		addDirs: true,
		headless: func(inv Invocation, opts []string) []string {
			return append([]string{"--print", inv.Prompt, "--dangerously-skip-permissions"}, opts...)
		},
		interactive: func(inv Invocation, opts []string) []string {
			args := planOr(inv, []string{"--mode", "plan"}, []string{"--dangerously-skip-permissions"})
			return append(append(args, opts...), "--prompt-interactive", inv.Prompt)
		},
	},
	"opencode": {
		effort: func(level string) []string { return []string{"--variant", level} },
		headless: func(inv Invocation, opts []string) []string {
			return append(append([]string{"run", "--auto"}, opts...), inv.Prompt)
		},
		interactive: func(inv Invocation, opts []string) []string {
			args := planOr(inv, []string{"--agent", "plan"}, []string{"--auto"})
			return append(append(args, opts...), "--prompt", inv.Prompt)
		},
	},
}

func planOr(inv Invocation, plan, normal []string) []string {
	if inv.PlanMode {
		return append([]string(nil), plan...)
	}
	return append([]string(nil), normal...)
}

// effortLevels are the words any backend accepts as a reasoning effort. Only
// these count after "@", so model IDs containing "@" (Vertex dates) survive.
var effortLevels = map[string]bool{"none": true, "minimal": true, "low": true, "medium": true, "high": true, "xhigh": true, "max": true, "ultra": true}

// Backends lists the supported backend names.
func Backends() []string {
	names := make([]string, 0, len(backends))
	for name := range backends {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ParseSpec parses "backend[:model][@effort]". Only the first colon splits,
// so models may contain colons (e.g. "opencode:ollama/qwen3:32b").
func ParseSpec(s string) (Spec, error) {
	rest, effort := strings.TrimSpace(s), ""
	if i := strings.LastIndex(rest, "@"); i >= 0 && effortLevels[rest[i+1:]] {
		rest, effort = rest[:i], rest[i+1:]
	}
	name, model, _ := strings.Cut(rest, ":")
	if _, ok := backends[name]; !ok {
		return Spec{}, fmt.Errorf("unknown agent backend %q in %q (want one of: %s)", name, s, strings.Join(Backends(), ", "))
	}
	return Spec{Backend: name, Model: model, Effort: effort}, nil
}

func (s Spec) String() string {
	out := s.Backend
	if s.Model != "" {
		out += ":" + s.Model
	}
	if s.Effort != "" {
		out += "@" + s.Effort
	}
	return out
}

// Args returns the command-line arguments (without the binary) for inv.
func (s Spec) Args(inv Invocation) []string {
	b := backends[s.Backend]
	var opts []string
	if s.Model != "" {
		opts = append(opts, "--model", s.Model)
	}
	if s.Effort != "" {
		opts = append(opts, b.effort(s.Effort)...)
	}
	if b.addDirs {
		for _, d := range inv.AddDirs {
			opts = append(opts, "--add-dir", d)
		}
	}
	if inv.Interactive {
		return b.interactive(inv, opts)
	}
	return b.headless(inv, opts)
}
