# Silence, Policy, ContactGroup, Schedule Migration Evidence

## Migration Files Created
- `database/postgresql/migrations/003_alert_policy_contact.sql` — Forward migration
- `database/postgresql/migrations/003_alert_policy_contact.rollback.sql` — Rollback script

## Tables Created

### silences (5 tables)
- **silences** — Time-bound alert suppression with matchers, status lifecycle (active/expired/pending)
- **maintenance_windows** — Scheduled maintenance periods with matchers, status (scheduled/active/ended)
- **notification_policies** — Routing policies with tenant_scope, matchers, channels, escalation_steps, schedules
- **contact_groups** — Named contact groups with member list and optional schedule reference
- **schedules** — On-call schedules with timezone, shifts, and exceptions

### Key Design Decisions
- `silences` and `maintenance_windows` use JSONB matchers for flexible label matching.
- `notification_policies` uses JSONB for channels, escalation_steps, and active_schedule — schema flexibility without join overhead.
- `contact_groups.members` is JSONB to support flexible member structures (name, email, phone, userId).
- `schedules.shifts` and `schedules.exceptions` are JSONB for flexible time window definitions.
- Unique index on `contact_groups(tenant_id, name)` and `schedules(tenant_id, name)` prevents duplicate names within a tenant.
- Indexes on `starts_at` and `ends_at` enable efficient time-range queries for active silence/window checks.

## Verification
- All 5 tables use `CREATE TABLE IF NOT EXISTS` for idempotency.
- Rollback drops all 5 tables in reverse dependency order.
- JSONB columns provide schema flexibility for future extensibility.