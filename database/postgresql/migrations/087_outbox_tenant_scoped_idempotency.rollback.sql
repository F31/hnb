-- Rollback: 087_outbox_tenant_scoped_idempotency
-- WARNING: restores the global uniqueness; tenants sharing an idempotency key
-- will collide again.

BEGIN;

DROP INDEX IF EXISTS idx_outbox_tenant_type_idem;

ALTER TABLE outbox_events
    ADD CONSTRAINT outbox_events_message_type_idempotency_key_key
    UNIQUE (message_type, idempotency_key);

COMMIT;
