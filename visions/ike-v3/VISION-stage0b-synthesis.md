# Vision: IKE Stage 0b — Synthesis

## Goal
Reconcile the three lens analyses with human-provided context.
Produce unified domain model and generate the extraction specification.

## Inputs Required
```
recon/
├── lens-business-analyst.md    # From Stage 0a
├── lens-product-manager.md     # From Stage 0a
├── lens-qa-adversarial.md      # From Stage 0a
└── HUMAN-CONTEXT.md            # Human-provided (may not exist)
```

## Workspace Rules
- ALL output files go in your working directory (cwd).
- Do NOT plan beyond this stage. Only plan tasks listed below.
- Do NOT plan improvements, fixes, or features for the target project.

---

## Tasks

### 1. Reconcile Lenses
Compare all three lens outputs. For each domain concept:

**Consensus** (High Confidence)
- All three lenses agree
- Mark as ready for extraction

**Partial Agreement** (Medium Confidence)
- Two lenses agree, one differs or silent
- Note the discrepancy, proceed with majority

**Contradiction** (Needs Resolution)
- Lenses directly conflict
- Check HUMAN-CONTEXT.md for resolution
- If no resolution provided, document in BLOCKERS.md and use best judgment

Output: `recon/synthesis.md`

Structure:
```markdown
# Domain Synthesis

## Consensus (High Confidence)
[What all lenses agree on]

## Partial Agreement (Medium Confidence)
[Where 2/3 agree — note the outlier]

## Resolved Contradictions
[Conflicts resolved by HUMAN-CONTEXT.md]

## Unresolved Contradictions
[Conflicts without human input — document assumptions made]

## Unified Domain Model
[Single coherent model incorporating all of the above]
```

### 2. Incorporate Human Context
If `recon/HUMAN-CONTEXT.md` exists:
- Apply all resolutions to contradictions
- Integrate domain knowledge into unified model
- Upgrade confidence levels where human confirmed

### 3. Map Logic Hotspots
Identify where business logic concentrates in the codebase:
- Validators and sanitizers
- Service/use-case layers
- Event handlers and listeners
- State machines and workflow engines
- Database constraints and triggers
- Configuration and feature flags

Output: `recon/hotspot-map.md`

### 4. Assess Complexity
- Scale metrics (files, lines, entities, integrations)
- Coupling analysis
- Documentation gap severity
- Estimated extraction effort

Output: `recon/complexity-assessment.md`

### 5. Generate Extraction Spec
Using synthesis and hotspot map, generate the Stage 1 VISION customized for this codebase.

Output: `recon/VISION-stage1-extraction.md`

This should include:
- Specific entities to extract (from unified model)
- Specific rule categories identified
- Specific flows discovered
- Specific integrations found
- Confidence levels carried forward
- Extraction priorities ordered by business criticality

**Important:** Do NOT include an "Output Artifacts" section describing where Stage 1a
should write files. That is defined by the Stage 1a VISION template, not this spec.
This spec only describes WHAT to extract, not WHERE to put it.

---

## Output Structure
```
recon/
├── lens-business-analyst.md        # From Stage 0a
├── lens-product-manager.md         # From Stage 0a
├── lens-qa-adversarial.md          # From Stage 0a
├── HUMAN-CONTEXT.md                # Human-provided (if exists)
├── synthesis.md                    # Reconciled domain model
├── hotspot-map.md                  # Where logic lives
├── complexity-assessment.md        # Scale and effort
└── VISION-stage1-extraction.md     # Generated, customized for this codebase

BLOCKERS.md                         # Unresolved contradictions (if any)
```

---

## Done When
- [ ] recon/synthesis.md exists with unified domain model
- [ ] All contradictions either resolved or documented in BLOCKERS.md
- [ ] recon/hotspot-map.md identifies logic concentration points
- [ ] recon/complexity-assessment.md provides effort estimate
- [ ] recon/VISION-stage1-extraction.md generated with project-specific details
- [ ] All files committed

---

## Next Step (Human)
After DONE.md appears:
1. Review `recon/VISION-stage1-extraction.md` — is it specific enough?
2. Check BLOCKERS.md — any unresolved items need your input before Stage 1
3. If satisfied, proceed to Stage 1
4. If gaps remain, add to HUMAN-CONTEXT.md and re-run Stage 0b
