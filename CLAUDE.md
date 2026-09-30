# Grange

Autonomous AI development toolkit using agricultural metaphors. Reap knowledge from existing projects, grow new ones from accumulated specs.

## What It Does

**Two-part lifecycle:**

1. **Reap** (`reap.sh`) - 6-stage IKE pipeline extracts implicit knowledge from existing codebases through three lenses (Business Analyst, Product Manager, QA Adversarial). Produces technology-agnostic specs with confidence scores.
2. **Grow** (`grow.sh`) - A fixed loop where 5 agent roles (Executor, Planner, Gap Finder, Oracle, Visionary) work toward a `VISION.md` contract until the Oracle's verdict says it's done (`DONE.md`). Each role can run on Claude Code, Codex, Antigravity (`agy`) or OpenCode.

**Supporting tools:**
- `harvest.sh` - Converts IKE extraction prompts into reusable spec files
- `distill.sh` - Discovers cross-project patterns from domain specs via clustering
- `digest.sh` - Compiles agent observations into human review digests
- `dashboard/` - Go web UI for monitoring agent status and decision queues

## Architecture

### Code layout

The orchestrator is a Go binary (`cmd/grange`, stdlib only). `grow.sh` and `digest.sh` are shims that run it through `lib/launch.sh`, which rebuilds `bin/grange` whenever the Go sources are newer, so symlinked projects pick up changes on the next run.

| Package | Owns |
|---------|------|
| `internal/agent` | Per-CLI argv (headless/interactive/plan), running agents in their own process group, `Ask` (answers via a file) |
| `internal/config` | `.env` loading, grow-mode presets, which backend each role uses |
| `internal/workspace` | PLAN.md tasks, `.locks/` counters and running markers (the dashboard reads these), LOG.md, git |
| `internal/grow` | The loop, gates, CI, digest, and the agent prompts (`internal/grow/prompts/*.md`, embedded) |
| `internal/reap` | The six IKE stages, `.ike-state` (plain `KEY=value`, unquoted: the dashboard reads it raw), checkpoints |

### Agent System (grow.sh)

| Agent | Role | Tier (default) |
|-------|------|----------------|
| **Executor** | Does tasks from PLAN.md, commits results | Cheap |
| **Planner** | Populates the plan, cuts misaligned tasks, adds the next one if the vision isn't covered | Cheap |
| **Gap Finder** (`Gap`) | Reviews each batch of new commits: drift, file size, tests, security, docs | Cheap |
| **Oracle** | Verifies the vision; writes a JSON verdict to `.locks/oracle-verdict.json` | Smart |
| **Visionary** | Detects systemic patterns, appends to VISION_REVIEW.md | Smart |

The loop (`internal/grow/grow.go`), one step per turn:
1. Empty plan: the Planner populates it. All tasks checked: the Oracle runs, and its verdict becomes `DONE.md` or `Fix:` tasks.
2. Otherwise wait for the gates, run the Executor, then CI. New commits: Gap Finder, then Planner (if under `MAX_PENDING_TASKS`).
3. Observation files grew by 3+ lines (or every `REFACTOR_INTERVAL` commits): Visionary. 10+ undigested lines: digest.

Three turns in a row without progress (no new commit, no task checked, no fix tasks) stop the loop with an error rather than burning tokens. Agents run one at a time; there are no watchers, locks or heartbeat.

### IKE Pipeline (reap.sh)

6 stages with human checkpoints between each. Each stage's `VISION.md` comes from `visions/ike-v3/`:
- **0a**: Three lens jobs (BA, PM, QA) in parallel, role `Lens` → `recon/lens-*.md`. The lens agents don't commit (three agents would fight over git's index lock); reap commits all three after.
- **0b**: One job does the whole stage, role `Synthesis` → `recon/synthesis.md`, `recon/VISION-stage1-extraction.md`
- **1a**: Grow loop on the extraction spec from 0b → `knowledge/`
- **1b**: Grow loop → `knowledge/prompts/`, `EXTRACTION-COMPLETE.md`
- **2a**: Grow loop in `BUILD_DIR` (asks for the target stack, or `--stack`) → `src/`, `tests/`
- **2b**: Grow loop → `docs/REBUILD-COMPLETE.md`

A stage is complete only when its required outputs exist (`internal/reap/stages.go`); for grow-loop stages the Oracle's `DONE.md` is necessary but not sufficient. After a stage, reap prompts `[Enter]/r/q` on a terminal and otherwise stops, printing the `./reap.sh resume` command. Target codebases are passed to agents as extra readable dirs (`--add-dir` where the CLI has one) and are read-only by prompt only.

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

- `.env` holds grange settings (backends, modes, gates). Each agent CLI uses its own login or API key; grange passes the environment through untouched
- `GRANGE_SMART` / `GRANGE_CHEAP` pick each tier's backend as `backend[:model][@effort]`: `claude`, `codex`, `agy` or `opencode`, e.g. `GRANGE_SMART=claude:claude-opus-5-5@high`, `GRANGE_CHEAP=codex:gpt-6.1-sol@medium` (default: `claude` for both). `@effort` maps to `--effort` (claude, agy), `-c model_reasoning_effort=` (codex) or `--variant` (opencode); only known effort words count after the last `@`
- `SMART_AGENTS` lists roles on the smart tier (default: `Oracle,Visionary`); `GRANGE_AGENT_<ROLE>` overrides one role (`EXECUTOR`, `PLANNER`, `GAP`, `ORACLE`, `VISIONARY`, `DIGEST`, `DISTILL`, `LENS`, `SYNTHESIS`)
- Model IDs in docs are dated recommendations (README "Agent backends"). Never write an ID you haven't checked against the CLI (`codex debug models`, `agy models`, `opencode models`, `claude --help`) or the vendor's docs; tests use neutral IDs like `model-a`
- Headless agents run with each CLI's auto-approve flag (`--dangerously-skip-permissions`, `--dangerously-bypass-approvals-and-sandbox`, `--auto`): they're unattended and need a shell anyway. Only point grange at code you'd let an agent loose on.
- `grow.sh start` checks every backend is on PATH before doing anything; `grange agent check` shows the role-to-backend mapping
- Values already in the environment win over `.env`

## Agile Vibe Code Configuration

Opt-in features inspired by XP + AI pair programming workflow. All backwards-compatible — defaults match original behavior.

| Variable | Default | Description |
|----------|---------|-------------|
| `GROW_MODE` | `auto` | Preset: `pair` (gates ON), `sleep` (gates OFF), `auto` (individual vars) |
| `ENABLE_CI` | `auto` | Run `ci.sh` after Executor tasks. `auto`=run if ci.sh exists, `true`=required, `false`=disabled |
| `ENABLE_APPROVAL_GATE` | `false` | Human must `touch .plan-approved` before Executor runs |
| `ENABLE_REVIEW_GATE` | `false` | Pause Executor when Visionary raises concerns. `touch .vision-reviewed` to resume |
| `ENABLE_TDD` | `true` | Executor writes failing tests first, implements to pass |
| `MAX_FILE_LINES` | `300` | Gap Finder flags files exceeding this line count for refactoring |
| `REFACTOR_INTERVAL` | `5` | Every N commits, signal Visionary to check for refactoring needs |
| `PAIR_STYLE` | `interactive` | Executor interaction in pair mode: `interactive` (human navigates in real-time) or `plan` (agent plans, human approves, then executes) |
| `PAIR_TIMEOUT` | `3600` | Session timeout for interactive Executor (seconds, default 1hr) |

**CI (`ci.sh`)**: A project-level script that runs tests, linting, security scans. Must exit 0 to pass. Keep it fast (<30s). `grange init` generates a template with commented examples for common stacks.

**Approval Gate**: When enabled, Executor won't run until `.plan-approved` exists. Workflow: Planner generates plan → human reviews PLAN.md → `touch .plan-approved` → Executor proceeds. The signal file is consumed on use.

**Gap Finder Checks** (always active):
1. Vision drift (original)
2. File size > `MAX_FILE_LINES` → adds refactoring task
3. Test existence for new source files → adds test task
4. Security review of diff → flags issues in DRIFT.md
5. Documentation — generates structured .md files for new patterns, decisions, and conventions (placed contextually: directory README.md, ARCHITECTURE.md, or CLAUDE.md)

**Review Gate**: When enabled (`ENABLE_REVIEW_GATE=true` or `GROW_MODE=pair`), the Executor pauses whenever Visionary writes to VISION_REVIEW.md. The human reviews the observations and runs `touch .vision-reviewed` to resume. Signal files `.vision-review-pending` and `.vision-reviewed` are consumed automatically.

**Interactive Pair Mode**: When `GROW_MODE=pair` and stdin is a terminal, the Executor runs as an interactive session of its backend's CLI — the human can see the agent's work in real time, interrupt to redirect, provide context, or adjust the approach. The agent pilots (writes code), the human navigates (steers direction). With `PAIR_STYLE=plan`, the Executor starts in its CLI's plan mode (`--permission-mode plan`, `agy --mode plan`, `opencode --agent plan`; Codex has none, so the prompt asks for a plan first) and the human approves before execution. After each task grange lists what's left and asks whether to continue. Without a terminal (e.g. started from the dashboard), pair mode's Executor runs headless.

**Permissions**: Headless agents always run with their CLI's auto-approve flag, in every mode; `sleep` only turns the gates off.

## Grow Modes

Presets that configure multiple variables at once. Individual `ENABLE_*` vars can still override mode defaults.

| Mode | Approval Gate | Review Gate | Startup Visionary | Executor Style | Best For |
|------|--------------|-------------|-------------------|----------------|----------|
| `auto` (default) | per ENABLE_* | per ENABLE_* | Hash-based skip | Non-interactive | Custom configs |
| `pair` | ON | ON | Always runs | Per `PAIR_STYLE` | Active development |
| `sleep` | OFF | OFF | Hash-based skip | Non-interactive | Overnight runs |

## Tests

```bash
go test ./... && tests/run.sh       # everything (runs on the host; needs Go)
tests/run.sh agent_test.sh          # one end-to-end file
```

- `go test ./...` — unit tests for the Go orchestrator (`cmd/`, `internal/`).
- `tests/run.sh` — end-to-end: each `test_*` function in `tests/*_test.sh` runs in its own bash process. `new_project` (in `tests/lib.sh`) builds a throwaway project where every agent CLI (claude, codex, agy, opencode) is a stub recording its PID/args, so tests never hit a real model. Drive the real scripts/binary rather than sourcing internals.

## Development Notes

- Remaining bash scripts use `set -euo pipefail`
- Headless agents get their own process group (`internal/agent/exec.go`); a timeout, shutdown or normal exit kills the whole group, so MCP/dev servers an agent started don't outlive it. Interactive (pair) agents share grange's group so their UI owns the terminal; Ctrl+C then goes to the agent, not grange
- Agent CLIs are found on PATH, so aliases and functions from the user's interactive shell (e.g. a zsh `claude-cheap`, or `codex` aliased with extra flags) are invisible. Grange passes its own flags per backend. `/bin/bash` is 3.2 on macOS; keep the remaining scripts compatible
- Scripts are symlinked into projects; find grange's own files via `readlink -f "$0"`, not `dirname "$0"`
- Detect a terminal with `workspace.IsTerminal` (a termios ioctl), never `os.ModeCharDevice`: `/dev/null` is a character device, and the dashboard starts grange with it as stdin. Getting this wrong makes reap skip its checkpoints and pair mode start a UI with no terminal
- Dashboard: Go binary at `dashboard/grange-dashboard`, serves on port 3000+
- Symlink architecture means toolkit updates propagate to all adopted projects automatically
