package grow

import (
	"embed"
	"strings"
	"text/template"
)

//go:embed prompts/*.md
var promptFS embed.FS

var prompts = template.Must(template.ParseFS(promptFS, "prompts/*.md"))

// promptData is everything the prompt templates can reference.
type promptData struct {
	Pair, PlanFirst  bool
	TDD, CI          bool
	Populate, Specs  bool
	Checkpoint       string
	CheckpointFile   string
	Minutes          int
	MaxFileLines     int
	Date             string
	Range, DiffRange string
	Trigger, History string
	VerifyCycles     int
	VerdictFile      string
}

func render(name string, data any) string {
	var b strings.Builder
	if err := prompts.ExecuteTemplate(&b, name+".md", data); err != nil {
		panic(err) // templates are embedded and covered by tests
	}
	return b.String()
}
