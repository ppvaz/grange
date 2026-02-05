# IKE v3: Institutional Knowledge Extractor

A six-stage pipeline for extracting implicit project knowledge into structured
catalogues, with human checkpoints between each autonomous agent run.

The input is any existing project — a codebase, a business model captured in a landing page, an institutional website, documentation. The output is technology-agnostic knowledge: entities, business rules, user flows, integration contracts, with confidence scores.

Stages 0-1 produce the knowledge base. Stage 2 optionally rebuilds from it.

> **Automated mode:** `reap.sh start /path/to/project` runs the full pipeline, managing stage transitions and state automatically. This document describes what each stage does and how to intervene.

## Use Cases

The knowledge catalogue is the core artifact. What you do with it is up to you:

- **Onboarding documentation** - get new developers productive faster
- **Compliance/audit trails** - document business rules and data flows
- **Architecture review** - understand system structure before changes
- **Risk assessment** - identify fragile areas and tribal knowledge gaps
- **Knowledge preservation** - capture expertise before team transitions
- **Project modernization** - rebuild with modern stack using extracted specs

---

## Philosophy

**grow.sh runs autonomously. Humans intervene between runs.**

Each stage is a complete grow.sh execution that produces artifacts and stops.
You review, add context, make decisions, then start the next stage.
No mid-flight interrupts. Clean handoffs.

---

## The Pipeline

```
STAGE 0a: LENSES                    STAGE 0b: SYNTHESIS
┌─────────────────────┐             ┌─────────────────────┐
│ • BA Lens           │             │ • Reconcile lenses  │
│ • PM Lens           │────[YOU]───▶│ • Read HUMAN-CONTEXT│
│ • QA Lens           │  review &   │ • Generate extraction│
│                     │  add context│   spec              │
└─────────────────────┘             └─────────────────────┘
         │                                    │
         ▼                                    ▼
    recon/lens-*.md                    recon/synthesis.md
                                       VISION-stage1-extraction.md
                                                │
                    ┌───────────────────────────┘
                    ▼
STAGE 1a: EXTRACTION                STAGE 1b: PROMPTS
┌─────────────────────┐             ┌─────────────────────┐
│ • Extract entities  │             │ • Generate prompts  │
│ • Extract rules     │────[YOU]───▶│ • Dependency graph  │
│ • Extract flows     │  validate   │ • Adversarial review│
│ • Extract integ.    │  low-conf   │ • Risk register     │
└─────────────────────┘             └─────────────────────┘
         │                                    │
         ▼                                    ▼
    knowledge/                         knowledge/prompts/
    CONFIDENCE-SUMMARY.md              RISK-REGISTER.md
                                       EXTRACTION-COMPLETE.md
                                                │
                    ┌───────────────────────────┘
                    ▼
STAGE 2a: BUILD                     STAGE 2b: VERIFY
┌─────────────────────┐             ┌─────────────────────┐
│ • Pre-build decisions│            │ • Run all tests     │
│ • Implement prompts │────[YOU]───▶│ • Verify flows      │
│ • Write tests       │  unblock    │ • Resolve risks     │
│ • Track gaps        │  issues     │ • Generate feedback │
└─────────────────────┘             └─────────────────────┘
         │                                    │
         ▼                                    ▼
    src/                              REBUILD-COMPLETE.md
    tests/                            KNOWLEDGE-GAPS.md
    docs/TRACEABILITY.md              (feedback to Stage 1)
```

---

## Files

| File | Stage | Purpose |
|------|-------|---------|
| `VISION-stage0a-lenses.md` | 0a | Run three analysis lenses |
| `VISION-stage0b-synthesis.md` | 0b | Reconcile lenses, generate extraction spec |
| `VISION-stage1a-extraction.md` | 1a | Extract knowledge with confidence scores |
| `VISION-stage1b-prompts.md` | 1b | Generate prompts, adversarial review |
| `VISION-stage2a-build.md` | 2a | Implement prompts, track progress |
| `VISION-stage2b-verify.md` | 2b | Verify, document, generate feedback |

---

## Full Run Walkthrough

> **Prefer `reap.sh`**: The walkthrough below shows the manual stage-by-stage process for understanding or debugging. For normal use, run `reap.sh start /path/to/project` — it handles vision file management, state tracking, and stage transitions automatically.

### Stage 0a: Lens Analysis
```bash
cd /path/to/target-project
cp /path/to/grange/visions/ike-v3/VISION-stage0a-lenses.md VISION.md
# Edit [TARGET_CODEBASE_PATH] in VISION.md
/path/to/grange/grow.sh start
# Wait for DONE.md
```

**You get:**
```
recon/
├── lens-business-analyst.md
├── lens-product-manager.md
└── lens-qa-adversarial.md
```

**Your job:**
1. Read all three lens files
2. Note contradictions
3. Create `recon/HUMAN-CONTEXT.md`:

```markdown
# Human Context

## Contradiction Resolutions

### User Roles (BA says 3, PM says 2, QA found 5 constants)
The business model is: [your explanation]
Correct interpretation: [your answer]

## Domain Knowledge
[Anything the agents couldn't infer from code alone]

## Corrections
[Any lens misunderstandings to fix]
```

---

### Stage 0b: Synthesis
```bash
rm DONE.md
cp VISION-stage0b-synthesis.md VISION.md
./grow.sh start
```

**You get:**
```
recon/
├── ...
├── HUMAN-CONTEXT.md          # Your input
├── synthesis.md              # Unified model
├── hotspot-map.md
└── complexity-assessment.md

VISION-stage1-extraction.md   # Customized for this codebase
```

**Your job:**
1. Review `VISION-stage1-extraction.md` — specific enough?
2. Check BLOCKERS.md — unresolved contradictions?
3. If good, proceed. If not, update HUMAN-CONTEXT.md and re-run 0b.

---

### Stage 1a: Deep Extraction
```bash
rm DONE.md
mv VISION-stage1-extraction.md VISION.md  # Use generated spec
./grow.sh start
```

**You get:**
```
knowledge/
├── entities/[name].md        # With confidence scores
├── rules/[category].md       # With confidence scores
├── flows/[name].md           # With confidence scores
├── integrations/[name].md    # With confidence scores
└── CONFIDENCE-SUMMARY.md
```

**Your job:**
1. Review CONFIDENCE-SUMMARY.md
2. For Low confidence items:
   - Edit the knowledge file directly to correct/validate
   - Or add context to `knowledge/HUMAN-CONTEXT.md`
3. Check BLOCKERS.md

---

### Stage 1b: Prompts & Risk Review
```bash
rm DONE.md
cp VISION-stage1b-prompts.md VISION.md
./grow.sh start
```

**You get:**
```
knowledge/
├── ...
├── prompts/
│   ├── PROMPT-001-[name].md
│   ├── PROMPT-002-[name].md
│   ├── ...
│   └── DEPENDENCY-GRAPH.md
├── RISK-REGISTER.md
└── EXTRACTION-COMPLETE.md
```

**Your job:**
1. Review RISK-REGISTER.md — High severity items
2. Review EXTRACTION-COMPLETE.md — ready for rebuild?
3. If not ready, add to HUMAN-CONTEXT.md and re-run 1b

---

### Stage 2a: Build
```bash
# New project directory
mkdir ../new-project && cd ../new-project
cp -r ../original-project/knowledge .
cp VISION-stage2a-build.md VISION.md
# Edit [TARGET STACK]
./grow.sh start
```

**You get:**
```
src/                          # Implementation
tests/                        # Derived tests
docs/
├── PRE-BUILD-DECISIONS.md
├── TRACEABILITY.md
├── DISCOVERED-GAPS.md
├── PROGRESS.md
└── implementation-logs/
```

**Your job:**
1. Review PROGRESS.md — any blocked?
2. Review DISCOVERED-GAPS.md — urgent issues?
3. If blocked, add `docs/HUMAN-DECISIONS.md` and re-run 2a
4. If complete, proceed to 2b

---

### Stage 2b: Verify
```bash
rm DONE.md
cp VISION-stage2b-verify.md VISION.md
./grow.sh start
```

**You get:**
```
docs/
├── ...
├── TEST-RESULTS.md
├── FLOW-VERIFICATION.md
├── RISK-RESOLUTION.md
├── CONFIDENCE-AUDIT.md
├── KNOWLEDGE-GAPS.md         # Feedback artifact
├── IMPROVEMENT-OPPORTUNITIES.md
├── DEVIATIONS.md
└── REBUILD-COMPLETE.md       # Final verdict
```

**Your job:**
1. Review REBUILD-COMPLETE.md — verdict acceptable?
2. If issues, fix and re-run 2b
3. If ready, ship it 🚀
4. Optionally: feed KNOWLEDGE-GAPS.md back to Stage 1 for future extractions

---

## Human Input Files

At each checkpoint, you communicate via markdown files:

| Stage | Input File | Purpose |
|-------|------------|---------|
| 0a→0b | `recon/HUMAN-CONTEXT.md` | Resolve contradictions, add domain knowledge |
| 1a→1b | `knowledge/HUMAN-CONTEXT.md` | Validate low-confidence, add missing context |
| 2a→2b | `docs/HUMAN-DECISIONS.md` | Unblock issues, make implementation decisions |

Format:
```markdown
# Human [Context|Decisions]

## [Topic]
**Issue:** [What agents flagged or got stuck on]
**Resolution:** [Your decision]
**Rationale:** [Why — helps agents understand for similar cases]

---
[Repeat for each item]
```

---

## Feedback Loop

After Stage 2b, KNOWLEDGE-GAPS.md contains improvements for extraction.

To improve the knowledge base:
1. Copy gap recommendations to appropriate knowledge/ files
2. Re-run Stage 1b to regenerate prompts with improved knowledge
3. Future rebuilds benefit from accumulated learning

```
KNOWLEDGE-GAPS.md
      │
      ▼
knowledge/rules/pricing.md  ← add missing edge case
knowledge/flows/checkout.md ← clarify ambiguous step
      │
      ▼
Re-run Stage 1b
      │
      ▼
Better prompts for next rebuild
```

---

## Deliverables Summary

| Artifact | Created In | Value |
|----------|------------|-------|
| `recon/` | 0a, 0b | Multi-lens analysis documentation |
| `knowledge/` | 1a, 1b | Technology-agnostic business logic |
| `knowledge/prompts/` | 1b | Executable rebuild specifications |
| `RISK-REGISTER.md` | 1b | Tribal knowledge made explicit |
| `src/` + `tests/` | 2a | New implementation |
| `docs/TRACEABILITY.md` | 2a | Proof of equivalence |
| `KNOWLEDGE-GAPS.md` | 2b | Extraction improvement feedback |
| `REBUILD-COMPLETE.md` | 2b | Final verification report |

---

## Quick Reference

### Automated (recommended)

```bash
./reap.sh start /path/to/project     # Full pipeline with human checkpoints
./reap.sh resume /path/to/project    # Continue after pausing
./reap.sh status /path/to/project    # Check progress
./reap.sh reset 2 /path/to/project   # Jump back to stage 2 (1a)
```

### Manual (for debugging or custom workflows)

```bash
# Stage 0a
cp VISION-stage0a-lenses.md VISION.md && ./grow.sh start
# → Review lens files → Create recon/HUMAN-CONTEXT.md

# Stage 0b
rm DONE.md && cp VISION-stage0b-synthesis.md VISION.md && ./grow.sh start
# → Review synthesis → Proceed or iterate

# Stage 1a
rm DONE.md && mv VISION-stage1-extraction.md VISION.md && ./grow.sh start
# → Review confidence → Create knowledge/HUMAN-CONTEXT.md if needed

# Stage 1b
rm DONE.md && cp VISION-stage1b-prompts.md VISION.md && ./grow.sh start
# → Review risks → Proceed or iterate

# Stage 2a (new directory)
cp VISION-stage2a-build.md VISION.md && ./grow.sh start
# → Review progress → Create docs/HUMAN-DECISIONS.md if blocked

# Stage 2b
rm DONE.md && cp VISION-stage2b-verify.md VISION.md && ./grow.sh start
# → Review verdict → Ship or iterate
```
