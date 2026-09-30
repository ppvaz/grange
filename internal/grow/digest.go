package grow

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ppvaz/grange/internal/agent"
	"github.com/ppvaz/grange/internal/workspace"
)

// Digest markers record how many lines of each observation file are already
// digested, as "FILE:count" lines in .locks/digest_markers.
func (g *Grow) digestMarkers() map[string]int {
	markers := map[string]int{}
	data, _ := os.ReadFile(g.ws.LockPath("digest_markers"))
	for _, line := range strings.Split(string(data), "\n") {
		if file, count, ok := strings.Cut(line, ":"); ok {
			markers[file], _ = strconv.Atoi(count)
		}
	}
	return markers
}

func (g *Grow) digestBacklog() int {
	markers, backlog := g.digestMarkers(), 0
	for _, f := range workspace.ObservationFiles {
		backlog += max(g.ws.LineCount(f)-markers[f], 0)
	}
	return backlog
}

type digestSection struct{ File, Content string }

// RunDigest summarises observations added since the last digest into
// HUMAN_DIGEST.md, for async human review.
func (g *Grow) RunDigest(ctx context.Context) error {
	markers := g.digestMarkers()
	var metrics []string
	var sections []digestSection
	for _, f := range workspace.ObservationFiles {
		// Counts match `wc -l` (as the markers do); content includes a final
		// line with no newline
		fresh := g.ws.LineCount(f) - markers[f]
		lines := strings.SplitAfter(g.ws.Read(f), "\n")
		if fresh <= 0 || markers[f] > len(lines) {
			metrics = append(metrics, "- "+f+": no new content")
			continue
		}
		metrics = append(metrics, fmt.Sprintf("- %s: +%d lines", f, fresh))
		sections = append(sections, digestSection{f, strings.TrimRight(strings.Join(lines[markers[f]:], ""), "\n")})
	}
	if len(sections) == 0 {
		g.log.Printf(workspace.Yellow, Digest, "No new observations since last digest")
		return nil
	}

	g.log.Printf(workspace.Blue, Digest, "Compiling digest from observation files...")
	prompt := render("digest", map[string]any{
		"Metrics":   strings.Join(metrics, "\n"),
		"Sections":  sections,
		"Generated": time.Now().Format("2006-01-02 15:04"),
	})
	digest, err := agent.Ask(ctx, agent.Run{
		Spec:    g.cfg.SpecFor(Digest),
		Inv:     agent.Invocation{Prompt: prompt},
		Dir:     g.ws.Dir,
		Timeout: g.cfg.AgentTimeout,
		Log:     g.log.Output(nil),
	})
	if err != nil {
		return fmt.Errorf("digest: %w", err)
	}

	f, err := os.OpenFile(g.ws.Path("HUMAN_DIGEST.md"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "\n---\n\n%s\n", strings.TrimSpace(digest)); err != nil {
		return err
	}

	var b strings.Builder
	for _, file := range workspace.ObservationFiles {
		fmt.Fprintf(&b, "%s:%d\n", file, g.ws.LineCount(file))
	}
	g.log.Printf(workspace.Green, Digest, "Digest written to HUMAN_DIGEST.md")
	return os.WriteFile(g.ws.LockPath("digest_markers"), []byte(b.String()), 0o644)
}
