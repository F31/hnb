-- Rollback: 055_lifecycle_plan_immutability

BEGIN;

DROP TRIGGER IF EXISTS operation_steps_immutable ON operation_steps;
DROP TRIGGER IF EXISTS execution_plans_immutable ON execution_plans;
DROP FUNCTION IF EXISTS operation_steps_immutable_inputs();
DROP FUNCTION IF EXISTS execution_plans_immutable_columns();

DROP INDEX IF EXISTS idx_operation_steps_provider;

ALTER TABLE operation_steps
    DROP COLUMN IF EXISTS compensation,
    DROP COLUMN IF EXISTS secret_references,
    DROP COLUMN IF EXISTS input_schema,
    DROP COLUMN IF EXISTS provider_protocol_version,
    DROP COLUMN IF EXISTS provider_digest,
    DROP COLUMN IF EXISTS provider_version,
    DROP COLUMN IF EXISTS target_kind,
    DROP COLUMN IF EXISTS target_ref;

ALTER TABLE operation_steps DROP CONSTRAINT IF EXISTS ck_operation_steps_provider_version_nonempty;
ALTER TABLE operation_steps DROP CONSTRAINT IF EXISTS ck_operation_steps_provider_digest_shape;
ALTER TABLE operation_steps DROP CONSTRAINT IF EXISTS ck_operation_steps_provider_protocol_v2;

COMMIT;
