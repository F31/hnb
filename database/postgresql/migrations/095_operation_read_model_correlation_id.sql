-- Migration: 095_operation_read_model_correlation_id
-- Description: Project correlation_id into the operation read model so list and
--              detail GET responses carry the real correlation id (previously
--              only write-action responses re-attached it from the write side).
--              Backfilled from operations.correlation_id; rows without a known
--              correlation keep NULL.
-- Dependencies: 008_operation_engine_core (operation_read_model, operations)

BEGIN;

ALTER TABLE operation_read_model
    ADD COLUMN IF NOT EXISTS correlation_id TEXT;

UPDATE operation_read_model rm
SET correlation_id = op.correlation_id
FROM operations op
WHERE op.id = rm.operation_id
  AND rm.correlation_id IS NULL
  AND op.correlation_id IS NOT NULL;

COMMIT;
