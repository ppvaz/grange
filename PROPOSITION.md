# Proposition: Autonomy Extensions

> **Status**: Draft - requires refinement and battle-testing before consideration
> **Context**: Observations from overnight brute-force runs suggest the system can grind through iterations autonomously, but human judgment remains the bottleneck for direction-setting and quality assessment.

---

## Current Human Dependencies

| Dependency | Type | Frequency |
|------------|------|-----------|
| Initial VISION.md authoring | Hard | Once per project |
| Blocker resolution (BLOCKERS.md) | Hard | Variable, often daily |
| IKE stage transitions | Hard | Per-stage checkpoint |
| VISION_REVIEW.md observations | Soft | Periodic review |
| Low-confidence knowledge validation | Soft | As flagged |

---

## Proposed Extensions

### 1. Auto-Resolution for Common Blockers

**Problem**: Executor logs blockers that have known solutions (missing env vars, standard dependency issues, common configuration gaps).

**Proposition**: Maintain a `RESOLUTIONS.md` catalog mapping blocker patterns to automatic remediation actions.

```markdown
# RESOLUTIONS.md

## Pattern: "missing environment variable *_API_KEY"
Action: Check .env.example, copy missing key with placeholder, log warning

## Pattern: "dependency not installed"
Action: Run package manager install, retry task

## Pattern: "port already in use"
Action: Kill stale process or increment port, retry
```

**Trust requirement**: High - autonomous system changes are risky. Needs extensive logging and rollback capability.

**Suggested milestone**: 50+ successful manual blocker resolutions logged with patterns before considering automation.

---

### 2. Threshold-Based Vision Amendments

**Problem**: Visionary detects patterns but only appends to VISION_REVIEW.md. Human must manually update VISION.md, creating delay.

**Proposition**: Allow Visionary to make minor scope adjustments directly when:
- Confidence is high (pattern seen 5+ times)
- Impact is low (removing a sub-feature, not a core goal)
- Change is additive to CUTS.md (already sanctioned removals)

**Safeguards**:
- All auto-amendments logged to `VISION_AMENDMENTS.md` with rationale
- Human can revert by editing VISION.md (system re-reads on change)
- Kill switch: `VISIONARY_READONLY=true` in environment

**Trust requirement**: Very high - vision is the sacred contract. Needs proven Visionary judgment over many cycles.

**Suggested milestone**: 20+ VISION_REVIEW.md observations that human later adopted verbatim.

---

### 3. Extended Oracle Authority

**Problem**: Oracle adds fix tasks for every test failure, even known issues or flaky tests. This can create churn.

**Proposition**: Allow Oracle to categorize failures:
- **Fix**: Add task (current behavior)
- **Known**: Log to `KNOWN_ISSUES.md`, continue without task
- **Flaky**: Retry once, then log if still failing

**Implementation**:
```markdown
# KNOWN_ISSUES.md

## test_legacy_integration
Status: Known flaky
Reason: External API rate limiting
Action: Skip in CI, track separately
```

**Trust requirement**: Medium - test failures are recoverable, but ignoring them risks drift.

**Suggested milestone**: Manual triage of 30+ test failures with documented patterns.

---

### 4. Async Human Review Batching — IMPLEMENTED

> **Status**: Implemented as `digest.sh`. Called automatically by `grow.sh` when observation files accumulate 10+ new lines (BLOCKERS.md, CUTS.md, DRIFT.md, VISION_REVIEW.md). Compiles new entries into `HUMAN_DIGEST.md` via `claude-cheap`.

**Problem**: VISION_REVIEW.md, DRIFT.md, and CUTS.md accumulate observations that don't block execution but require eventual human attention.

**Proposition**: Generate a periodic `HUMAN_DIGEST.md` summarizing:
- New observations since last digest
- Patterns detected across files
- Suggested actions ranked by impact
- Estimated review time

**Trigger**: ~~Daily at configured hour, or~~ when observation count exceeds threshold (10+ new lines).

**Trust requirement**: Low - purely informational, no autonomous action.

---

## Implementation Priority

| Extension | Trust Required | Risk | Value | Priority |
|-----------|---------------|------|-------|----------|
| ~~Async Human Review Batching~~ | ~~Low~~ | ~~Low~~ | ~~Medium~~ | Done (`digest.sh`) |
| Extended Oracle Authority | Medium | Medium | Medium | 2 - After test triage data |
| Auto-Resolution for Blockers | High | High | High | 3 - After pattern catalog |
| Vision Amendments | Very High | Very High | Medium | 4 - Long-term goal |

---

## Validation Criteria

Before implementing any extension:

1. **Logging**: Every autonomous decision must be logged with full context
2. **Reversibility**: Human must be able to undo any automated change
3. **Kill switch**: Environment variable to disable the extension
4. **Metrics**: Track success/failure rate of autonomous decisions
5. **Thresholds**: Define clear "stop if X failures" circuit breakers

---

## Open Questions

- How do we measure "trust" quantitatively?
- What's the acceptable error rate for autonomous decisions?
- Should extensions be project-specific or global?
- How do we handle cascading failures from bad autonomous decisions?

---

## Proposition: Dashboard

> **Status**: Discussion — clarifying whether this is fundamental or cosmetic.

### The bottleneck the CLI can't solve

The system runs autonomously. What's slow is the human parts:

- Noticing a stage finished (you're watching a terminal or you miss it)
- Reviewing artifacts across multiple files to make a decision
- Acting on VISION_REVIEW.md observations (they pile up, get read later)
- Understanding at a glance: what are my agents doing, what's blocked, what needs me?

A dashboard that solves *those* problems adds fundamental value. One that just renders markdown prettily does not.

### What a fundamental dashboard would do

| Panel | What it shows | Why it matters |
|-------|--------------|---------------|
| **Agent status** | Running, queued, rate-limited, timed out | Currently inferred from LOG.md and .locks/ |
| **Decision queue** | VISION_REVIEW.md observations, BLOCKERS.md items, checkpoint prompts | Surfaces what needs you — the actual bottleneck |
| **IKE pipeline** | Stage progress, artifacts produced, confidence heatmap | `.ike-state` is invisible unless you check |
| **Spec library** | Projects reaped, specs harvested, patterns distilled, coverage | The knowledge estate at a glance |
| **Run history** | Past grow.sh runs, outcomes, time-to-DONE | No way to see this today — LOG.md gets rotated |

### What it would NOT need

- A code editor (you have one)
- A way to write VISION.md (that's a thinking activity, not a UI activity)
- Real-time log streaming (terminal does this fine)
- Agent configuration UI (env vars work)

### Implementation

Lightweight — a local web server that reads the same markdown files and `.locks/` state. No database needed. The filesystem IS the database. Single binary that serves on `localhost:3000`.

### Verdict

**Fundamental if running multiple projects or wanting to act on checkpoints faster. Just a plus if running one project at a time from the terminal.**

The dashboard becomes essential in the consulting accelerator model (see Monetization below) — managing multiple active extractions and a growing spec library.

---

## Proposition: Landing Page

> **Status**: Discussion — depends on audience and monetization.

### When the README is enough

If Grange stays open-source and personal, the README is the landing page. The agricultural metaphor lands, the two-idea structure works, the lifecycle diagram tells the story.

### When a landing page earns its keep

If there's something to sell or sign up for. The story writes itself:

```
Hero:     "Your projects know more than your documentation says."

Problem:  Institutional knowledge lives in code, in people's heads,
          in the gap between what's documented and what's real.
          When people leave, projects age, or you need to modernize —
          that knowledge is the first casualty.

Solution: Grange extracts it, structures it, finds the patterns,
          and uses them to build.

How:      reap → harvest → distill → grow

Proof:    "149 specs extracted from 6 projects.
           17 cross-domain patterns discovered automatically.
           Technology-agnostic business knowledge with confidence scores."
```

The visual language designs itself — the agricultural metaphor gives you seasons, growth cycles, harvests, estates. Concrete and memorable in a space (AI agents, knowledge management) drowning in abstract jargon.

### Verdict

A landing page without something to sell is a blog post. Build it when the monetization model is clear.

---

## Proposition: Monetization

> **Status**: Discussion — evaluating angles.

### What's actually valuable (ranked)

1. **The IKE extraction pipeline.** Multi-lens project analysis with confidence scoring, generating technology-agnostic specs. Companies doing modernization, M&A due diligence, compliance documentation, or "we inherited this project and nobody knows how it works" — they'd pay for this. The output (knowledge catalogue) is tangible and immediately useful.

2. **The accumulated pattern library.** 17 patterns from 6 projects today. This compounds. Every project reaped makes the library more valuable. Cross-domain patterns are reusable IP.

3. **The agent orchestration framework.** Interesting technically but hard to monetize directly — it's a framework, not a product. People would use it, not buy it.

### Models

| Model | Fit | Honest take |
|-------|-----|-------------|
| **Consulting accelerator** | High | Use Grange to deliver modernization/documentation projects faster. Sell the service, not the tool. The tool is the moat. |
| **Extraction-as-a-service** | Medium | "Point us at your project, get a knowledge catalogue." Productized service. Dashboard becomes client-facing. |
| **Open core** | Medium | CLI is free, dashboard + collaboration + hosted runs are paid. Needs a user base first. |
| **SaaS platform** | Low (today) | Too early. Needs hosted infra, multi-tenancy, auth, billing. |
| **Personal power tool** | High | Compound your own spec library across every project. The value is in what you accumulate, not in selling the tool. |

### Recommended path

**Near-term: consulting accelerator + personal knowledge compound.** Use Grange on every project. Spec library grows. Patterns get better. Deliver faster because you've seen the patterns before. The tool is your competitive advantage, not your product.

**Mid-term: extraction-as-a-service.** When the spec library has 50+ projects and the patterns speak for themselves, productize the extraction. "Send us your project, we send you a knowledge catalogue." Dashboard becomes client-facing.

**The dashboard decision follows from this:** build it for the operator (you) first. Agent status + decision queue + spec library overview. If extraction-as-a-service happens, extend it for clients.

---

*This document captures ideas for future consideration. Implementation requires demonstrated system reliability and human confidence built through extended use.*
