-- Migration: 087_outbox_tenant_scoped_idempotency
-- Description: Scope the outbox idempotency uniqueness per tenant. The previous
-- global UNIQUE (message_type, idempotency_key) made two tenants sharing an
-- idempotency key collide on insert (step-requested events derive their keys
-- from the operation idempotency key + deterministic step IDs), breaking
-- cross-tenant isolation for the legacy SubmitOperation path.
-- Dependencies: 008_operation_engine_core

BEGIN;

ALTER TABLE outbox_events
    DROP CONSTRAINT IF EXISTS outbox_events_message_type_idempotency_key_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_outbox_tenant_type_idem
    ON outbox_events (tenant_id, message_type, idempotency_key);

COMMIT;
