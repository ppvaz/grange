# Vision: IKE Stage 2b — Verification & Completion

## Goal
Verify functional equivalence, run adversarial checks, finalize documentation,
and produce feedback artifacts for improving future extractions.

## Inputs Required
```
src/                            # Implementation from Stage 2a
tests/                          # Tests from Stage 2a
docs/
├── PRE-BUILD-DECISIONS.md
├── TRACEABILITY.md
├── DISCOVERED-GAPS.md
├── PROGRESS.md
├── implementation-logs/
└── HUMAN-DECISIONS.md          # If exists

knowledge/                      # Original extraction
├── flows/
├── rules/
├── RISK-REGISTER.md
└── CONFIDENCE-SUMMARY.md
```

## Workspace Rules
- ALL output files go in your working directory (cwd).
- Do NOT plan beyond this stage. Only plan tasks listed below.

---

## Tasks

### 1. Full Test Suite
Run all tests and document results.

Output: `docs/TEST-RESULTS.md`

Format:
```markdown
# Test Results

## Summary
- **Date:** [Timestamp]
- **Total tests:** X
- **Passing:** Y (Z%)
- **Failing:** W

## By Category
| Category | Total | Pass | Fail |
|----------|-------|------|------|
| Unit | X | Y | Z |
| Integration | X | Y | Z |
| Contract | X | Y | Z |

## Failing Tests
| Test | Location | Reason | Severity |
|------|----------|--------|----------|
| [Name] | [Path] | [Why failing] | H/M/L |

## Coverage
[If coverage tooling available]
- Lines: X%
- Branches: Y%
- Functions: Z%
```

### 2. Flow Verification
For each flow in `knowledge/flows/`, verify implementation matches.

Output: `docs/FLOW-VERIFICATION.md`

Format:
```markdown
# Flow Verification

## [Flow Name]
**Knowledge file:** knowledge/flows/[name].md
**Implementation:** src/workflows/[name]

### Happy Path
- [ ] All steps implemented
- [ ] Matches documented sequence
- [ ] Produces expected outputs

### Error Paths
- [ ] [Error path 1] — implemented
- [ ] [Error path 2] — implemented
...

### Edge Cases (from QA lens)
- [ ] [Edge case 1] — handled
- [ ] [Edge case 2] — handled
...

### Verdict: [Pass | Fail | Partial]
**Notes:** [Any deviations or concerns]

---
[Repeat for each flow]
```

### 3. Risk Register Resolution
Verify all risks from RISK-REGISTER.md are addressed.

Output: `docs/RISK-RESOLUTION.md`

Format:
```markdown
# Risk Resolution

| Risk ID | Description | Strategy | Resolution | Status |
|---------|-------------|----------|------------|--------|
| TK-001 | [Tribal knowledge] | [From PRE-BUILD] | [How resolved] | ✅/⚠️/❌ |
| UI-001 | [Undocumented integration] | [Strategy] | [Resolution] | ✅/⚠️/❌ |
...

## Summary
- Resolved: X
- Accepted (documented): Y
- Outstanding: Z

## Outstanding Risks
[Detail any unresolved risks and why they're acceptable or what's needed]
```

### 4. Confidence Audit
Review handling of Low confidence items.

Output: `docs/CONFIDENCE-AUDIT.md`

Format:
```markdown
# Low Confidence Item Audit

| Source | Item | Confidence | Implementation | Validated? |
|--------|------|------------|----------------|------------|
| PROMPT-003 | [Description] | Low | [Where] | Yes/No/Deferred |

## Validation Methods
- Human confirmed: X items
- Feature flagged: Y items
- Deferred (post-launch): Z items

## Items Still Uncertain
[List any Low confidence items that remain unvalidated]
```

### 5. Generate Feedback Artifacts

**5a. Knowledge Gaps**
Consolidate gaps for improving Stage 1.

Output: `docs/KNOWLEDGE-GAPS.md`

Format:
```markdown
# Knowledge Gaps

Feedback for improving extraction in future runs.

## Missing Rules
| ID | Description | Discovered During | Recommended Addition |
|----|-------------|-------------------|---------------------|
| MR-001 | [What was missing] | PROMPT-NNN | [What to add to knowledge/rules/] |

## Ambiguous Flows
| ID | Flow | Ambiguity | Resolution Used | Recommended Clarification |
|----|------|-----------|-----------------|--------------------------|

## Undocumented Integrations
| ID | System | What Was Missing | Recommended Addition |
|----|--------|------------------|---------------------|

## Other
[Any other feedback]
```

**5b. Improvement Opportunities**
Where rebuild could exceed original.

Output: `docs/IMPROVEMENT-OPPORTUNITIES.md`

Format:
```markdown
# Improvement Opportunities

Where the new implementation could be better than the legacy system.

## [Opportunity Title]
**Original behavior:** [What legacy did]
**Problem:** [Why suboptimal]
**Proposed improvement:** [What new system could do]
**Risk:** [What could go wrong]
**Decision:** [Implement | Defer | Reject] — [Rationale]

---
[Repeat for each opportunity]
```

**5c. Deviations Log**
All intentional differences from knowledge/.

Output: `docs/DEVIATIONS.md`

Format:
```markdown
# Intentional Deviations

Differences between knowledge/ spec and actual implementation.

| Prompt | Knowledge Spec | Actual Implementation | Rationale |
|--------|----------------|----------------------|-----------|
| PROMPT-NNN | [What spec said] | [What we did] | [Why] |
```

### 6. Final Summary
Create completion report.

Output: `docs/REBUILD-COMPLETE.md`

Format:
```markdown
# Rebuild Complete

## Date
[Timestamp]

## Summary
- **Prompts implemented:** X / Y
- **Tests passing:** X / Y (Z%)
- **Flows verified:** X / Y
- **Risks resolved:** X / Y

## Confidence Distribution (Final)
- High confidence: X%
- Medium confidence: Y%
- Low confidence (validated): Z%
- Low confidence (deferred): W%

## Outstanding Items
[Any deferred work, known issues, or accepted risks]

## Feedback Generated
- Knowledge gaps: X items (see KNOWLEDGE-GAPS.md)
- Improvement opportunities: Y items (see IMPROVEMENT-OPPORTUNITIES.md)
- Deviations documented: Z items (see DEVIATIONS.md)

## Verdict
[Ready for production | Ready with caveats | Needs more work]

## Next Steps
[Recommendations for post-rebuild actions]
```

---

## Output Structure
```
docs/
├── PRE-BUILD-DECISIONS.md      # From Stage 2a
├── TRACEABILITY.md             # From Stage 2a
├── DISCOVERED-GAPS.md          # From Stage 2a
├── PROGRESS.md                 # From Stage 2a
├── implementation-logs/        # From Stage 2a
├── TEST-RESULTS.md             # New
├── FLOW-VERIFICATION.md        # New
├── RISK-RESOLUTION.md          # New
├── CONFIDENCE-AUDIT.md         # New
├── KNOWLEDGE-GAPS.md           # New (feedback artifact)
├── IMPROVEMENT-OPPORTUNITIES.md # New (feedback artifact)
├── DEVIATIONS.md               # New
└── REBUILD-COMPLETE.md         # New (final summary)
```

---

## Done When
- [ ] All tests pass (or failures documented and accepted)
- [ ] All flows verified against knowledge/flows/
- [ ] All risks from RISK-REGISTER.md resolved or explicitly accepted
- [ ] Low confidence items audited
- [ ] KNOWLEDGE-GAPS.md ready to feed back to Stage 1
- [ ] IMPROVEMENT-OPPORTUNITIES.md captures enhancement ideas
- [ ] DEVIATIONS.md documents all intentional differences
- [ ] REBUILD-COMPLETE.md provides final summary
- [ ] All files committed

---

## Next Step (Human)
After DONE.md appears:
1. Review REBUILD-COMPLETE.md — is verdict acceptable?
2. Review TEST-RESULTS.md — any concerning failures?
3. Review RISK-RESOLUTION.md — any outstanding risks unacceptable?
4. If not ready:
   - Address issues
   - Re-run Stage 2b
5. If ready:
   - Archive knowledge/ with the new codebase
   - Optionally: feed KNOWLEDGE-GAPS.md back into Stage 1 for improved extraction
   - Ship it 🚀
