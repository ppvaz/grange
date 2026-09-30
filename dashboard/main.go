package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed static
var staticFiles embed.FS

// --- Types ---

type AgentInfo struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	LastRun string `json:"lastRun"`
	Elapsed int64  `json:"elapsed"`
}

type PlanInfo struct {
	Exists    bool     `json:"exists"`
	Pending   int      `json:"pending"`
	Completed int      `json:"completed"`
	Tasks     []string `json:"tasks"`
}

type FileInfo struct {
	Exists   bool   `json:"exists"`
	Lines    int    `json:"lines"`
	Modified string `json:"modified,omitempty"`
}

type PipelineStage struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type PipelineInfo struct {
	Active       bool            `json:"active"`
	CurrentStage int             `json:"currentStage"`
	Stages       []PipelineStage `json:"stages"`
	TargetPath   string          `json:"targetPath"`
	StartedAt    string          `json:"startedAt"`
}

type SpecsInfo struct {
	Patterns int            `json:"patterns"`
	Projects map[string]int `json:"projects"`
	Total    int            `json:"total"`
}

type LogEntry struct {
	Time    string `json:"time"`
	Message string `json:"message"`
	Raw     string `json:"raw"`
}

type CIInfo struct {
	Enabled    bool   `json:"enabled"`
	LastResult string `json:"lastResult"` // "pass", "fail", "unknown"
	LastRun    string `json:"lastRun"`
}

type GateInfo struct {
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"` // "disabled", "waiting", "approved"
}

type ReviewGateInfo struct {
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"` // "disabled", "pending", "acknowledged"
}

type HealthInfo struct {
	AgentCount   int    `json:"agentCount"`
	MaxAgents    int    `json:"maxAgents"`
	TimeoutCount int    `json:"timeoutCount"`
	CommitCount  int    `json:"commitCount"`
	SignalCount  int    `json:"signalCount"`
	VerifyCycles int    `json:"verifyCycles"`
	VisionHash   string `json:"visionHash"`
}

type ActionResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type DashboardState struct {
	WorkDir    string              `json:"workDir"`
	GrangeDir  string              `json:"grangeDir"`
	Agents     []AgentInfo         `json:"agents"`
	Plan       PlanInfo            `json:"plan"`
	Files      map[string]FileInfo `json:"files"`
	Pipeline   PipelineInfo        `json:"pipeline"`
	Specs      SpecsInfo           `json:"specs"`
	Log        []LogEntry          `json:"log"`
	Done       bool                `json:"done"`
	CI         CIInfo              `json:"ci"`
	Gate       GateInfo            `json:"gate"`
	ReviewGate ReviewGateInfo      `json:"reviewGate"`
	Mode       string              `json:"mode"`
	Health     HealthInfo          `json:"health"`
	Config     map[string]string   `json:"config"`
	Running    bool                `json:"running"`
}

// Multi-project types

type ProjectSummary struct {
	Name       string        `json:"name"`
	Path       string        `json:"path"`
	Status     string        `json:"status"`
	Done       int           `json:"done"`
	Total      int           `json:"total"`
	Commits    int           `json:"commits"`
	LastCommit string        `json:"lastCommit"`
	LastEpoch  int64         `json:"lastEpoch"`
	Goal       string        `json:"goal"`
	HasDone    bool          `json:"hasDone"`
	Signals    int           `json:"signals"`
	Pipeline   *PipelineInfo `json:"pipeline,omitempty"`
}

type AttentionItem struct {
	Project  string `json:"project"`
	Path     string `json:"path"`
	Reason   string `json:"reason"`
	Priority int    `json:"priority"`
}

type PortfolioStats struct {
	TotalDone    int            `json:"totalDone"`
	TotalTasks   int            `json:"totalTasks"`
	TotalCommits int            `json:"totalCommits"`
	ByStatus     map[string]int `json:"byStatus"`
}

type ProjectsResponse struct {
	ScanDir   string           `json:"scanDir"`
	Projects  []ProjectSummary `json:"projects"`
	Stats     PortfolioStats   `json:"stats"`
	Attention []AttentionItem  `json:"attention"`
}

// --- Config ---

var (
	workDir   string
	grangeDir string
	scanDir   string
	allMode   bool
	ansiRe    = regexp.MustCompile(`\033\[[0-9;]*m`)
)

var agentNames = []string{"Executor", "Planner", "Gap", "Oracle", "Visionary"}

// Valid agent CLI names for run-agent endpoint
var validAgentCLINames = map[string]bool{
	"executor": true, "planner": true, "gap": true, "oracle": true, "visionary": true,
}

// Process tracking for grow.sh
var (
	growCmd *exec.Cmd
	mu      sync.Mutex
	planMu  sync.Mutex
)

// Rate limiter
var (
	rateMu      sync.Mutex
	rateLimits  = make(map[string]time.Time)
)

// Config defaults for feature flags
var configDefaults = map[string]string{
	"GROW_MODE":             "auto",
	"ENABLE_CI":             "auto",
	"ENABLE_APPROVAL_GATE":  "false",
	"ENABLE_REVIEW_GATE":    "false",
	"ENABLE_TDD":            "true",
	"MAX_FILE_LINES":        "300",
	"REFACTOR_INTERVAL":     "5",
	"PAIR_STYLE":            "interactive",
	"PAIR_TIMEOUT":          "3600",
	"GRANGE_SMART":          "claude",
	"GRANGE_CHEAP":          "claude",
	"SMART_AGENTS":          "Oracle,Visionary",
	"AGENT_TIMEOUT":         "600",
	"ORACLE_TIMEOUT":        "1800",
	"MAX_PENDING_TASKS":     "5",
}

var stageIDs = []string{"0a", "0b", "1a", "1b", "2a", "2b"}
var stageNames = []string{"Lenses", "Synthesis", "Extraction", "Prompts", "Build", "Verify"}

var signalFiles = map[string]string{
	"vision":   "VISION.md",
	"blockers": "BLOCKERS.md",
	"drift":    "DRIFT.md",
	"cuts":     "CUTS.md",
	"review":   "VISION_REVIEW.md",
	"done":     "DONE.md",
	"plan":     "PLAN.md",
}

// Whitelist of readable files for /api/file endpoint
var readableFiles = map[string]string{
	"vision":   "VISION.md",
	"blockers": "BLOCKERS.md",
	"drift":    "DRIFT.md",
	"cuts":     "CUTS.md",
	"review":   "VISION_REVIEW.md",
	"done":     "DONE.md",
	"plan":     "PLAN.md",
	"log":      "LOG.md",
	"ci":       ".locks/ci.log",
}

// --- Filesystem readers (parameterized by dir) ---

func readCIStatus(dir string) CIInfo {
	ci := CIInfo{LastResult: "unknown"}

	// Check if ci.sh exists
	ciScript := filepath.Join(dir, "ci.sh")
	if _, err := os.Stat(ciScript); err != nil {
		return ci // ci.sh doesn't exist
	}
	ci.Enabled = true

	// Read last line of ci.log for result
	ciLog := filepath.Join(dir, ".locks", "ci.log")
	data, err := os.ReadFile(ciLog)
	if err != nil {
		return ci
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "PASS") {
			ci.LastResult = "pass"
			ci.LastRun = strings.TrimPrefix(line, "PASS ")
			break
		} else if strings.HasPrefix(line, "FAIL") {
			ci.LastResult = "fail"
			ci.LastRun = strings.TrimPrefix(line, "FAIL ")
			break
		}
	}
	return ci
}

func readGateStatus(dir string) GateInfo {
	gate := GateInfo{Status: "disabled"}

	// Check .env for ENABLE_APPROVAL_GATE=true
	envPath := filepath.Join(dir, ".env")
	data, err := os.ReadFile(envPath)
	if err == nil {
		kv := parseKeyValue(string(data))
		if v, ok := kv["ENABLE_APPROVAL_GATE"]; ok && v == "true" {
			gate.Enabled = true
			gate.Status = "waiting"
		}
	}

	if !gate.Enabled {
		return gate
	}

	// Check for .plan-approved signal
	approvalFile := filepath.Join(dir, ".plan-approved")
	if _, err := os.Stat(approvalFile); err == nil {
		gate.Status = "approved"
	}
	return gate
}

func readReviewGateStatus(dir string) ReviewGateInfo {
	rg := ReviewGateInfo{Status: "disabled"}

	// Check .env for ENABLE_REVIEW_GATE=true or GROW_MODE=pair
	envPath := filepath.Join(dir, ".env")
	data, err := os.ReadFile(envPath)
	if err == nil {
		kv := parseKeyValue(string(data))
		if v, ok := kv["ENABLE_REVIEW_GATE"]; ok && v == "true" {
			rg.Enabled = true
		}
		if v, ok := kv["GROW_MODE"]; ok && v == "pair" {
			rg.Enabled = true
		}
	}

	if !rg.Enabled {
		return rg
	}

	rg.Status = "clear"

	// Check for .vision-review-pending signal
	pendingFile := filepath.Join(dir, ".vision-review-pending")
	if _, err := os.Stat(pendingFile); err == nil {
		rg.Status = "pending"
		// Check for .vision-reviewed acknowledgement
		ackFile := filepath.Join(dir, ".vision-reviewed")
		if _, err := os.Stat(ackFile); err == nil {
			rg.Status = "acknowledged"
		}
	}
	return rg
}

func readGrowMode(dir string) string {
	envPath := filepath.Join(dir, ".env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		return "auto"
	}
	kv := parseKeyValue(string(data))
	if v, ok := kv["GROW_MODE"]; ok && (v == "pair" || v == "sleep" || v == "auto") {
		return v
	}
	return "auto"
}

func readHealth(dir string) HealthInfo {
	lockDir := filepath.Join(dir, ".locks")
	h := HealthInfo{MaxAgents: 2}

	readInt := func(name string) int {
		data, err := os.ReadFile(filepath.Join(lockDir, name))
		if err != nil {
			return 0
		}
		n, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		return n
	}

	h.AgentCount = readInt("agent_count")
	h.TimeoutCount = readInt("timeout_count")
	h.CommitCount = readInt("commit_count")
	h.SignalCount = readInt("signal_count")
	h.VerifyCycles = readInt("verify_cycles")

	// Read MAX_AGENTS from config if available
	envPath := filepath.Join(dir, ".env")
	if data, err := os.ReadFile(envPath); err == nil {
		kv := parseKeyValue(string(data))
		if v, ok := kv["MAX_AGENTS"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				h.MaxAgents = n
			}
		}
	}

	// Count active running agents from running_* dirs
	entries, err := os.ReadDir(lockDir)
	if err == nil {
		count := 0
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), "running_") {
				count++
			}
		}
		h.AgentCount = count
	}

	// Vision validated hash
	data, err := os.ReadFile(filepath.Join(lockDir, "vision_validated_hash"))
	if err == nil {
		h.VisionHash = strings.TrimSpace(string(data))
	}

	return h
}

func readConfig(dir string) map[string]string {
	config := make(map[string]string)
	for k, v := range configDefaults {
		config[k] = v
	}

	envPath := filepath.Join(dir, ".env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		return config
	}

	kv := parseKeyValue(string(data))
	for k := range configDefaults {
		if v, ok := kv[k]; ok {
			config[k] = v
		}
	}
	return config
}

func isRunning(dir string) bool {
	lockDir := filepath.Join(dir, ".locks")
	entries, err := os.ReadDir(lockDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "running_") {
			return true
		}
	}
	return false
}

func readAgents(dir string) []AgentInfo {
	lockDir := filepath.Join(dir, ".locks")
	agents := make([]AgentInfo, 0, len(agentNames))

	for _, name := range agentNames {
		agent := AgentInfo{Name: name, Status: "idle"}

		// Check if running
		runMarker := filepath.Join(lockDir, "running_"+name)
		if info, err := os.Stat(runMarker); err == nil && info.IsDir() {
			agent.Status = "running"
			agent.Elapsed = int64(time.Since(info.ModTime()).Seconds())
		}

		// Check if locked (but not running = stale/finishing)
		lockFile := filepath.Join(lockDir, name+".lock.d")
		if _, err := os.Stat(lockFile); err == nil && agent.Status != "running" {
			agent.Status = "locked"
		}

		// Read last run time
		timeFile := filepath.Join(lockDir, name+".last")
		if data, err := os.ReadFile(timeFile); err == nil {
			ts := strings.TrimSpace(string(data))
			if epoch, err := strconv.ParseInt(ts, 10, 64); err == nil {
				t := time.Unix(epoch, 0)
				agent.LastRun = t.Format("15:04:05")
				if agent.Status == "running" {
					agent.Elapsed = int64(time.Since(t).Seconds())
				}
			}
		}

		agents = append(agents, agent)
	}
	return agents
}

func readPlan(dir string) PlanInfo {
	planPath := filepath.Join(dir, "PLAN.md")
	plan := PlanInfo{Tasks: []string{}}

	data, err := os.ReadFile(planPath)
	if err != nil {
		return plan
	}
	plan.Exists = true

	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "- [X]") {
			plan.Completed++
			plan.Tasks = append(plan.Tasks, trimmed)
		} else if strings.HasPrefix(trimmed, "- [ ]") {
			plan.Pending++
			plan.Tasks = append(plan.Tasks, trimmed)
		}
	}
	return plan
}

func readFileInfo(dir, name, filename string) FileInfo {
	path := filepath.Join(dir, filename)
	fi := FileInfo{}

	stat, err := os.Stat(path)
	if err != nil {
		return fi
	}
	fi.Exists = true
	fi.Modified = stat.ModTime().Format(time.RFC3339)

	data, err := os.ReadFile(path)
	if err != nil {
		return fi
	}
	fi.Lines = strings.Count(string(data), "\n")
	if len(data) > 0 && data[len(data)-1] != '\n' {
		fi.Lines++
	}
	return fi
}

func readFiles(dir string) map[string]FileInfo {
	files := make(map[string]FileInfo)
	for key, filename := range signalFiles {
		files[key] = readFileInfo(dir, key, filename)
	}
	return files
}

func readPipeline(dir string) PipelineInfo {
	pipeline := PipelineInfo{
		Stages: make([]PipelineStage, len(stageIDs)),
	}

	for i, id := range stageIDs {
		pipeline.Stages[i] = PipelineStage{
			ID:     id,
			Name:   stageNames[i],
			Status: "pending",
		}
	}

	stateFile := filepath.Join(dir, ".ike-state")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return pipeline
	}

	pipeline.Active = true
	state := parseKeyValue(string(data))

	if v, ok := state["CURRENT_STAGE"]; ok {
		if stage, err := strconv.Atoi(v); err == nil {
			pipeline.CurrentStage = stage
			for i := range pipeline.Stages {
				if i <= stage {
					pipeline.Stages[i].Status = "complete"
				} else if i == stage+1 {
					pipeline.Stages[i].Status = "active"
				}
			}
		}
	}

	if v, ok := state["TARGET_PATH"]; ok {
		pipeline.TargetPath = v
	}
	if v, ok := state["STARTED_AT"]; ok {
		pipeline.StartedAt = v
	}

	return pipeline
}

func parseKeyValue(content string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			result[key] = val
		}
	}
	return result
}

func readSpecs(dir string) SpecsInfo {
	specsDir := filepath.Join(dir, "specs")
	specs := SpecsInfo{Projects: make(map[string]int)}

	entries, err := os.ReadDir(specsDir)
	if err != nil {
		return specs
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		count := countMDFiles(filepath.Join(specsDir, name))
		if count > 0 {
			if name == "patterns" {
				specs.Patterns = count
			} else {
				specs.Projects[name] = count
			}
			specs.Total += count
		}
	}
	return specs
}

func countMDFiles(dir string) int {
	count := 0
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".md") {
			count++
		}
		return nil
	})
	return count
}

func readLog(dir string) []LogEntry {
	logPath := filepath.Join(dir, "LOG.md")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	// Take last 50 lines
	start := 0
	if len(lines) > 50 {
		start = len(lines) - 50
	}
	lines = lines[start:]

	entries := make([]LogEntry, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		clean := ansiRe.ReplaceAllString(line, "")
		entry := LogEntry{Raw: clean}

		// Try to parse "HH:MM:SS message" format
		if len(clean) >= 8 && clean[2] == ':' && clean[5] == ':' {
			entry.Time = clean[:8]
			entry.Message = strings.TrimSpace(clean[8:])
		} else {
			entry.Message = clean
		}

		entries = append(entries, entry)
	}
	return entries
}

func checkDone(dir string) bool {
	donePath := filepath.Join(dir, "DONE.md")
	_, err := os.Stat(donePath)
	return err == nil
}

// --- Multi-project discovery ---

func discoverProjects(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var projects []string
	seen := make(map[string]bool)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectDir := filepath.Join(dir, entry.Name())

		// Check for grow.sh symlink pointing to grange
		growPath := filepath.Join(projectDir, "grow.sh")
		target, err := filepath.EvalSymlinks(growPath)
		if err == nil {
			expectedTarget := filepath.Join(grangeDir, "grow.sh")
			if target == expectedTarget {
				projects = append(projects, projectDir)
				seen[projectDir] = true
				continue
			}
		}

		// Check for .ike-state (IKE-only projects without grow.sh)
		ikeState := filepath.Join(projectDir, ".ike-state")
		if _, err := os.Stat(ikeState); err == nil && !seen[projectDir] {
			projects = append(projects, projectDir)
		}
	}
	return projects
}

func gitStats(dir string) (commits int, lastCommit string, lastEpoch int64) {
	// Commit count
	out, err := exec.Command("git", "-C", dir, "rev-list", "--count", "HEAD").Output()
	if err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil {
			commits = n
		}
	}

	// Last commit time
	out, err = exec.Command("git", "-C", dir, "log", "-1", "--format=%ct").Output()
	if err == nil {
		if epoch, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); err == nil {
			lastEpoch = epoch
			diff := time.Since(time.Unix(epoch, 0))
			switch {
			case diff < time.Hour:
				lastCommit = fmt.Sprintf("%dm ago", int(diff.Minutes()))
			case diff < 24*time.Hour:
				lastCommit = fmt.Sprintf("%dh ago", int(diff.Hours()))
			default:
				lastCommit = fmt.Sprintf("%dd ago", int(diff.Hours()/24))
			}
		}
	}
	if lastCommit == "" {
		lastCommit = "never"
	}
	return
}

func readGoal(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "VISION.md"))
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	inGoal := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "## Goal") {
			inGoal = true
			continue
		}
		if inGoal {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if strings.HasPrefix(trimmed, "##") {
				break
			}
			if len(trimmed) > 80 {
				return trimmed[:77] + "..."
			}
			return trimmed
		}
	}
	return ""
}

func countSignals(dir string) int {
	count := 0
	for _, f := range []string{"BLOCKERS.md", "CUTS.md", "DRIFT.md", "VISION_REVIEW.md"} {
		info, err := os.Stat(filepath.Join(dir, f))
		if err == nil && info.Size() > 0 {
			count++
		}
	}
	return count
}

func readProjectSummary(dir string) ProjectSummary {
	plan := readPlan(dir)
	commits, lastCommit, lastEpoch := gitStats(dir)
	hasDone := checkDone(dir)
	pipeline := readPipeline(dir)

	total := plan.Completed + plan.Pending
	status := "new"
	if hasDone {
		status = "done"
	} else if pipeline.Active && total == 0 {
		status = "ike"
	} else if lastEpoch > 0 && time.Since(time.Unix(lastEpoch, 0)) < 7*24*time.Hour {
		status = "active"
	} else if total > 0 {
		status = "stalled"
	}

	summary := ProjectSummary{
		Name:       filepath.Base(dir),
		Path:       dir,
		Status:     status,
		Done:       plan.Completed,
		Total:      total,
		Commits:    commits,
		LastCommit: lastCommit,
		LastEpoch:  lastEpoch,
		Goal:       readGoal(dir),
		HasDone:    hasDone,
		Signals:    countSignals(dir),
	}
	if pipeline.Active {
		summary.Pipeline = &pipeline
	}
	return summary
}

func buildAttention(projects []ProjectSummary) []AttentionItem {
	var items []AttentionItem

	for _, p := range projects {
		if p.Status == "done" {
			continue
		}

		// Stalled with incomplete tasks
		if p.Status == "stalled" && p.Total-p.Done > 0 {
			daysSince := 0
			if p.LastEpoch > 0 {
				daysSince = int(time.Since(time.Unix(p.LastEpoch, 0)).Hours() / 24)
			}
			items = append(items, AttentionItem{
				Project:  p.Name,
				Path:     p.Path,
				Reason:   fmt.Sprintf("Has %d pending tasks, inactive for %d days", p.Total-p.Done, daysSince),
				Priority: 1,
			})
		}

		// Non-empty BLOCKERS.md
		blockersPath := filepath.Join(p.Path, "BLOCKERS.md")
		if data, err := os.ReadFile(blockersPath); err == nil && len(strings.TrimSpace(string(data))) > 0 {
			summary := strings.TrimSpace(strings.Split(string(data), "\n")[0])
			if len(summary) > 80 {
				summary = summary[:77] + "..."
			}
			items = append(items, AttentionItem{
				Project:  p.Name,
				Path:     p.Path,
				Reason:   "Has active blockers: " + summary,
				Priority: 1,
			})
		}

		// Large DRIFT.md
		driftPath := filepath.Join(p.Path, "DRIFT.md")
		if data, err := os.ReadFile(driftPath); err == nil {
			lineCount := strings.Count(string(data), "\n")
			if lineCount > 100 {
				items = append(items, AttentionItem{
					Project:  p.Name,
					Path:     p.Path,
					Reason:   fmt.Sprintf("Significant drift detected (%d lines)", lineCount),
					Priority: 2,
				})
			}
		}

		// Unaddressed VISION_REVIEW.md
		reviewPath := filepath.Join(p.Path, "VISION_REVIEW.md")
		if data, err := os.ReadFile(reviewPath); err == nil && len(strings.TrimSpace(string(data))) > 0 {
			items = append(items, AttentionItem{
				Project:  p.Name,
				Path:     p.Path,
				Reason:   "Visionary observations pending review",
				Priority: 3,
			})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Priority < items[j].Priority
	})
	return items
}

// --- Handlers ---

func resolveDir(r *http.Request) string {
	if project := r.URL.Query().Get("project"); project != "" {
		return project
	}
	return workDir
}

func handleState(w http.ResponseWriter, r *http.Request) {
	dir := resolveDir(r)
	state := DashboardState{
		WorkDir:    dir,
		GrangeDir:  grangeDir,
		Agents:     readAgents(dir),
		Plan:       readPlan(dir),
		Files:      readFiles(dir),
		Pipeline:   readPipeline(dir),
		Specs:      readSpecs(dir),
		Log:        readLog(dir),
		Done:       checkDone(dir),
		CI:         readCIStatus(dir),
		Gate:       readGateStatus(dir),
		ReviewGate: readReviewGateStatus(dir),
		Mode:       readGrowMode(dir),
		Health:     readHealth(dir),
		Config:     readConfig(dir),
		Running:    isRunning(dir),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

func handleFile(w http.ResponseWriter, r *http.Request) {
	dir := resolveDir(r)

	// Extract key from path: /api/file/{key}
	key := strings.TrimPrefix(r.URL.Path, "/api/file/")
	key = strings.TrimSuffix(key, "/")

	filename, ok := readableFiles[key]
	if !ok {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	// Check for agent log request
	var path string
	if strings.HasPrefix(key, "agent-") {
		path = filepath.Join(dir, ".locks", filename+".log")
	} else {
		path = filepath.Join(dir, filename)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	// Strip ANSI codes
	clean := ansiRe.ReplaceAllString(string(data), "")

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(clean))
}

func handleProjects(w http.ResponseWriter, r *http.Request) {
	dirs := discoverProjects(scanDir)

	projects := make([]ProjectSummary, 0, len(dirs))
	for _, dir := range dirs {
		projects = append(projects, readProjectSummary(dir))
	}

	// Sort: active, ike, stalled, new, done
	statusOrder := map[string]int{"active": 0, "ike": 1, "stalled": 2, "new": 3, "done": 4}
	sort.Slice(projects, func(i, j int) bool {
		return statusOrder[projects[i].Status] < statusOrder[projects[j].Status]
	})

	// Portfolio stats
	stats := PortfolioStats{ByStatus: make(map[string]int)}
	for _, p := range projects {
		stats.TotalDone += p.Done
		stats.TotalTasks += p.Total
		stats.TotalCommits += p.Commits
		stats.ByStatus[p.Status]++
	}

	resp := ProjectsResponse{
		ScanDir:   scanDir,
		Projects:  projects,
		Stats:     stats,
		Attention: buildAttention(projects),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// --- Action helpers ---

func respondJSON(w http.ResponseWriter, status int, resp ActionResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

func requirePOST(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			respondJSON(w, http.StatusMethodNotAllowed, ActionResponse{Error: "POST required"})
			return
		}
		handler(w, r)
	}
}

func rateLimit(action string, cooldown time.Duration) bool {
	rateMu.Lock()
	defer rateMu.Unlock()
	if last, ok := rateLimits[action]; ok {
		if time.Since(last) < cooldown {
			return false
		}
	}
	rateLimits[action] = time.Now()
	return true
}

// --- Phase 1: Gate action handlers ---

func handleActionApprove(w http.ResponseWriter, r *http.Request) {
	dir := resolveDir(r)

	// Validate gate is enabled
	gate := readGateStatus(dir)
	if !gate.Enabled {
		respondJSON(w, http.StatusBadRequest, ActionResponse{Error: "Approval gate is not enabled"})
		return
	}
	if gate.Status != "waiting" {
		respondJSON(w, http.StatusConflict, ActionResponse{Error: "Gate is not in waiting state"})
		return
	}

	if !rateLimit("approve", 2*time.Second) {
		respondJSON(w, http.StatusTooManyRequests, ActionResponse{Error: "Please wait before retrying"})
		return
	}

	if err := os.WriteFile(filepath.Join(dir, ".plan-approved"), []byte(""), 0644); err != nil {
		respondJSON(w, http.StatusInternalServerError, ActionResponse{Error: "Failed to write approval file"})
		return
	}

	respondJSON(w, http.StatusOK, ActionResponse{OK: true, Message: "Plan approved"})
}

func handleActionReviewAck(w http.ResponseWriter, r *http.Request) {
	dir := resolveDir(r)

	// Validate review gate is enabled
	rg := readReviewGateStatus(dir)
	if !rg.Enabled {
		respondJSON(w, http.StatusBadRequest, ActionResponse{Error: "Review gate is not enabled"})
		return
	}
	if rg.Status != "pending" {
		respondJSON(w, http.StatusConflict, ActionResponse{Error: "No pending review to acknowledge"})
		return
	}

	if !rateLimit("review-ack", 2*time.Second) {
		respondJSON(w, http.StatusTooManyRequests, ActionResponse{Error: "Please wait before retrying"})
		return
	}

	if err := os.WriteFile(filepath.Join(dir, ".vision-reviewed"), []byte(""), 0644); err != nil {
		respondJSON(w, http.StatusInternalServerError, ActionResponse{Error: "Failed to write review acknowledgement"})
		return
	}

	respondJSON(w, http.StatusOK, ActionResponse{OK: true, Message: "Review acknowledged"})
}

// --- Phase 2: Agent lifecycle handlers ---

func resolveGrowPath(dir string) (string, error) {
	growPath := filepath.Join(dir, "grow.sh")
	// Verify grow.sh exists (symlink or regular file)
	if _, err := os.Lstat(growPath); err != nil {
		return "", fmt.Errorf("grow.sh not found in %s", dir)
	}
	return growPath, nil
}

func handleActionStart(w http.ResponseWriter, r *http.Request) {
	dir := resolveDir(r)

	if isRunning(dir) {
		respondJSON(w, http.StatusConflict, ActionResponse{Error: "Agents are already running"})
		return
	}

	if !rateLimit("start", 5*time.Second) {
		respondJSON(w, http.StatusTooManyRequests, ActionResponse{Error: "Please wait before retrying"})
		return
	}

	growPath, err := resolveGrowPath(dir)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, ActionResponse{Error: err.Error()})
		return
	}

	cmd := exec.Command("bash", growPath, "start")
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		respondJSON(w, http.StatusInternalServerError, ActionResponse{Error: "Failed to start grow.sh: " + err.Error()})
		return
	}

	mu.Lock()
	growCmd = cmd
	mu.Unlock()

	// Reap the process in background to avoid zombies
	go cmd.Wait()

	respondJSON(w, http.StatusOK, ActionResponse{OK: true, Message: "Agents started"})
}

func handleActionStop(w http.ResponseWriter, r *http.Request) {
	dir := resolveDir(r)

	if !rateLimit("stop", 5*time.Second) {
		respondJSON(w, http.StatusTooManyRequests, ActionResponse{Error: "Please wait before retrying"})
		return
	}

	mu.Lock()
	cmd := growCmd
	mu.Unlock()

	stopped := false

	// Try stored command first
	if cmd != nil && cmd.Process != nil {
		pgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err == nil {
			syscall.Kill(-pgid, syscall.SIGTERM)
			stopped = true
		}
	}

	// Also try pgrep fallback for externally-started grow.sh, which execs
	// the Go binary (bin/grange grow start)
	if !stopped {
		out, err := exec.Command("pgrep", "-f", "grange grow start|grow.sh start").Output()
		if err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if pid, err := strconv.Atoi(strings.TrimSpace(line)); err == nil {
					syscall.Kill(pid, syscall.SIGTERM)
					stopped = true
				}
			}
		}
	}

	if !stopped {
		respondJSON(w, http.StatusConflict, ActionResponse{Error: "No running agents found"})
		return
	}

	// Clean up running markers after a brief delay
	go func() {
		time.Sleep(3 * time.Second)
		lockDir := filepath.Join(dir, ".locks")
		entries, err := os.ReadDir(lockDir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), "running_") {
				os.RemoveAll(filepath.Join(lockDir, e.Name()))
			}
		}
		mu.Lock()
		growCmd = nil
		mu.Unlock()
	}()

	respondJSON(w, http.StatusOK, ActionResponse{OK: true, Message: "Stop signal sent"})
}

func handleActionRunAgent(w http.ResponseWriter, r *http.Request) {
	dir := resolveDir(r)
	agent := r.URL.Query().Get("agent")

	if !validAgentCLINames[agent] {
		respondJSON(w, http.StatusBadRequest, ActionResponse{Error: "Invalid agent name. Must be one of: executor, planner, gap, oracle, visionary"})
		return
	}

	if !rateLimit("run-agent-"+agent, 5*time.Second) {
		respondJSON(w, http.StatusTooManyRequests, ActionResponse{Error: "Please wait before retrying"})
		return
	}

	growPath, err := resolveGrowPath(dir)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, ActionResponse{Error: err.Error()})
		return
	}

	cmd := exec.Command("bash", growPath, agent)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		respondJSON(w, http.StatusInternalServerError, ActionResponse{Error: "Failed to run agent: " + err.Error()})
		return
	}

	go cmd.Wait()

	respondJSON(w, http.StatusOK, ActionResponse{OK: true, Message: agent + " agent started"})
}

// --- Phase 3: Plan management handlers ---

func handleActionPlanAdd(w http.ResponseWriter, r *http.Request) {
	dir := resolveDir(r)

	var body struct {
		Task string `json:"task"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondJSON(w, http.StatusBadRequest, ActionResponse{Error: "Invalid request body"})
		return
	}

	task := strings.TrimSpace(body.Task)
	task = strings.ReplaceAll(task, "\n", " ")
	task = strings.ReplaceAll(task, "\r", " ")
	if task == "" {
		respondJSON(w, http.StatusBadRequest, ActionResponse{Error: "Task text is required"})
		return
	}
	if len(task) > 500 {
		respondJSON(w, http.StatusBadRequest, ActionResponse{Error: "Task text exceeds 500 character limit"})
		return
	}

	if !rateLimit("plan-add", 1*time.Second) {
		respondJSON(w, http.StatusTooManyRequests, ActionResponse{Error: "Please wait before retrying"})
		return
	}

	planPath := filepath.Join(dir, "PLAN.md")

	planMu.Lock()
	defer planMu.Unlock()

	data, err := os.ReadFile(planPath)
	if err != nil {
		// PLAN.md doesn't exist yet — create it
		data = []byte{}
	}

	content := string(data)
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "- [ ] " + task + "\n"

	if err := os.WriteFile(planPath, []byte(content), 0644); err != nil {
		respondJSON(w, http.StatusInternalServerError, ActionResponse{Error: "Failed to write PLAN.md"})
		return
	}

	respondJSON(w, http.StatusOK, ActionResponse{OK: true, Message: "Task added"})
}

func handleActionPlanToggle(w http.ResponseWriter, r *http.Request) {
	dir := resolveDir(r)

	var body struct {
		Index int `json:"index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondJSON(w, http.StatusBadRequest, ActionResponse{Error: "Invalid request body"})
		return
	}

	if !rateLimit("plan-toggle", 1*time.Second) {
		respondJSON(w, http.StatusTooManyRequests, ActionResponse{Error: "Please wait before retrying"})
		return
	}

	planPath := filepath.Join(dir, "PLAN.md")

	planMu.Lock()
	defer planMu.Unlock()

	data, err := os.ReadFile(planPath)
	if err != nil {
		respondJSON(w, http.StatusNotFound, ActionResponse{Error: "PLAN.md not found"})
		return
	}

	lines := strings.Split(string(data), "\n")
	taskIdx := 0
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "- [X]") || strings.HasPrefix(trimmed, "- [ ]") {
			if taskIdx == body.Index {
				// Toggle
				if strings.HasPrefix(trimmed, "- [ ]") {
					lines[i] = strings.Replace(line, "- [ ]", "- [x]", 1)
				} else {
					lines[i] = regexp.MustCompile(`- \[[xX]\]`).ReplaceAllString(line, "- [ ]")
				}
				found = true
				break
			}
			taskIdx++
		}
	}

	if !found {
		respondJSON(w, http.StatusBadRequest, ActionResponse{Error: "Task index out of range"})
		return
	}

	if err := os.WriteFile(planPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		respondJSON(w, http.StatusInternalServerError, ActionResponse{Error: "Failed to write PLAN.md"})
		return
	}

	respondJSON(w, http.StatusOK, ActionResponse{OK: true, Message: "Task toggled"})
}

func handleActionPlanRemove(w http.ResponseWriter, r *http.Request) {
	dir := resolveDir(r)

	var body struct {
		Index  int    `json:"index"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondJSON(w, http.StatusBadRequest, ActionResponse{Error: "Invalid request body"})
		return
	}

	reason := strings.TrimSpace(body.Reason)
	if reason == "" {
		reason = "cut from dashboard"
	}

	if !rateLimit("plan-remove", 1*time.Second) {
		respondJSON(w, http.StatusTooManyRequests, ActionResponse{Error: "Please wait before retrying"})
		return
	}

	planPath := filepath.Join(dir, "PLAN.md")
	cutsPath := filepath.Join(dir, "CUTS.md")

	planMu.Lock()
	defer planMu.Unlock()

	data, err := os.ReadFile(planPath)
	if err != nil {
		respondJSON(w, http.StatusNotFound, ActionResponse{Error: "PLAN.md not found"})
		return
	}

	lines := strings.Split(string(data), "\n")
	taskIdx := 0
	removedLine := -1
	var taskText string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "- [X]") || strings.HasPrefix(trimmed, "- [ ]") {
			if taskIdx == body.Index {
				taskText = regexp.MustCompile(`^- \[[ xX]\]\s*`).ReplaceAllString(trimmed, "")
				removedLine = i
				break
			}
			taskIdx++
		}
	}

	if removedLine < 0 {
		respondJSON(w, http.StatusBadRequest, ActionResponse{Error: "Task index out of range"})
		return
	}

	// Remove line from PLAN.md
	newLines := append(lines[:removedLine], lines[removedLine+1:]...)
	if err := os.WriteFile(planPath, []byte(strings.Join(newLines, "\n")), 0644); err != nil {
		respondJSON(w, http.StatusInternalServerError, ActionResponse{Error: "Failed to write PLAN.md"})
		return
	}

	// Append to CUTS.md
	cutEntry := fmt.Sprintf("- %s — cut: %s (%s)\n", taskText, reason, time.Now().Format("2006-01-02"))
	cutsData, _ := os.ReadFile(cutsPath)
	cutsContent := string(cutsData)
	if len(cutsContent) > 0 && !strings.HasSuffix(cutsContent, "\n") {
		cutsContent += "\n"
	}
	cutsContent += cutEntry
	os.WriteFile(cutsPath, []byte(cutsContent), 0644)

	respondJSON(w, http.StatusOK, ActionResponse{OK: true, Message: "Task cut: " + taskText})
}

// --- Phase 4: CI trigger handler ---

func handleActionCIRun(w http.ResponseWriter, r *http.Request) {
	dir := resolveDir(r)

	ciPath := filepath.Join(dir, "ci.sh")
	info, err := os.Stat(ciPath)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, ActionResponse{Error: "ci.sh not found"})
		return
	}
	if info.Mode()&0111 == 0 {
		respondJSON(w, http.StatusBadRequest, ActionResponse{Error: "ci.sh is not executable"})
		return
	}

	if !rateLimit("ci-run", 10*time.Second) {
		respondJSON(w, http.StatusTooManyRequests, ActionResponse{Error: "CI recently triggered, please wait"})
		return
	}

	// Run CI in background
	go func() {
		lockDir := filepath.Join(dir, ".locks")
		os.MkdirAll(lockDir, 0755)
		logPath := filepath.Join(lockDir, "ci.log")

		logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		defer logFile.Close()

		cmd := exec.Command("bash", ciPath)
		cmd.Dir = dir
		cmd.Stdout = logFile
		cmd.Stderr = logFile

		ts := time.Now().Format("2006-01-02 15:04:05")
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(logFile, "FAIL %s\n", ts)
		} else {
			fmt.Fprintf(logFile, "PASS %s\n", ts)
		}
	}()

	respondJSON(w, http.StatusOK, ActionResponse{OK: true, Message: "CI run started"})
}

func init() {
	// Add agent log keys to readable files
	for _, name := range agentNames {
		key := "agent-" + strings.ToLower(name)
		readableFiles[key] = name // store original-cased name for path resolution
	}
}

// --- Main ---

func main() {
	port := flag.String("p", "3000", "port")
	work := flag.String("w", ".", "work directory to monitor")
	grange := flag.String("g", "", "grange directory (default: binary's parent)")
	allFlag := flag.Bool("a", false, "multi-project overview mode")
	scan := flag.String("s", "", "scan directory for multi-project mode")
	flag.Parse()

	allMode = *allFlag

	var err error
	workDir, err = filepath.Abs(*work)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving work dir: %v\n", err)
		os.Exit(1)
	}

	if *grange != "" {
		grangeDir, err = filepath.Abs(*grange)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving grange dir: %v\n", err)
			os.Exit(1)
		}
	} else {
		exe, err := os.Executable()
		if err != nil {
			grangeDir = workDir
		} else {
			grangeDir = filepath.Dir(exe)
		}
	}

	if *scan != "" {
		scanDir, err = filepath.Abs(*scan)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving scan dir: %v\n", err)
			os.Exit(1)
		}
	}

	// Serve embedded static files
	staticSub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error accessing static files: %v\n", err)
		os.Exit(1)
	}
	staticServer := http.FileServer(http.FS(staticSub))

	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", handleState)
	mux.HandleFunc("/api/file/", handleFile)

	// Action endpoints
	mux.HandleFunc("/api/action/approve", requirePOST(handleActionApprove))
	mux.HandleFunc("/api/action/review-ack", requirePOST(handleActionReviewAck))
	mux.HandleFunc("/api/action/start", requirePOST(handleActionStart))
	mux.HandleFunc("/api/action/stop", requirePOST(handleActionStop))
	mux.HandleFunc("/api/action/run-agent", requirePOST(handleActionRunAgent))
	mux.HandleFunc("/api/action/plan/add", requirePOST(handleActionPlanAdd))
	mux.HandleFunc("/api/action/plan/toggle", requirePOST(handleActionPlanToggle))
	mux.HandleFunc("/api/action/plan/remove", requirePOST(handleActionPlanRemove))
	mux.HandleFunc("/api/action/ci/run", requirePOST(handleActionCIRun))

	if allMode {
		mux.HandleFunc("/api/projects", handleProjects)
		// Serve overview.html at root, index.html at /project
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				// Serve overview.html
				data, err := fs.ReadFile(staticSub, "overview.html")
				if err != nil {
					http.Error(w, "overview not found", http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(data)
				return
			}
			staticServer.ServeHTTP(w, r)
		})
		mux.HandleFunc("/project", func(w http.ResponseWriter, r *http.Request) {
			data, err := fs.ReadFile(staticSub, "index.html")
			if err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(data)
		})
	} else {
		mux.Handle("/", staticServer)
	}

	fmt.Printf("\033[32m🌾 Grange Dashboard\033[0m\n")
	if allMode {
		fmt.Printf("   Mode:       multi-project overview\n")
		fmt.Printf("   Scanning:   %s\n", scanDir)
	} else {
		fmt.Printf("   Monitoring: %s\n", workDir)
	}
	fmt.Printf("   Grange:     %s\n", grangeDir)
	fmt.Printf("   URL:        http://localhost:%s\n\n", *port)

	if err := http.ListenAndServe(":"+*port, mux); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
