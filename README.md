# Grange

A toolkit for **disciplined AI-assisted development** — autonomous agent orchestration with XP guardrails, supporting both interactive pair programming and unattended autonomous modes.

**Autonomous agent orchestration.** `grow.sh` runs five specialized AI agent roles in a fixed loop around a vision document: plan, execute, run CI, review the new commits, and verify. An Oracle decides when the vision has been achieved, and grange turns its verdict into either `DONE.md` or fix tasks. A Visionary watches for systemic problems and suggests course corrections. Each role can run on Claude Code, OpenAI Codex, Google Antigravity (`agy`) or OpenCode, on whichever model you choose. Grow modes let you run interactively with human gates, or fully autonomous overnight.

**Institutional knowledge extraction.** `reap.sh` walks the IKE pipeline — analyzing existing projects through business analyst, product manager, and QA lenses to extract the knowledge that lives in people's heads and between the lines. Not just code: business models, value propositions, user journeys, domain rules, integration contracts. Technology-agnostic knowledge that can be harvested into spec libraries and distilled into reusable patterns across projects.

Together they form a grange — the estate where you reap existing projects, harvest their specs, distill the patterns, and grow new implementations.

## The Toolkit

| Tool | Purpose |
|------|---------|
| `grow.sh` | Autonomous multi-agent orchestrator — runs agents against a vision until done |
| `reap.sh` | Walk the IKE pipeline — extract institutional knowledge from existing projects |
| `harvest.sh` | Convert extracted prompts into spec library files |
| `distill.sh` | Discover cross-project patterns from domain specs |
| `digest.sh` | Compile human review digests from agent observations |
| `dashboard/` | Web UI for agent status, gates, plan edits and the IKE pipeline |

## The Full Lifecycle

```
Existing Project (code, docs, website, business model)
      │
      ▼
  reap.sh          6-stage IKE pipeline: analyze → synthesize → extract → prompt
      │
      ▼
  harvest.sh       Convert IKE prompts into spec library files
      │
      ▼
  distill.sh       Cluster specs from multiple projects into reusable patterns
      │
      ▼
  grow.sh          Autonomous agents build a new project from specs + vision
      │
      ├── digest.sh    (called automatically to summarize agent observations)
      ▼
  New Project
```

Each tool works standalone. `grow.sh` only needs a `VISION.md` — it doesn't care where that vision came from. `distill.sh` only needs specs in the library. The lifecycle is the full story, but you can enter at any point.

## Methodology

Inspired by Akita's blog post ("Do Zero à Pós-Produção em 1 Semana") — 274 commits, 8 days, 4 apps, 1323 tests. The insight: AI pair programming works when you apply XP discipline. Grange codifies this into automated guardrails:

- **TDD-first**: Executor writes failing tests before implementation
- **CI on every commit**: `ci.sh` runs tests, linting, security scanning (~22s target)
- **Small releases**: Each commit is atomic and production-ready
- **Continuous refactoring**: Gap Finder flags files >300 lines, Visionary signals refactoring needs every N commits
- **Security as habit**: Gap Finder reviews every diff for security issues, ci.sh runs tooling (Brakeman, bandit, etc.)
- **Documentation as investment**: Gap Finder generates structured documentation for new patterns, decisions, and conventions — placed where they're most useful for humans and AI agents
- **Human judgment preserved**: Approval and review gates ensure the human remains "the adult in the room" — pair mode puts them in the driver's seat with an interactive agent session

## Quick Start

```bash
git clone https://github.com/ppvaz/grange.git
cd grange
./install.sh          # symlinks `grange` into your PATH and builds bin/grange

grange init my-app    # scaffold a new project
cd my-app
cp .env.example .env  # optional: pick agent backends and models
edit VISION.md        # define what you're building
./grow.sh start       # let agents work

# Or bring grange into an existing project:
cd ~/Projects/existing-app
grange adopt
```

## Installation

```bash
./install.sh
```

Tries `/usr/local/bin` first, falls back to `~/.local/bin`, then builds the Go orchestrator into `bin/grange`. The shims rebuild it whenever the Go sources change, so a `git pull` in grange reaches every project. Or link it manually:

```bash
chmod +x grange.sh
ln -s "$(pwd)/grange.sh" /usr/local/bin/grange
```

## grange CLI

`grange` is the global entry point. It scaffolds new projects with symlinks back to the toolkit.

```bash
grange init <dir>        # create a new project scaffold
grange adopt [dir]       # bring grange into an existing project
grange eject [dir]       # remove grange from a completed project
grange dashboard [dir]   # launch the dashboard for a project
grange grow [cmd]        # same as ./grow.sh [cmd] in the current project
grange reap <cmd>        # same as ./reap.sh <cmd>
grange agent check       # show which agent CLI and model each role uses
grange help              # show usage
```

Both `init` and `adopt` automatically launch the dashboard in the browser.

### grange init

`grange init` creates:

```
my-project/
├── grow.sh       → grange/grow.sh
├── reap.sh       → grange/reap.sh
├── harvest.sh    → grange/harvest.sh
├── distill.sh    → grange/distill.sh
├── digest.sh     → grange/digest.sh
├── visions/      → grange/visions/
├── specs/        → grange/specs/
├── .env.example  (copied)
├── .gitignore    (generated)
└── VISION.md     (placeholder)
```

Scripts and directories are symlinked so updates to the toolkit propagate automatically. All grange symlinks are gitignored — only your project code gets committed. Config files are copied since they're project-specific.

### grange adopt

`grange adopt` brings grange into an existing project directory. It creates the same symlinks and config as `init` but is safe for ongoing work:

- Skips any files/symlinks that already exist (never clobbers)
- Appends grange entries to `.gitignore` instead of overwriting
- Skips git init if already a repo
- Idempotent — safe to run multiple times

### grange eject

`grange eject` cleanly removes grange from a completed project:

- Harvests `.md` files from the project root into the grange specs library
- Removes all grange symlinks (only symlinks, never regular files)
- Removes runtime state (`.locks/`, `.ike-state`, `.git-commit-signal`, `LOG.md`)
- Removes agent working files (`PLAN.md`, `BLOCKERS.md`, `CUTS.md`, etc.)
- Cleans grange entries from `.gitignore`

## Requirements

- macOS or Linux, Bash, git
- Go 1.22+ (to build `bin/grange`; stdlib only, no modules to fetch)
- At least one agent CLI on PATH, logged in: `claude` (Claude Code), `codex` (OpenAI Codex), `agy` (Google Antigravity) or `opencode`. Grange runs the binary directly, so shell aliases and functions around it don't apply.
- `python3` for `distill.sh`

## grow.sh — Autonomous Agent Orchestration

Five agent roles take turns in a fixed loop until the vision is achieved:

| Agent | Role | When it runs |
|-------|------|--------------|
| **Executor** | Completes the most important task in PLAN.md, commits atomic changes | Every turn with pending tasks, once the gates allow |
| **Planner** | Populates an empty plan; later cuts misaligned tasks and adds the next one only if the vision isn't covered yet | Empty plan, after new commits, when the Executor stalls |
| **Gap Finder** | 5-dimensional review of new commits: drift, file size, tests, security, documentation | After every turn that produced commits |
| **Oracle** | Checks the vision is fully achieved (build, tests, boot, artifacts) and writes a JSON verdict | When every task is checked |
| **Visionary** | Watches for systemic problems, suggests course corrections in VISION_REVIEW.md | At startup, after 3+ new observation lines, every `REFACTOR_INTERVAL` commits, after repeated timeouts |

### How it works

1. Create a `VISION.md` describing what you want built
2. Run `./grow.sh start`
3. Each turn: run the Executor, then `ci.sh`, then the Gap Finder and Planner on what changed
4. When no tasks are left, the Oracle verifies. Its verdict becomes `DONE.md`, or `Fix:` tasks and another round
5. Three turns in a row with no progress stop the run with an error, rather than spending tokens in circles

### Usage

```bash
# Start the full agent system
./grow.sh start

# Run individual agents
./grow.sh executor    # Execute tasks from PLAN.md
./grow.sh planner     # Add tasks based on VISION.md (includes alignment validation)
./grow.sh gap         # Check implementation against vision
./grow.sh oracle      # Review project holistically
./grow.sh visionary   # Analyze patterns and refine vision

# Check current state
./grow.sh status
```

### Grow modes

```bash
GROW_MODE=pair ./grow.sh start                    # Interactive pair: human navigates in real-time
GROW_MODE=pair PAIR_STYLE=plan ./grow.sh start    # Plan mode: agent plans, human approves, then executes
GROW_MODE=sleep ./grow.sh start                   # Autonomous: gates OFF
GROW_MODE=auto ./grow.sh start                    # Default: read individual ENABLE_* vars
```

| Mode | Use case | Approval gate | Review gate | Executor |
|------|----------|--------------|-------------|----------|
| `pair` | Active development sessions | ON | ON | Interactive (human navigates) |
| `sleep` | Overnight autonomous runs | OFF | OFF | Headless |
| `auto` | Custom configuration | Per env var | Per env var | Headless |

In **pair mode**, the Executor runs as an interactive session of its agent CLI in your terminal — you see the agent's work in real time and can interrupt to redirect, provide context, or adjust the approach. The agent pilots, you navigate. Approval and review gates are ON (`touch .plan-approved` / `touch .vision-reviewed`, or use the dashboard), and grange asks before moving to the next task. With `PAIR_STYLE=plan`, the agent proposes a plan before changing anything (Claude Code, Antigravity and OpenCode use their native plan modes; Codex is asked to in the prompt). Without a terminal, pair mode's Executor runs headless. In **sleep mode**, all gates are off, and the Visionary skips startup validation if the vision hasn't changed.

Headless agents run with their CLI's auto-approve flag in every mode (`--dangerously-skip-permissions`, `--dangerously-bypass-approvals-and-sandbox`, `--auto`): nobody is there to answer a prompt, and the agents need a shell anyway. Point grange at code you'd let an agent loose on.

### Core files (per run)

| File | Purpose |
|------|---------|
| `VISION.md` | Project goals — the contract agents work toward |
| `PLAN.md` | Actionable task checklist |
| `BLOCKERS.md` | Obstacles preventing progress |
| `CUTS.md` | Out-of-scope tasks (removed by Planner during alignment checks) |
| `DRIFT.md` | Implementation deviations (flagged by Gap Finder) |
| `VISION_REVIEW.md` | Observations for vision refinement (from Visionary) |
| `DONE.md` | Completion marker — written from the Oracle's verdict when the vision is achieved |
| `HUMAN_DIGEST.md` | Summaries of new observations for async review (from `digest.sh`) |
| `LOG.md` | Timestamped action log |
| `.plan-approved` | Signal file: human approves plan (consumed on use) |
| `.vision-review-pending` | Signal file: Visionary raised concerns (auto-created) |
| `.vision-reviewed` | Signal file: human acknowledges review (consumed on use) |

### Agent backends

Each role runs on one of four agent CLIs, picked per tier as `backend[:model][@effort]` in `.env` (or the environment, which wins). Mixing is fine, e.g. Codex does the work and Claude verifies it:

```bash
# .env
GRANGE_SMART=claude:claude-opus-5-5@high   # Oracle, Visionary (see SMART_AGENTS)
GRANGE_CHEAP=codex:gpt-6.1-sol@medium      # Executor, Planner, Gap Finder, digest, distill, reap's jobs
GRANGE_AGENT_GAP=agy:gemini-3.8-flash-medium   # optional: one role on its own backend
```

`backend` is `claude`, `codex`, `agy` or `opencode` (default: `claude` for both tiers). Leave out `:model` or `@effort` to use the CLI's own configured default. Run `grange agent check` to see which backend and model each role will use, and whether that CLI is installed; `./grow.sh start` refuses to begin if any is missing.

Suggested tiers, checked 2026-09-30. Lineups change often, so confirm what your account has with `codex debug models`, `agy models` or `opencode models`:

| Backend | Smart tier | Cheap tier | `@effort` becomes |
|---------|-----------|------------|-------------------|
| Claude Code | `claude:claude-opus-5-5@high` | `claude:claude-sonnet-5-5` | `--effort` (low … max) |
| Codex | `codex:gpt-6-astra@high` (Pro plan; on Plus, `codex:gpt-6.1-sol@xhigh`) | `codex:gpt-6.1-sol@medium` | `-c model_reasoning_effort=…` (levels vary by model) |
| Antigravity | `agy:gemini-3.1-pro-high` | `agy:gemini-3.8-flash-medium` | `--effort`; Gemini IDs already carry it (`-low`/`-medium`/`-high`), so leave `@effort` off |
| OpenCode | a strong model from a provider you've logged into, e.g. `opencode:opencode/claude-opus-5-5` (Zen) | e.g. `opencode:opencode/gpt-6.1-sol` (Zen) | `--variant` |

- Pin full model IDs rather than aliases like `opus`: some aliases resolve to older models on Bedrock and Vertex.
- Set an effort for the cheap tier. Without one, a CLI uses its own config; a Codex config with `model_reasoning_effort = "max"` makes every cheap-tier call run at max.
- OpenCode only offers models from providers you've authenticated (`opencode auth login`); its models are `provider/model`.

Control which roles are smart with `SMART_AGENTS`:

```bash
# Default: Oracle and Visionary use the smart tier, the rest use the cheap tier
./grow.sh start

# Everything on the cheap tier
SMART_AGENTS="" ./grow.sh start

# Add Executor to the smart tier
SMART_AGENTS="Oracle,Visionary,Executor" ./grow.sh start
```

| Agent | Default Tier | Rationale |
|-------|-------------|-----------|
| **Oracle** | Smart | Critical completion decision. Wrong call = premature stop or an endless fix loop. |
| **Visionary** | Smart | Sophisticated pattern recognition across signal files. Runs infrequently. |
| **Executor** | Cheap | Runs every turn. Can iterate on mistakes. A cheaper model with more tries works. |
| **Planner** | Cheap | Runs often. Includes alignment validation. |
| **Gap Finder** | Cheap | Reviews one batch of commits at a time. Errors are recoverable. |

### Design

- **One step at a time**: a fixed loop, one agent at a time — no watchers, locks or heartbeat to go wrong
- **Self-correcting**: Agents review each other's work
- **Human-readable state**: All progress in markdown files; the dashboard reads the same files
- **Verified completion**: the Oracle's verdict is structured JSON; grange never declares done on its own
- **Stall-proof**: three turns without progress stop the run instead of burning tokens
- **Clean shutdown**: each headless agent runs in its own process group, so Ctrl+C, a timeout or the dashboard's stop button take down everything it started
- **Checkpoint/resume**: Executor saves progress on timeout, next run continues

## reap.sh — Knowledge Extraction

The IKE (Institutional Knowledge Extractor) pipeline analyzes existing projects through three lenses — business analyst, product manager, and QA adversarial — and extracts technology-agnostic knowledge: entities, business rules, user flows, and integration contracts.

The input is any project: a codebase, a landing page, an institutional website, business documentation. The output is structured knowledge with confidence scores.

### Usage

Run it from a grange work directory (`grange init extraction-dir`); the target codebase is only read.

```bash
# Start the IKE pipeline, with a human checkpoint after each stage
./reap.sh start /path/to/target-project

# Continue after reviewing (from the work directory, or pass it)
./reap.sh resume [work-dir]

# Stage 2a needs the target stack; pass it up front when there's no terminal
./reap.sh resume --stack "Node.js + TypeScript + PostgreSQL" --build-dir ../rebuild

# Check pipeline progress
./reap.sh status [work-dir]

# Make the next resume start at a specific stage
./reap.sh reset 2 [work-dir]
```

On a terminal, reap asks `[Enter] continue | [r] rerun | [q] quit` after each stage. Without one (scripts, the dashboard), it stops after each stage and prints the resume command.

### The 6 stages

| # | Stage | What happens | Human checkpoint |
|---|-------|-------------|-----------------|
| 0a | Lens Analysis | Three parallel analyses (BA, PM, QA) | Review lenses, add domain context |
| 0b | Synthesis | Reconcile lenses, generate extraction spec | Review unified model |
| 1a | Deep Extraction | Extract entities, rules, flows, integrations | Validate low-confidence items |
| 1b | Prompts | Generate atomic prompts, adversarial review | Review risk register |
| 2a | Build | Implement prompts in dependency order | Unblock issues |
| 2b | Verify | Full test suite, flow verification, final report | Ship or iterate |

Stage 0a runs its three lenses as parallel agent jobs (role `Lens`), and 0b is a single job (role `Synthesis`); stages 1a–2b are complete grow loops. A stage only counts as done when its required outputs exist, not just when an Oracle says so. `reap.sh` handles the stage transitions, vision files and state tracking, pausing for human review after each stage.

See [visions/ike-v3/](visions/ike-v3/) for detailed stage documentation, expected artifacts, and the manual walkthrough.

## harvest.sh & distill.sh — Spec Library

After reaping projects, `harvest.sh` converts IKE prompts into spec library files. After harvesting multiple projects, `distill.sh` clusters specs by capability via LLM and generates anonymized, reusable pattern files.

```bash
# Convert IKE prompts from a reaped project into specs
./harvest.sh /path/to/project
./harvest.sh /path/to/project --dry-run

# Discover patterns across all domain specs
./distill.sh
./distill.sh --dry-run
./distill.sh --clean
```

The `specs/` directory contains the accumulated library:

- **`specs/patterns/`** — Generic, cross-domain patterns. Only `entity-crud.md` is committed, as a reference for the format; the rest are gitignored and generated locally by `distill.sh`.
- **`specs/{project}/`** — Domain-specific specs (gitignored). Populated per-clone via `harvest.sh`.

The Planner agent automatically checks `specs/` and references matching specs in tasks it creates. The Executor follows those references to read acceptance criteria.

See [specs/README.md](specs/README.md) for full documentation.

## Visions

The `visions/` directory contains reusable multi-stage vision templates:

| Vision | Purpose |
|--------|---------|
| [ike-v3](visions/ike-v3/) | Institutional Knowledge Extractor — extract implicit project knowledge into structured catalogues |

Use `reap.sh` to automate the full pipeline, or copy stage files manually and run `grow.sh` yourself.

## Future Directions

The Agile Vibe Code methodology — TDD, CI gates, security reviews, documentation checks — is now built into the agent system. The `GROW_MODE` system provides the foundation for more nuanced autonomy profiles beyond the current pair/sleep/auto presets.

See [PROPOSITION.md](PROPOSITION.md) for draft ideas on extending agent autonomy (auto-resolution of common blockers, threshold-based vision amendments, extended Oracle authority).
