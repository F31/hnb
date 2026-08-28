-- Rollback: 058_operation_read_model_intent_id

BEGIN;

ALTER TABLE operation_read_model DROP COLUMN IF EXISTS intent_id;

COMMIT;
