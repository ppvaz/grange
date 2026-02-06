# Vision: IKE Stage 2a — Pre-Build & Implementation

## Goal
Set up the new project, make pre-build decisions on risks, and implement
prompts in dependency order. Stop periodically for human review of progress.

## Target Stack
`[DEFINE YOUR TARGET STACK]`

## Inputs Required
```
knowledge/
├── entities/
├── rules/
├── flows/
├── integrations/
├── prompts/
│   ├── PROMPT-001-[name].md
│   ├── ...
│   └── DEPENDENCY-GRAPH.md
├── RISK-REGISTER.md
└── EXTRACTION-COMPLETE.md
```

## Workspace Rules
- ALL output files go in your working directory (cwd).
- Do NOT plan beyond this stage. Only plan tasks listed below.

---

## Tasks

### 1. Pre-Build Risk Decisions
Review RISK-REGISTER.md. For each risk, document handling strategy.

Output: `docs/PRE-BUILD-DECISIONS.md`

Format:
```markdown
# Pre-Build Decisions

## Risk Mitigations

### [RISK-ID]: [Description]
**Severity:** [From register]
**Strategy:** [How we'll handle it]
- [ ] Accept (document why)
- [ ] Mitigate (describe how)
- [ ] Defer (will address in Stage 2b)

**Implementation notes:**
[Specific guidance for when this comes up during build]

---
[Repeat for each risk]
```

### 2. Project Setup
Initialize project structure for target stack.

Output: Standard project skeleton with:
```
src/
├── domain/          # Will hold entities
├── rules/           # Will hold business logic
├── workflows/       # Will hold flows
├── integrations/    # Will hold adapters
└── [stack-specific]

tests/
├── unit/
├── integration/
└── contract/

docs/
├── PRE-BUILD-DECISIONS.md
└── implementation-logs/
```

### 3. Implement Prompts (Dependency Order)
Execute prompts from DEPENDENCY-GRAPH.md Layer 0 first, then Layer 1, etc.

For each prompt:

**3a. Read the prompt file**
- Understand goal, inputs, outputs, acceptance criteria
- Read referenced entities, rules, flows

**3b. Implement**
- Write code in appropriate src/ location
- Follow target stack conventions

**3c. Write tests**
- Derive test cases from acceptance criteria
- Place in appropriate tests/ location

**3d. Log implementation**

Output: `docs/implementation-logs/PROMPT-[NNN].md`

Format:
```markdown
# PROMPT-[NNN] Implementation Log

## Status: [Complete | Blocked | Deferred]

## Confidence: [From prompt]

## Implementation
- Location: `src/path/to/files`
- Approach: [Brief description of how implemented]

## Tests
- Location: `tests/path/to/files`
- Passing: [Yes | No | Partial]

## Acceptance Criteria Check
- [ ] [Criterion 1] — [Pass/Fail]
- [ ] [Criterion 2] — [Pass/Fail]
...

## Deviations from Prompt
[Any intentional differences, with rationale]
[If none: "None — implemented as specified"]

## Discovered Gaps
[Anything the prompt didn't specify that required a decision]
[If none: "None"]

## Blockers
[If blocked, what's needed]
[If none: "None"]
```

**3e. Update traceability**

Output: `docs/TRACEABILITY.md` (append/update)

Format:
```markdown
# Traceability Matrix

| Prompt | Status | Implementation | Tests | Confidence | Gaps |
|--------|--------|----------------|-------|------------|------|
| PROMPT-001 | ✅ | src/domain/user.ts | tests/unit/user.test.ts | High | None |
| PROMPT-002 | ✅ | src/rules/pricing.ts | tests/unit/pricing.test.ts | Medium | Edge case |
| PROMPT-003 | 🚧 | In progress | - | Low | - |
| PROMPT-004 | ⏸️ | Blocked | - | High | Needs clarification |
```

### 4. Track Discovered Issues
As gaps emerge during implementation:

Output: `docs/DISCOVERED-GAPS.md` (append)

Format:
```markdown
# Discovered Gaps

## GAP-[NNN]: [Title]

**Discovered during:** PROMPT-[NNN]
**Category:** [Missing Rule | Ambiguous Flow | Undocumented Integration | Other]

**Description:**
[What was unclear or missing from knowledge/]

**How resolved:**
[Decision made during implementation]

**Feedback for knowledge/:**
[What should be added to improve extraction]

---
```

---

## Output Structure
```
src/
├── domain/
├── rules/
├── workflows/
└── integrations/

tests/
├── unit/
├── integration/
└── contract/

docs/
├── PRE-BUILD-DECISIONS.md
├── TRACEABILITY.md
├── DISCOVERED-GAPS.md
├── implementation-logs/
│   ├── PROMPT-001.md
│   ├── PROMPT-002.md
│   └── ...
└── PROGRESS.md
```

### 5. Progress Summary
After each layer of prompts, update progress.

Output: `docs/PROGRESS.md`

Format:
```markdown
# Implementation Progress

## Current Layer: [N]

## Completed
- Layer 0: [X/Y prompts]
- Layer 1: [X/Y prompts]
...

## Blocked
| Prompt | Reason | Needs |
|--------|--------|-------|
| PROMPT-NNN | [Why] | [What to unblock] |

## Test Status
- Total tests: X
- Passing: Y
- Failing: Z

## Ready for Human Review
[Yes — all current layer complete | No — blocked items need input]
```

---

## Done When
- [ ] PRE-BUILD-DECISIONS.md addresses all High severity risks
- [ ] Project skeleton created
- [ ] All prompts in Layers 0-N implemented (or explicitly blocked/deferred)
- [ ] Implementation log exists for each attempted prompt
- [ ] TRACEABILITY.md up to date
- [ ] DISCOVERED-GAPS.md captures any issues found
- [ ] All tests passing (or failures documented)
- [ ] PROGRESS.md shows current state

---

## Next Step (Human)
After DONE.md appears:
1. Review PROGRESS.md — overall status
2. Review any blocked prompts — can you unblock?
3. Review DISCOVERED-GAPS.md — any need immediate attention?
4. Review test results — acceptable?
5. If blocked items exist:
   - Add resolutions to `docs/HUMAN-DECISIONS.md`
   - Re-run Stage 2a (will continue from where it left off)
6. If all prompts complete, proceed to Stage 2b
