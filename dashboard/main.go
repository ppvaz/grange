package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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

// --- Config ---

var (
	workDir   string
	grangeDir string
	ansiRe    = regexp.MustCompile(`\033\[[0-9;]*m`)
)

var agentNames = []string{"Executor", "Planner", "Critic", "Gap", "Oracle", "Visionary"}

var stageIDs = []string{"0a", "0b", "1a", "1b", "2a", "2b"}
var stageNames = []string{"Lenses", "Synthesis", "Extraction", "Prompts", "Build", "Verify"}

var signalFiles = map[string]string{
	"vision":  "VISION.md",
	"blockers": "BLOCKERS.md",
	"drift":   "DRIFT.md",
	"cuts":    "CUTS.md",
	"review":  "VISION_REVIEW.md",
	"done":    "DONE.md",
	"plan":    "PLAN.md",
}

// Whitelist of readable files for /api/file endpoint
var readableFiles = map[string]string{
	"vision":  "VISION.md",
	"blockers": "BLOCKERS.md",
	"drift":   "DRIFT.md",
	"cuts":    "CUTS.md",
	"review":  "VISION_REVIEW.md",
	"done":    "DONE.md",
	"plan":    "PLAN.md",
	"log":     "LOG.md",
}

// --- Filesystem readers ---

func readAgents() []AgentInfo {
	lockDir := filepath.Join(workDir, ".locks")
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

func readPlan() PlanInfo {
	planPath := filepath.Join(workDir, "PLAN.md")
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

func readFileInfo(name, filename string) FileInfo {
	path := filepath.Join(workDir, filename)
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

func readFiles() map[string]FileInfo {
	files := make(map[string]FileInfo)
	for key, filename := range signalFiles {
		files[key] = readFileInfo(key, filename)
	}
	return files
}

func readPipeline() PipelineInfo {
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

	stateFile := filepath.Join(workDir, ".ike-state")
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

func readSpecs() SpecsInfo {
	specsDir := filepath.Join(workDir, "specs")
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

func readLog() []LogEntry {
	logPath := filepath.Join(workDir, "LOG.md")
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

func checkDone() bool {
	donePath := filepath.Join(workDir, "DONE.md")
	_, err := os.Stat(donePath)
	return err == nil
}

// --- Handlers ---

func handleState(w http.ResponseWriter, r *http.Request) {
	state := DashboardState{
		WorkDir:   workDir,
		GrangeDir: grangeDir,
		Agents:    readAgents(),
		Plan:      readPlan(),
		Files:     readFiles(),
		Pipeline:  readPipeline(),
		Specs:     readSpecs(),
		Log:       readLog(),
		Done:      checkDone(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

func handleFile(w http.ResponseWriter, r *http.Request) {
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
		path = filepath.Join(workDir, ".locks", filename+".log")
	} else {
		path = filepath.Join(workDir, filename)
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
	flag.Parse()

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

	// Serve embedded static files
	staticSub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error accessing static files: %v\n", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", handleState)
	mux.HandleFunc("/api/file/", handleFile)
	mux.Handle("/", http.FileServer(http.FS(staticSub)))

	fmt.Printf("\033[32m🌾 Grange Dashboard\033[0m\n")
	fmt.Printf("   Monitoring: %s\n", workDir)
	fmt.Printf("   Grange:     %s\n", grangeDir)
	fmt.Printf("   URL:        http://localhost:%s\n\n", *port)

	if err := http.ListenAndServe(":"+*port, mux); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
