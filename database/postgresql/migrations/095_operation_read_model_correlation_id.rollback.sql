-- Rollback: 095_operation_read_model_correlation_id

BEGIN;

ALTER TABLE operation_read_model DROP COLUMN IF EXISTS correlation_id;

COMMIT;
