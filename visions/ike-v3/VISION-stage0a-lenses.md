# Vision: IKE Stage 0a — Lens Analysis

## Goal
Run three parallel analyses of the target codebase from different perspectives.
Stop after all lenses complete. Human will review before synthesis.

## Target
`[TARGET_CODEBASE_PATH]`

---

## Tasks

### 1. Business Analyst Lens
Analyze as a **business analyst**, ignoring all technical implementation.

Extract:
- **User types** — Who uses this system? Goals and motivations?
- **Core domain objects** — Fundamental "things" in business language
- **User journeys** — Key flows from each user type's perspective
- **Business rules** — Constraints, validations, policies governing behavior
- **Value exchanges** — Where money, goods, or information flows

Output: `recon/lens-business-analyst.md`
Format: Structured domain map. No technical terms.

### 2. Product Manager Lens
Analyze as a **product manager** focused on user value.

Extract:
- **Core value proposition** — What problem does this solve? Why use this?
- **Feature hierarchy** — Essential vs nice-to-have. What breaks if removed?
- **User segments** — Different user types served differently?
- **Growth mechanics** — Acquisition, retention, monetization patterns
- **Unshipped intent** — Planned features that never launched

Output: `recon/lens-product-manager.md`
Format: Product spec, not technical document.

### 3. QA Adversarial Lens
Analyze as a **QA engineer trying to break it**.

Extract:
- **Assumption mismatches** — Code assumes X, UI says Y, errors say Z
- **Edge cases** — Boundaries, empty states, max values, permission edges
- **Dead paths** — Unreachable code, impossible states, orphaned features
- **Inconsistent logic** — Same concept handled differently in different places
- **Failure modes** — What breaks silently vs loudly?

Output: `recon/lens-qa-adversarial.md`
Format: Risk map, not feature list.

---

## Output Structure
```
recon/
├── lens-business-analyst.md
├── lens-product-manager.md
└── lens-qa-adversarial.md
```

---

## Done When
- [ ] recon/lens-business-analyst.md exists and covers all user types and journeys
- [ ] recon/lens-product-manager.md exists and covers value prop and feature hierarchy
- [ ] recon/lens-qa-adversarial.md exists and covers assumption mismatches and edge cases
- [ ] All three files are committed

---

## Next Step (Human)
After DONE.md appears:
1. Read all three lens files
2. Note where they agree (high confidence)
3. Note where they contradict (needs your input)
4. Create `recon/HUMAN-CONTEXT.md` with:
   - Resolutions for contradictions
   - Domain knowledge agents couldn't infer
   - Corrections to any lens misunderstandings
5. Proceed to Stage 0b
