# Grange

Autonomous AI development toolkit using agricultural metaphors. Reap knowledge from existing projects, grow new ones from accumulated specs.

## What It Does

**Two-part lifecycle:**

1. **Reap** (`reap.sh`) - 6-stage IKE pipeline extracts implicit knowledge from existing codebases through three lenses (Business Analyst, Product Manager, QA Adversarial). Produces technology-agnostic specs with confidence scores.
2. **Grow** (`grow.sh`) - Event-driven multi-agent system where 5 agents (Executor, Planner, Gap Finder, Oracle, Visionary) self-organize around a `VISION.md` contract until the Oracle declares it done via `DONE.md`.

**Supporting tools:**
- `harvest.sh` - Converts IKE extraction prompts into reusable spec files
- `distill.sh` - Discovers cross-project patterns from domain specs via clustering
- `digest.sh` - Compiles agent observations into human review digests
- `dashboard/` - Go web UI for monitoring agent status and decision queues

## Architecture

### Agent System (grow.sh)

| Agent | Role | API Tier |
|-------|------|----------|
| **Executor** | Does tasks from PLAN.md, commits results | Cheap (`claude-cheap`) |
| **Planner** | Adds/cuts tasks, checks alignment with vision | Cheap (`claude-cheap`) |
| **Gap Finder** | Reviews each commit for vision drift | Cheap (`claude-cheap`) |
| **Oracle** | Declares vision achieved when all checks pass | Smart (Opus) |
| **Visionary** | Detects systemic patterns, suggests refinements | Smart (Opus) |

Event-driven: file watchers (inotifywait/fswatch) trigger agents on changes to PLAN.md, VISION.md, git commits, and signal files. Heartbeat every 10 minutes as safety net.

Concurrency: `MAX_AGENTS=2`, mkdir-based atomic locking, rate limiting (`MIN_INTERVAL=30s`), debouncing (`DEBOUNCE_INTERVAL=60s`).

### IKE Pipeline (reap.sh)

6 stages with human checkpoints between each:
- **0a**: Parallel lens analysis (BA, PM, QA) → `recon/lens-*.md`
- **0b**: Synthesize lenses → `recon/synthesis.md`
- **1a**: Extract entities, rules, flows, integrations → `knowledge/`
- **1b**: Generate atomic prompts + risk review → `knowledge/prompts/`
- **2a**: Build from prompts → `src/`, `tests/`
- **2b**: Verify implementation → `REBUILD-COMPLETE.md`

## CLI

```bash
# Global (install: ln -s /path/to/grange/grange.sh /usr/local/bin/grange)
grange init <dir>        # Scaffold new project (symlinks + VISION.md + git init)
grange adopt [dir]       # Bring grange into existing project (safe, idempotent)
grange eject [dir]       # Remove grange, harvest specs to library
grange dashboard [dir]   # Launch web dashboard

# Per-project
./grow.sh start          # Launch full agent system
./grow.sh executor|planner|gap|oracle|visionary  # Run single agent
./grow.sh status         # Show vision, plan, recent log
./reap.sh start /path    # Start IKE extraction
./harvest.sh /path       # Convert IKE prompts to specs
./distill.sh             # Discover cross-project patterns
```

## Project Scaffold (after `grange init`)

```
my-project/
├── grow.sh          → symlink to grange
├── reap.sh          → symlink
├── harvest.sh       → symlink
├── distill.sh       → symlink
├── digest.sh        → symlink
├── visions/         → symlink (IKE stage templates)
├── specs/           → symlink (spec library)
├── .env             # API keys (gitignored)
├── .gitignore       # Grange entries pre-configured
└── VISION.md        # THE CONTRACT - agents work toward this
```

**Runtime files created by agents:**
- `PLAN.md` - Task checklist (`- [ ]` / `- [x]`)
- `BLOCKERS.md` - Obstacles
- `CUTS.md` - Out-of-scope cuts with reasoning
- `DRIFT.md` - Implementation deviations
- `VISION_REVIEW.md` - Visionary observations for human review
- `DONE.md` - Created by Oracle when vision is achieved
- `LOG.md` - Timestamped action log
- `.locks/` - Concurrency state

## Spec Library (`specs/`)

- `specs/patterns/` - Generic cross-domain patterns (committed). Example: `entity-crud.md`, `paginated-list.md`, `user-identity.md`
- `specs/{project}/` - Domain-specific specs (gitignored, populated by `harvest.sh`)
- Format: Business Requirement, What We Need, Business Rules, Data Needs, Dependencies, Success Criteria
- Planner references matching specs in tasks; Executor reads them for acceptance criteria

## Writing a VISION.md

The vision is the contract. It should be:
- **Specific** - What exactly are we building?
- **Measurable** - How do we know when it's done?
- **Bounded** - What's explicitly out of scope?

Structure:
```markdown
# Vision: [Project Name]

## Goal
[1-2 sentences: what this project achieves]

## What We're Building
[Concrete deliverables]

## Done When
- [ ] [Measurable criterion 1]
- [ ] [Measurable criterion 2]

## Out of Scope
- [Thing we're NOT doing]

## Tech Constraints
- [Stack/framework/language requirements]
```

## Environment

- `.env` holds API keys: `ANTHROPIC_API_KEY` (for smart agents), plus any other provider keys
- `SMART_AGENTS` env var controls which agents use Opus vs cheap model (default: `Oracle,Visionary`)
- `claude` = regular Claude CLI, `claude-cheap` = alias for cheaper model (defined in ~/.bashrc)

## Development Notes

- All bash scripts use `set -euo pipefail`
- File watching: `inotifywait` on Linux, `fswatch` on macOS
- Locking: mkdir-based atomic locks (macOS-compatible, no flock dependency)
- Dashboard: Go binary at `dashboard/grange-dashboard`, serves on port 3000+
- Symlink architecture means toolkit updates propagate to all adopted projects automatically
