# Spec Library

Queryable collection of business specs extracted from real projects via IKE (Institutional Knowledge Extractor). Agents reference these specs to generate more precise, acceptance-criteria-driven tasks.

## Organization

**Committed (generic):**

Only `patterns/entity-crud.md` is tracked as a reference example of the spec format.

**Gitignored (populated per-clone via harvest.sh or distill.sh):**

Everything else under `specs/` is gitignored — both domain-specific specs (sensitive client data) and the remaining pattern specs. Populate them locally by running `harvest.sh` or `distill.sh`.

## Spec Format

Domain specs use the `_TEMPLATE` format with sections: Business Requirement, What We Need, Business Rules, Data Needs, Dependencies, Success Criteria, and Business Value.

Pattern specs add: Customization Variables table and cross-project Implementation table.

## Adding Specs

### From an IKE extraction

```bash
./harvest.sh /path/to/project        # Convert IKE prompts to specs
./harvest.sh /path/to/project --dry-run  # Preview without writing
```

### Discovering cross-project patterns

```bash
./distill.sh              # Discover patterns across domain specs
./distill.sh --dry-run    # Preview clusters without writing
./distill.sh --clean      # Wipe cache and re-discover from scratch
```

Reads all domain specs, clusters by capability via LLM, and generates anonymized pattern files in `patterns/`. Skips patterns that already exist.

### Manually

Copy the format from any existing spec in `patterns/` or a domain directory.

## How Agents Use Specs

The Planner agent checks `specs/` before adding tasks. When a matching spec exists, the task references it (e.g., `- [ ] Implement specs/patterns/user-identity.md: Add auth`). The Executor then reads the referenced spec for acceptance criteria.
