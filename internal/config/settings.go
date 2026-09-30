package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ppvaz/grange/internal/agent"
)

// Settings is everything grow and reap read from the environment.
type Settings struct {
	Mode             string // auto | pair | sleep
	CI               string // auto | true | false
	ApprovalGate     bool
	ReviewGate       bool
	TDD              bool
	AlwaysVisionary  bool // pair mode validates the vision on every start
	MaxFileLines     int
	RefactorInterval int
	MaxPendingTasks  int
	PairStyle        string // interactive | plan
	PairTimeout      time.Duration
	AgentTimeout     time.Duration
	OracleTimeout    time.Duration
	JobTimeout       time.Duration // reap's one-shot jobs (lenses, synthesis)

	Smart       agent.Spec
	Cheap       agent.Spec
	SmartAgents []string
	overrides   map[string]agent.Spec
}

// Load reads Settings from the environment (call LoadDotEnv first).
func Load() (Settings, error) {
	s := Settings{
		Mode:             env("GROW_MODE", "auto"),
		CI:               env("ENABLE_CI", "auto"),
		TDD:              envBool("ENABLE_TDD", true),
		MaxFileLines:     envInt("MAX_FILE_LINES", 300),
		RefactorInterval: envInt("REFACTOR_INTERVAL", 5),
		MaxPendingTasks:  envInt("MAX_PENDING_TASKS", 5),
		PairStyle:        env("PAIR_STYLE", "interactive"),
		PairTimeout:      envSeconds("PAIR_TIMEOUT", 3600),
		AgentTimeout:     envSeconds("AGENT_TIMEOUT", 600),
		OracleTimeout:    envSeconds("ORACLE_TIMEOUT", 1800),
		JobTimeout:       envSeconds("REAP_JOB_TIMEOUT", 1800),
		SmartAgents:      splitList(env("SMART_AGENTS", "Oracle,Visionary")),
		overrides:        map[string]agent.Spec{},
	}

	switch s.Mode {
	case "pair":
		s.ApprovalGate = envBool("ENABLE_APPROVAL_GATE", true)
		s.ReviewGate = envBool("ENABLE_REVIEW_GATE", true)
		s.AlwaysVisionary = true
	case "sleep", "auto":
		s.ApprovalGate = envBool("ENABLE_APPROVAL_GATE", false)
		s.ReviewGate = envBool("ENABLE_REVIEW_GATE", false)
	default:
		return s, fmt.Errorf("GROW_MODE=%q: want auto, pair or sleep", s.Mode)
	}

	var err error
	if s.Smart, err = agent.ParseSpec(env("GRANGE_SMART", "claude")); err != nil {
		return s, fmt.Errorf("GRANGE_SMART: %w", err)
	}
	if s.Cheap, err = agent.ParseSpec(env("GRANGE_CHEAP", "claude")); err != nil {
		return s, fmt.Errorf("GRANGE_CHEAP: %w", err)
	}
	for _, kv := range os.Environ() {
		key, value, _ := strings.Cut(kv, "=")
		role, ok := strings.CutPrefix(key, "GRANGE_AGENT_")
		if !ok || value == "" {
			continue
		}
		spec, err := agent.ParseSpec(value)
		if err != nil {
			return s, fmt.Errorf("%s: %w", key, err)
		}
		s.overrides[strings.ToLower(role)] = spec
	}
	return s, nil
}

// SpecFor picks the backend for a role: GRANGE_AGENT_<ROLE> if set, else the
// smart tier for roles in SMART_AGENTS, else the cheap tier.
func (s Settings) SpecFor(role string) agent.Spec {
	if spec, ok := s.overrides[strings.ToLower(role)]; ok {
		return spec
	}
	for _, smart := range s.SmartAgents {
		if strings.EqualFold(smart, role) {
			return s.Smart
		}
	}
	return s.Cheap
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v == "true" || v == "1"
}

func envInt(key string, fallback int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return n
	}
	return fallback
}

func envSeconds(key string, fallback int) time.Duration {
	return time.Duration(envInt(key, fallback)) * time.Second
}

func splitList(s string) []string {
	var out []string
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
