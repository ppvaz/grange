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

### 4. Async Human Review Batching

**Problem**: VISION_REVIEW.md, DRIFT.md, and CUTS.md accumulate observations that don't block execution but require eventual human attention.

**Proposition**: Generate a periodic `HUMAN_DIGEST.md` summarizing:
- New observations since last digest
- Patterns detected across files
- Suggested actions ranked by impact
- Estimated review time

**Trigger**: Daily at configured hour, or when observation count exceeds threshold.

**Trust requirement**: Low - purely informational, no autonomous action.

**Suggested milestone**: Can implement immediately as read-only aggregation.

---

## Implementation Priority

| Extension | Trust Required | Risk | Value | Priority |
|-----------|---------------|------|-------|----------|
| Async Human Review Batching | Low | Low | Medium | 1 - Safe to try |
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

*This document captures ideas for future consideration. Implementation requires demonstrated system reliability and human confidence built through extended use.*
