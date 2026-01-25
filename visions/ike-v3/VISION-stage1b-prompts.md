# Vision: IKE Stage 1b — Prompts & Adversarial Review

## Goal
Transform extracted knowledge into atomic, agent-executable prompts.
Run adversarial review to identify tribal knowledge at risk.
Human input from Stage 1a review is incorporated.

## Inputs Required
```
knowledge/
├── entities/                   # From Stage 1a
├── rules/                      # From Stage 1a
├── flows/                      # From Stage 1a
├── integrations/               # From Stage 1a
├── CONFIDENCE-SUMMARY.md       # From Stage 1a
└── HUMAN-CONTEXT.md            # Human-provided (if exists)
```

---

## Tasks

### 1. Incorporate Human Feedback
If `knowledge/HUMAN-CONTEXT.md` exists:
- Apply corrections to relevant knowledge files
- Upgrade confidence levels where human validated
- Update CONFIDENCE-SUMMARY.md

### 2. Generate Atomic Prompts
Transform knowledge into self-contained, agent-executable prompts.

Output: `knowledge/prompts/PROMPT-[NNN]-[name].md`

Order by dependency — foundational capabilities first.

Format:
```markdown
# PROMPT-[NNN]: [Capability Name]

## Goal
[One sentence]

## Domain Context
References (agent should read these first):
- Entities: [list relevant entity files]
- Rules: [list relevant rule IDs]
- Flows: [list relevant flow files]

## Inputs
[What this capability receives]

## Outputs
[What this capability produces]

## Acceptance Criteria
[Specific, testable conditions for success]
1. [Criterion]
2. [Criterion]
...

## Dependencies
[Previous prompt numbers this builds on, or "None"]

## Confidence: [High | Medium | Low]
[Inherited from source knowledge — lowest of referenced items]
```

### 3. Build Dependency Graph
Map relationships between prompts.

Output: `knowledge/prompts/DEPENDENCY-GRAPH.md`

Format:
```markdown
# Prompt Dependency Graph

## Execution Order
Prompts must be executed in an order that satisfies dependencies.

## Graph
```
PROMPT-001 (None)
    ↓
PROMPT-002 (depends: 001)
    ↓
PROMPT-003 (depends: 001, 002)
PROMPT-004 (depends: 001)
    ↓
PROMPT-005 (depends: 003, 004)
...
```

## Layers
Layer 0 (no dependencies): [list]
Layer 1 (depends on L0): [list]
Layer 2 (depends on L1): [list]
...
```

### 4. Adversarial Review
Answer: "If we rebuilt using only these prompts, what would we lose?"

Output: `knowledge/RISK-REGISTER.md`

Format:
```markdown
# Risk Register

## Tribal Knowledge
| ID | Description | Location | Severity | Notes |
|----|-------------|----------|----------|-------|
| TK-001 | [What] | [Where in code] | H/M/L | [Why it matters] |

## Undocumented Integrations
| ID | System | Purpose | Location | Severity |
|----|--------|---------|----------|----------|
| UI-001 | [What] | [Why] | [Where] | H/M/L |

## Performance Accommodations
| ID | Description | Reason | Location | Severity |
|----|-------------|--------|----------|----------|
| PA-001 | [What] | [Why it exists] | [Where] | H/M/L |

## Compliance/Legal Logic
| ID | Description | Requirement | Location | Severity |
|----|-------------|-------------|----------|----------|
| CL-001 | [What] | [Regulation/policy] | [Where] | H/M/L |

## Summary
- Total risks: X
- High severity: Y
- Requires human decision: Z
```

### 5. Finalize Extraction
Create completion summary.

Output: `knowledge/EXTRACTION-COMPLETE.md`

Format:
```markdown
# Extraction Complete

## Date
[Timestamp]

## Coverage
- Entities: X extracted
- Rules: X extracted  
- Flows: X extracted
- Integrations: X extracted
- Prompts: X generated

## Confidence Distribution
- High: X%
- Medium: Y%
- Low: Z%

## Risks Identified
- Tribal knowledge: X items
- Undocumented integrations: Y items
- Performance accommodations: Z items
- Compliance logic: W items

## Ready for Rebuild
[Yes / Yes with caveats / No — needs human review]

## Items Requiring Human Decision Before Rebuild
[List any High severity risks or Low confidence prompts]
```

---

## Output Structure
```
knowledge/
├── entities/
├── rules/
├── flows/
├── integrations/
├── prompts/
│   ├── PROMPT-001-[name].md
│   ├── PROMPT-002-[name].md
│   ├── ...
│   └── DEPENDENCY-GRAPH.md
├── CONFIDENCE-SUMMARY.md       # Updated
├── RISK-REGISTER.md            # New
└── EXTRACTION-COMPLETE.md      # New
```

---

## Done When
- [ ] Human feedback incorporated (if HUMAN-CONTEXT.md existed)
- [ ] All prompts generated with dependency references
- [ ] DEPENDENCY-GRAPH.md shows valid execution order
- [ ] Adversarial review complete, RISK-REGISTER.md populated
- [ ] EXTRACTION-COMPLETE.md summarizes coverage and readiness
- [ ] All files committed

---

## Next Step (Human)
After DONE.md appears:
1. Review RISK-REGISTER.md — decide on High severity items
2. Review EXTRACTION-COMPLETE.md — is it ready for rebuild?
3. If "No — needs human review":
   - Address listed items
   - Add decisions to `knowledge/HUMAN-CONTEXT.md`
   - Re-run Stage 1b
4. If ready, proceed to Stage 2a
