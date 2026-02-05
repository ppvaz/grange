# Entity CRUD with Ownership

> Frequency: 6/6 projects | Layer: Entity | Recommendation: always

## Business Requirement

Create, read, update, delete entities with ownership-based access control.

Entity: ENTITY_NAME has ownership field, CRUD operations with authorization, soft delete pattern, audit trail.

## What We Need

### Required Operations

- Entity record with unique ID and ownership metadata
- Access check result (allowed/denied with reason)
- Audit trail of changes

## Business Rules

- Create sets ownership to creating user
- Create validates all required fields
- Read enforces authorization (owner OR elevated role)
- Update only allowed by owner OR elevated role
- Delete sets soft delete flag, does not destroy record
- All modifications logged with timestamp and actor
- Unique identifier generated on creation

### Customization Variables

| Variable | Description | Examples |
|----------|-------------|----------|
| `$ENTITY_NAME` | Business object type | property, prescription, schedule, listing, prediction |
| `$REQUIRED_FIELDS` | Mandatory fields on create | domain-specific field list |
| `$ELEVATED_ROLES` | Roles that bypass ownership | admin, moderator, manager |
| `$SOFT_DELETE_FIELD` | Deletion marker field | removed, archived, deleted_at, status=deleted |
| `$OWNERSHIP_FIELD` | Field storing owner | createdBy, owner, userId, agentId |

## Integration Points

This feature is used by:
- Status/Workflow State Machine
- Paginated List with Filters
- Detail View with Related Content

## Data Needs

- Create: entity data + creating user
- Read: entity ID + requesting user
- Update: entity ID + updates + requesting user
- Delete: entity ID + requesting user

## Dependencies

- User Identity & Role System

## Success Criteria

- [ ] Create sets ownership to creating user
- [ ] Create validates all required fields
- [ ] Read enforces authorization (owner OR elevated role)
- [ ] Update only allowed by owner OR elevated role
- [ ] Delete sets soft delete flag, does not destroy record
- [ ] All modifications logged with timestamp and actor
- [ ] Unique identifier generated on creation

## Business Value

- Appears in 6/6 analyzed projects

| Project | Implementation |
|---------|---------------|
| E-commerce | Order with customerId ownership, admin can view all |
| Project Management | Task with assigneeId, project lead can reassign any |
| Content Platform | Article with authorId, editor/moderator can edit any |
| Booking System | Reservation with guestId, staff can manage any |
| Support System | Ticket with reporterId, support agent routes to team |
| Inventory | Asset with departmentId, operations manager configures rules |

## Reference

- Source: cross-project pattern analysis (6 projects)
- Source: cross-project pattern analysis (6 projects)
