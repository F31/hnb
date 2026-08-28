# AlertRule, AlertInstance, and State Audit Migration Evidence

## Migration Files Created
- `database/postgresql/migrations/002_alert_notification_core.sql` — Forward migration
- `database/postgresql/migrations/002_alert_notification_core.rollback.sql` — Rollback script

## Tables Created

### alert_rules
| Column | Type | Constraints |
|--------|------|-------------|
| id | UUID | PK, auto-generated |
| tenant_scope | TEXT | CHECK (global, tenant) |
| source_type | TEXT | NOT NULL |
| severity | TEXT | CHECK (critical, warning, info) |
| expression_ref | TEXT | nullable |
| labels | JSONB | default {} |
| annotations | JSONB | default {} |
| enabled | BOOLEAN | default true |
| version | INTEGER | default 1 |
| created_at/updated_at | TIMESTAMPTZ | auto |

### alert_instances
| Column | Type | Constraints |
|--------|------|-------------|
| id | UUID | PK |
| tenant_id | TEXT | NOT NULL |
| project_id | TEXT | nullable |
| environment_id | TEXT | nullable |
| rule_id | UUID | FK → alert_rules, ON DELETE SET NULL |
| source | TEXT | NOT NULL |
| severity | TEXT | CHECK (critical, warning, info) |
| fingerprint | TEXT | NOT NULL |
| state | TEXT | CHECK (pending, firing, acknowledged, silenced, resolved) |
| summary | TEXT | NOT NULL |
| first_seen_at/last_seen_at | TIMESTAMPTZ | NOT NULL |
| occurrence_count | INTEGER | default 1 |
| version | INTEGER | NOT NULL, optimistic concurrency |

### alert_state_audits
| Column | Type | Constraints |
|--------|------|-------------|
| id | UUID | PK |
| alert_id | UUID | FK → alert_instances, ON DELETE CASCADE |
| previous_state | TEXT | NOT NULL |
| new_state | TEXT | NOT NULL |
| actor_id | TEXT | NOT NULL |
| reason | TEXT | nullable |
| version | INTEGER | NOT NULL |

## Key Design Decisions
- `fingerprint` unique index is partial (WHERE state != 'resolved') — only active alerts have unique fingerprints.
- `alert_state_audits` uses ON DELETE CASCADE — audit trail follows alert lifecycle.
- `rule_id` uses ON DELETE SET NULL — rules can be deleted without losing alert history.
- Eight indexes on alert_instances for common query patterns: tenant, state, severity, source, time, operation.

## Verification
- Migration follows the same `CREATE TABLE IF NOT EXISTS` pattern as 001.
- Rollback drops all three tables in reverse dependency order.
- All CHECK constraints enforce enum values matching the JSON Schema definitions.