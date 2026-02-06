# Vision: IKE Stage 1a — Deep Extraction

## Goal
Extract all business logic from the codebase into structured, confidence-scored
knowledge artifacts. Stop after extraction. Human will review before prompt generation.

## Inputs Required
```
recon/
├── synthesis.md                # Unified domain model
├── hotspot-map.md              # Where to focus
└── HUMAN-CONTEXT.md            # Human domain knowledge (if exists)

VISION-stage1-extraction.md     # Project-specific extraction spec (from Stage 0b)
```

Read VISION-stage1-extraction.md first — it contains project-specific entities,
rules, flows, and integrations to extract.

## Workspace Rules
- ALL output files go in your working directory (cwd).
- Do NOT plan beyond this stage. Only plan tasks listed below.
- Do NOT plan improvements, fixes, or features for the target project.

---

## Tasks

### 1. Extract Entities
For each domain object identified in synthesis.md:

Output: `knowledge/entities/[entity-name].md`

Format:
```markdown
# Entity: [Name]

## Business Purpose
[Why this exists in plain language]

## Properties
| Property | Type | Constraints | Business Meaning |
|----------|------|-------------|------------------|

## Invariants
[What must ALWAYS be true]

## Lifecycle States
[If applicable: states and valid transitions]

## Relationships
[Connections to other entities]

## Source Locations
[File paths in legacy code]

## Confidence: [High | Medium | Low]
[Inherited from synthesis.md consensus level]
```

### 2. Extract Business Rules
For each rule governing behavior:

Output: `knowledge/rules/[category].md` (group related rules)

Format for each rule:
```markdown
## RULE-[CAT]-[NNN]: [Name]

### Plain English
[One sentence a business person would understand]

### Trigger Conditions
[When does this rule apply?]

### Logic
[What happens, step by step]

### Exceptions
[Edge cases, overrides, special conditions]

### Error Behavior
[What happens on violation?]

### Source Locations
[File paths]

### Confidence: [High | Medium | Low]
```

### 3. Extract Flows
For each user journey or workflow:

Output: `knowledge/flows/[flow-name].md`

Format:
```markdown
# Flow: [Name]

## Business Purpose
[What user goal does this accomplish?]

## Actors
[Who/what initiates and participates]

## Preconditions
[What must be true before flow starts]

## Steps
1. [Step]
2. [Decision point: if X then Y else Z]
3. ...

## State Transitions
[Before → After for each step if applicable]

## Side Effects
[Notifications, logging, external calls]

## Error Paths
[What happens when things go wrong]

## Source Locations
[File paths]

## Confidence: [High | Medium | Low]
```

### 4. Extract Integrations
For each external system:

Output: `knowledge/integrations/[system-name].md`

Format:
```markdown
# Integration: [System Name]

## Purpose
[Why we integrate]

## Protocol
[REST, GraphQL, webhook, file, etc.]

## Authentication
[Method, credential handling]

## Endpoints/Contracts
[Request/response shapes]

## Error Handling
[Retry logic, fallbacks, failure modes]

## Rate Limits / Timeouts
[Constraints]

## Source Locations
[File paths]

## Confidence: [High | Medium | Low]
```

### 5. Generate Confidence Summary
Aggregate confidence levels across all extracted knowledge.

Output: `knowledge/CONFIDENCE-SUMMARY.md`

Format:
```markdown
# Confidence Summary

| Area | High | Medium | Low | Total |
|------|------|--------|-----|-------|
| Entities | X | Y | Z | N |
| Rules | X | Y | Z | N |
| Flows | X | Y | Z | N |
| Integrations | X | Y | Z | N |

## Items Needing Validation
[List all Low confidence items with brief reason]
```

---

## Output Structure
```
knowledge/
├── README.md                    # Overview and glossary
├── entities/
│   └── [entity-name].md
├── rules/
│   └── [category].md
├── flows/
│   └── [flow-name].md
├── integrations/
│   └── [system-name].md
└── CONFIDENCE-SUMMARY.md
```

---

## Done When
- [ ] All entities from synthesis.md extracted with confidence scores
- [ ] All business rules extracted and categorized
- [ ] All user flows documented with error paths
- [ ] All external integrations documented with contracts
- [ ] knowledge/README.md provides overview and glossary
- [ ] CONFIDENCE-SUMMARY.md shows extraction coverage
- [ ] Low confidence items listed for human review
- [ ] All files committed

---

## Next Step (Human)
After DONE.md appears:
1. Review CONFIDENCE-SUMMARY.md
2. For each Low confidence item:
   - Validate or correct in the knowledge file directly
   - Or add context to `knowledge/HUMAN-CONTEXT.md`
3. Review any BLOCKERS.md items
4. Proceed to Stage 1b when satisfied
