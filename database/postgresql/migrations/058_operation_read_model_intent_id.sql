-- Migration: 058_operation_read_model_intent_id
-- Description: Add the owning runtime intent UUID to the operation read model
--              so the Operation BFF and Operation Center can deep-link to the
--              originating intent. Backfilled conservatively from
--              operations.runtime_intent_id; rows without a known intent keep
--              NULL and are omitted from the deep link.
-- Dependencies: 053_runtime_intent_commitment

BEGIN;

ALTER TABLE operation_read_model
    ADD COLUMN IF NOT EXISTS intent_id UUID;

UPDATE operation_read_model rm
SET intent_id = op.runtime_intent_id
FROM operations op
WHERE op.id = rm.operation_id
  AND rm.intent_id IS NULL
  AND op.runtime_intent_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_operation_read_model_intent_id
    ON operation_read_model(tenant_id, intent_id);

COMMIT;
