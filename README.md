# Evolver

An event-driven multi-agent system orchestrator for autonomous project development.

## Overview

Evolver coordinates multiple specialized AI agents that work together to transform a vision into a completed implementation. It uses file-based events and markdown documents for state management, enabling continuous self-correction and vision alignment.

## Requirements

- Bash
- `inotifywait` (inotify-tools package)
- `opencode` CLI tool

## Usage

```bash
# Start the full agent system
./evolve.sh start

# Run individual agents
./evolve.sh executor    # Execute tasks from PLAN.md
./evolve.sh planner     # Add tasks based on VISION.md
./evolve.sh critic      # Validate task alignment
./evolve.sh gap         # Check implementation against vision
./evolve.sh oracle      # Review project holistically
./evolve.sh visionary   # Analyze patterns and refine vision

# Check current state
./evolve.sh status
```

## Architecture

### Core Files

| File | Purpose |
|------|---------|
| `VISION.md` | Project goals and high-level vision |
| `PLAN.md` | Actionable task checklist |
| `BLOCKERS.md` | Obstacles preventing progress |
| `CUTS.md` | Out-of-scope tasks |
| `DRIFT.md` | Implementation deviations |
| `VISION_REVIEW.md` | Observations for vision refinement |
| `DONE.md` | Completion marker (exits system) |
| `LOG.md` | Timestamped action log |

### Agents

- **Executor** - Completes tasks from PLAN.md and commits changes
- **Planner** - Reviews vision and adds concrete next steps
- **Critic** - Validates task alignment with vision
- **Gap Finder** - Checks implementations against original intent
- **Oracle** - Reviews project holistically; signals completion
- **Visionary** - Analyzes patterns to refine the vision

### Event System

Agents are triggered by file system events rather than polling:
- PLAN.md changes trigger Executor and Critic
- VISION.md changes trigger Critic and Planner
- Git commits trigger Gap Finder, Planner, and Oracle
- Signal accumulation triggers Visionary
- Periodic heartbeat ensures forward progress

## How It Works

1. Create a `VISION.md` describing your project goals
2. Run `./evolve.sh start`
3. Agents autonomously plan, execute, review, and correct
4. System terminates when Oracle creates `DONE.md`

## Design Features

- **Event-driven**: Uses `inotifywait` for responsive triggers
- **Self-correcting**: Agents review each other's work
- **Human-readable state**: All progress in markdown files
- **Race-condition safe**: File locking for atomic operations
- **Rate-limited**: Prevents agent thrashing
- **Git-integrated**: Post-commit hooks signal reactive agents
