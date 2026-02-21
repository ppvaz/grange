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

type DashboardState struct {
	WorkDir   string              `json:"workDir"`
	GrangeDir string              `json:"grangeDir"`
	Agents    []AgentInfo         `json:"agents"`
	Plan      PlanInfo            `json:"plan"`
	Files     map[string]FileInfo `json:"files"`
	Pipeline  PipelineInfo        `json:"pipeline"`
	Specs     SpecsInfo           `json:"specs"`
	Log       []LogEntry          `json:"log"`
	Done      bool                `json:"done"`
}

// Multi-project types

type ProjectSummary struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Status     string `json:"status"`
	Done       int    `json:"done"`
	Total      int    `json:"total"`
	Commits    int    `json:"commits"`
	LastCommit string `json:"lastCommit"`
	LastEpoch  int64  `json:"lastEpoch"`
	Goal       string `json:"goal"`
	HasDone    bool   `json:"hasDone"`
	Signals    int    `json:"signals"`
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

var agentNames = []string{"Executor", "Planner", "Critic", "Gap", "Oracle", "Visionary"}

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
}

// --- Filesystem readers (parameterized by dir) ---

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
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		growPath := filepath.Join(dir, entry.Name(), "grow.sh")
		target, err := filepath.EvalSymlinks(growPath)
		if err != nil {
			continue
		}
		expectedTarget := filepath.Join(grangeDir, "grow.sh")
		if target == expectedTarget {
			projects = append(projects, filepath.Join(dir, entry.Name()))
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

	total := plan.Completed + plan.Pending
	status := "new"
	if hasDone {
		status = "done"
	} else if lastEpoch > 0 && time.Since(time.Unix(lastEpoch, 0)) < 7*24*time.Hour {
		status = "active"
	} else if total > 0 {
		status = "stalled"
	}

	return ProjectSummary{
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
		WorkDir:   dir,
		GrangeDir: grangeDir,
		Agents:    readAgents(dir),
		Plan:      readPlan(dir),
		Files:     readFiles(dir),
		Pipeline:  readPipeline(dir),
		Specs:     readSpecs(dir),
		Log:       readLog(dir),
		Done:      checkDone(dir),
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

	// Sort: active, stalled, new, done
	statusOrder := map[string]int{"active": 0, "stalled": 1, "new": 2, "done": 3}
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
