# Channel, NotificationJob, Delivery, and Outbox Migration Evidence

## Migration Files Created
- `database/postgresql/migrations/004_alert_channel_delivery.sql` — Forward migration
- `database/postgresql/migrations/004_alert_channel_delivery.rollback.sql` — Rollback script

## Tables Created

### notification_channels (5 new tables + 1 extension)
- **notification_channels** — Channel configurations with type, capability, config/secret ref, conformance ref
- **notification_jobs** — Per-notification dispatch jobs with idempotency key, priority, state machine
- **delivery_records** — Delivery state tracking with channel type, masked destination, attempt count, timestamps
- **delivery_attempts** — Per-attempt records with result class, response code, duration, trace ID
- **user_notification_preferences** — Per-user language, timezone, channel, and severity filter preferences
- **outbox_events** — Extended with `alert_id` and `delivery_id` columns for alert notification context

### Key Design Decisions
- `idempotency_key` on notification_jobs has a unique index — prevents duplicate dispatch.
- Delivery state machine (pending → sending → accepted/delivered/read/ failed/suppressed/cancelled) matches the schema definition.
- `delivery_records.destination_masked` stores only masked contact info (e.g., `+86****1234`).
- `delivery_attempts` captures every attempt with trace_id for distributed tracing.
- `user_notification_preferences` has a unique constraint on (tenant_id, user_id).
- `priority` on notification_jobs (0-100) enables Critical alerts to be routed before lower-priority ones.
- `next_attempt_at` on delivery_records enables efficient retry scheduling queries.

## Verification
- All tables use `CREATE TABLE IF NOT EXISTS` for idempotent migration.
- Rollback drops 5 tables and removes 2 columns from outbox_events.
- CHECK constraints enforce enum values matching the JSON Schema definitions.
- Foreign keys ensure referential integrity across the alert notification chain.