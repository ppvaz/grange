# Grange

Two big ideas, one toolkit.

**Autonomous agent orchestration.** `grow.sh` is an event-driven system where six specialized AI agents self-organize around a vision document. They plan, execute, review each other's work, detect drift, and course-correct — reacting to file changes rather than running on timers. An Oracle agent decides when the vision has been achieved. A Visionary agent watches for systemic problems and triggers course corrections. The system runs unattended until the job is done or a human intervenes.

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

## Quick Start

```bash
git clone https://github.com/ppvaz/grange.git
cd grange
./install.sh          # symlinks `grange` into your PATH

grange init my-app    # scaffold a new project
cd my-app
cp .env.example .env  # add your API keys
edit VISION.md        # define what you're building
./grow.sh start       # let agents work
```

## Installation

```bash
./install.sh
```

Tries `/usr/local/bin` first, falls back to `~/.local/bin`. Or do it manually:

```bash
chmod +x grange.sh
ln -s "$(pwd)/grange.sh" /usr/local/bin/grange
```

## grange CLI

`grange` is the global entry point. It scaffolds new projects with symlinks back to the toolkit.

```bash
grange init <dir>     # create a new project scaffold
grange help           # show usage
```

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

Scripts are symlinked so updates to the toolkit propagate automatically. Config files are copied since they're project-specific.

## Requirements

- Bash
- `claude` CLI
- `inotifywait` (Linux) or `fswatch` (macOS) for file watching
- Z.ai API key (for GLM-4.7 iterative agents) — see `.env.example`

## grow.sh — Autonomous Agent Orchestration

Six agents react to file system events, self-correct, and converge on a vision:

| Agent | Role | Trigger |
|-------|------|---------|
| **Executor** | Completes tasks from PLAN.md, commits changes | PLAN.md changes, git commits, heartbeat |
| **Planner** | Reviews vision, adds concrete next steps | VISION.md changes, git commits |
| **Critic** | Validates task alignment, removes what doesn't fit | PLAN.md and VISION.md changes |
| **Gap Finder** | Checks each commit against the vision for drift | Git commits |
| **Oracle** | Decides when the vision is fully achieved | All tasks complete |
| **Visionary** | Watches for systemic problems, suggests course corrections | Signal accumulation (3+ observations) |

### How it works

1. Create a `VISION.md` describing what you want built
2. Run `./grow.sh start`
3. Agents autonomously plan, execute, review, and correct
4. System terminates when Oracle creates `DONE.md`

### Usage

```bash
# Start the full agent system
./grow.sh start

# Run individual agents
./grow.sh executor    # Execute tasks from PLAN.md
./grow.sh planner     # Add tasks based on VISION.md
./grow.sh critic      # Validate task alignment
./grow.sh gap         # Check implementation against vision
./grow.sh oracle      # Review project holistically
./grow.sh visionary   # Analyze patterns and refine vision

# Check current state
./grow.sh status
```

### Core files (per run)

| File | Purpose |
|------|---------|
| `VISION.md` | Project goals — the contract agents work toward |
| `PLAN.md` | Actionable task checklist |
| `BLOCKERS.md` | Obstacles preventing progress |
| `CUTS.md` | Out-of-scope tasks (removed by Critic) |
| `DRIFT.md` | Implementation deviations (flagged by Gap Finder) |
| `VISION_REVIEW.md` | Observations for vision refinement (from Visionary) |
| `DONE.md` | Completion marker — Oracle creates this when the vision is achieved |
| `LOG.md` | Timestamped action log |

### Dual-API routing

All agents use the `claude` CLI. By default, high-stakes agents route through native Anthropic (Opus 4.5) while iterative agents route through Z.ai (GLM-4.7) for cost efficiency. Control with `SMART_AGENTS`:

```bash
# Default: Oracle and Visionary use Opus, rest use GLM
./grow.sh start

# Route all agents through Z.ai (GLM-only, no Anthropic subscription needed)
SMART_AGENTS="" ./grow.sh start

# Add Executor to the Opus tier
SMART_AGENTS="Oracle,Visionary,Executor" ./grow.sh start
```

| Agent | Default Tier | Rationale |
|-------|-------------|-----------|
| **Oracle** | Opus | Critical completion decision. Wrong call = premature stop or infinite loop. |
| **Visionary** | Opus | Sophisticated pattern recognition across signal files. Runs infrequently. |
| **Executor** | GLM | Runs frequently. Can iterate on mistakes. Cheaper model with more tries works. |
| **Planner** | GLM | Runs frequently. Tasks get validated by Critic anyway. |
| **Critic** | GLM | Simple alignment checks. High frequency, low complexity. |
| **Gap Finder** | GLM | Reviews one commit at a time. Errors are recoverable. |

### Design

- **Event-driven**: `inotifywait`/`fswatch` triggers, not polling
- **Self-correcting**: Agents review each other's work
- **Human-readable state**: All progress in markdown files
- **Race-condition safe**: mkdir-based locking, atomic counters
- **Rate-limited**: Prevents agent thrashing (configurable debounce)
- **Git-integrated**: Post-commit hooks signal reactive agents
- **Checkpoint/resume**: Executor saves progress on timeout, next run continues

## reap.sh — Knowledge Extraction

The IKE (Institutional Knowledge Extractor) pipeline analyzes existing projects through three lenses — business analyst, product manager, and QA adversarial — and extracts technology-agnostic knowledge: entities, business rules, user flows, and integration contracts.

The input is any project: a codebase, a landing page, an institutional website, business documentation. The output is structured knowledge with confidence scores.

### Usage

```bash
# Run the full IKE pipeline with human checkpoints between stages
./reap.sh start /path/to/target-project

# Resume after pausing
./reap.sh resume /path/to/target-project

# Check pipeline progress
./reap.sh status /path/to/target-project

# Jump back to a specific stage
./reap.sh reset 2 /path/to/target-project
```

### The 6 stages

| # | Stage | What happens | Human checkpoint |
|---|-------|-------------|-----------------|
| 0a | Lens Analysis | Three parallel analyses (BA, PM, QA) | Review lenses, add domain context |
| 0b | Synthesis | Reconcile lenses, generate extraction spec | Review unified model |
| 1a | Deep Extraction | Extract entities, rules, flows, integrations | Validate low-confidence items |
| 1b | Prompts | Generate atomic prompts, adversarial review | Review risk register |
| 2a | Build | Implement prompts in dependency order | Unblock issues |
| 2b | Verify | Full test suite, flow verification, final report | Ship or iterate |

Each stage is a complete `grow.sh` run. `reap.sh` automates the stage transitions, vision file management, and state tracking — pausing for human review between each stage.

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

- **`specs/patterns/`** — 17 generic, cross-domain patterns (committed). Extracted from a 6-project analysis covering auth, CRUD, workflows, notifications, dashboards, and more.
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

See [PROPOSITION.md](PROPOSITION.md) for draft ideas on extending agent autonomy (auto-resolution of common blockers, threshold-based vision amendments, extended Oracle authority).
