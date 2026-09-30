package reap

import (
	"fmt"
	"os"
	"path/filepath"
)

type kind int

const (
	lenses   kind = iota // three one-shot lens jobs in parallel
	oneShot              // one job does everything in the stage's VISION.md
	growLoop             // a full grow loop on the stage's VISION.md
)

type stage struct {
	ID, Name, Vision, Description, Checkpoint string
	Kind                                      kind
	// Artifacts returns what's missing for the stage to count as complete.
	Artifacts func(dir string) []string
}

type lens struct{ Task, Name, File string }

var lensJobs = []lens{
	{"1", "Business Analyst", "recon/lens-business-analyst.md"},
	{"2", "Product Manager", "recon/lens-product-manager.md"},
	{"3", "QA Adversarial", "recon/lens-qa-adversarial.md"},
}

var stages = []stage{
	{
		ID: "0a", Name: "Lens Analysis", Vision: "VISION-stage0a-lenses.md", Kind: lenses,
		Description: "Run three parallel analyses (BA, PM, QA lenses) of the target codebase",
		Checkpoint:  "Review lens files → Create recon/HUMAN-CONTEXT.md",
		Artifacts: func(dir string) []string {
			var files []string
			for _, l := range lensJobs {
				files = append(files, l.File)
			}
			return missingFiles(dir, files...)
		},
	},
	{
		ID: "0b", Name: "Synthesis", Vision: "VISION-stage0b-synthesis.md", Kind: oneShot,
		Description: "Reconcile lenses with your context, generate extraction spec",
		Checkpoint:  "Review recon/VISION-stage1-extraction.md → Check BLOCKERS.md",
		Artifacts: func(dir string) []string {
			return missingFiles(dir, "recon/synthesis.md", "recon/VISION-stage1-extraction.md")
		},
	},
	{
		ID: "1a", Name: "Deep Extraction", Vision: "VISION-stage1a-extraction.md", Kind: growLoop,
		Description: "Extract entities, rules, flows, integrations with confidence scores",
		Checkpoint:  "Review CONFIDENCE-SUMMARY.md → Validate low-confidence items",
		Artifacts: func(dir string) []string {
			return append(missingDirs(dir, "knowledge/entities", "knowledge/rules", "knowledge/flows", "knowledge/integrations"),
				missingFiles(dir, "knowledge/CONFIDENCE-SUMMARY.md")...)
		},
	},
	{
		ID: "1b", Name: "Prompts & Review", Vision: "VISION-stage1b-prompts.md", Kind: growLoop,
		Description: "Generate atomic prompts, dependency graph, adversarial review",
		Checkpoint:  "Review RISK-REGISTER.md → Check EXTRACTION-COMPLETE.md",
		Artifacts: func(dir string) []string {
			return append(missingDirs(dir, "knowledge/prompts"), missingFiles(dir, "knowledge/EXTRACTION-COMPLETE.md")...)
		},
	},
	{
		ID: "2a", Name: "Build", Vision: "VISION-stage2a-build.md", Kind: growLoop,
		Description: "Pre-build decisions, project setup, implement prompts",
		Checkpoint:  "Review PROGRESS.md → Unblock issues → Add docs/HUMAN-DECISIONS.md",
		Artifacts:   func(dir string) []string { return missingDirs(dir, "src") },
	},
	{
		ID: "2b", Name: "Verify", Vision: "VISION-stage2b-verify.md", Kind: growLoop,
		Description: "Full test suite, flow verification, risk resolution, final report",
		Checkpoint:  "Review REBUILD-COMPLETE.md → Ship or iterate",
		Artifacts:   func(dir string) []string { return missingFiles(dir, "docs/REBUILD-COMPLETE.md") },
	},
}

// buildStage is the first stage that runs in BUILD_DIR when one is set.
const buildStage = 4

func missingFiles(dir string, files ...string) []string {
	var missing []string
	for _, f := range files {
		if info, err := os.Stat(filepath.Join(dir, f)); err != nil || info.Size() == 0 {
			missing = append(missing, f)
		}
	}
	return missing
}

func missingDirs(dir string, dirs ...string) []string {
	var missing []string
	for _, d := range dirs {
		if entries, err := os.ReadDir(filepath.Join(dir, d)); err != nil || len(entries) == 0 {
			missing = append(missing, d+"/")
		}
	}
	return missing
}

// artifactSummary lists a completed stage's key outputs for the checkpoint.
func artifactSummary(idx int, dir string) []string {
	count := func(d string) string {
		entries, _ := os.ReadDir(filepath.Join(dir, d))
		return fmt.Sprintf("%s/ (%d files)", d, len(entries))
	}
	switch idx {
	case 2:
		return []string{count("knowledge/entities"), count("knowledge/rules"), count("knowledge/flows"), count("knowledge/integrations"), "knowledge/CONFIDENCE-SUMMARY.md"}
	case 3:
		return []string{count("knowledge/prompts"), "knowledge/RISK-REGISTER.md", "knowledge/EXTRACTION-COMPLETE.md"}
	case 4:
		return []string{"src/", "tests/", "docs/TRACEABILITY.md", "docs/PROGRESS.md"}
	case 5:
		return []string{"docs/TEST-RESULTS.md", "docs/FLOW-VERIFICATION.md", "docs/RISK-RESOLUTION.md", "docs/REBUILD-COMPLETE.md"}
	case 1:
		return []string{"recon/synthesis.md", "recon/hotspot-map.md", "recon/complexity-assessment.md", "recon/VISION-stage1-extraction.md"}
	default:
		var files []string
		for _, l := range lensJobs {
			files = append(files, l.File)
		}
		return files
	}
}
