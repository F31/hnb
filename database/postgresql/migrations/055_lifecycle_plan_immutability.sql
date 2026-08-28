-- Migration: 055_lifecycle_plan_immutability
-- Description: Add per-step Provider pin, immutable step input and compensation metadata,
-- enforce execution plan immutability at the database level, and persist the target
-- snapshot taken by the planner.
-- Tiers: All
-- Dependencies: 008_operation_engine_core, 013_operation_fencing_v2, 025_runtime_intent_audit

BEGIN;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'operation_steps' AND column_name = 'fencing_token'
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'operation_steps' AND column_name = 'last_lease_id'
    ) THEN
        ALTER TABLE operation_steps RENAME COLUMN fencing_token TO last_lease_id;
    END IF;
END
$$;

ALTER TABLE operation_steps
    ADD COLUMN IF NOT EXISTS provider_version TEXT,
    ADD COLUMN IF NOT EXISTS provider_digest TEXT,
    ADD COLUMN IF NOT EXISTS provider_protocol_version TEXT,
    ADD COLUMN IF NOT EXISTS input_schema TEXT,
    ADD COLUMN IF NOT EXISTS secret_references JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS compensation JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS target_ref TEXT,
    ADD COLUMN IF NOT EXISTS target_kind TEXT;

UPDATE operation_steps
SET provider_version = COALESCE(provider_version, ''),
    provider_digest = CASE WHEN provider_id IS NULL OR provider_id = '' THEN '' ELSE COALESCE(provider_digest, '') END,
    provider_protocol_version = COALESCE(NULLIF(provider_protocol_version, ''), '2.0.0'),
    input_schema = COALESCE(input_schema, ''),
    target_kind = COALESCE(target_kind, ''),
    target_ref = COALESCE(target_ref, '');

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ck_operation_steps_provider_version_nonempty'
    ) THEN
        ALTER TABLE operation_steps ADD CONSTRAINT ck_operation_steps_provider_version_nonempty
            CHECK (provider_version <> '' OR provider_id IS NULL OR provider_id = '');
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ck_operation_steps_provider_digest_shape'
    ) THEN
        ALTER TABLE operation_steps ADD CONSTRAINT ck_operation_steps_provider_digest_shape
            CHECK (provider_digest = '' OR provider_digest ~ '^sha256:[0-9a-f]{64}$');
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ck_operation_steps_provider_protocol_v2'
    ) THEN
        ALTER TABLE operation_steps ADD CONSTRAINT ck_operation_steps_provider_protocol_v2
            CHECK (provider_protocol_version IN ('', '2.0.0'));
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_operation_steps_provider
    ON operation_steps(operation_id, provider_id) WHERE provider_id IS NOT NULL;

-- Execution plan immutability: reject mutations of plan identity and JSON once
-- the plan is in the active state. Superseded/cancelled plans remain mutable for
-- lifecycle bookkeeping but their plan_json/plan_digest/release_id must not be
-- altered.
CREATE OR REPLACE FUNCTION execution_plans_immutable_columns()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.plan_digest IS DISTINCT FROM OLD.plan_digest
       OR NEW.plan_json IS DISTINCT FROM OLD.plan_json
       OR NEW.release_id IS DISTINCT FROM OLD.release_id
       OR NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
       OR NEW.runtime_intent_id IS DISTINCT FROM OLD.runtime_intent_id
    THEN
        RAISE EXCEPTION 'execution_plans row is immutable: plan_id %, tenant %', OLD.id, OLD.tenant_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'execution_plans_immutable'
    ) THEN
        CREATE TRIGGER execution_plans_immutable
            BEFORE UPDATE ON execution_plans
            FOR EACH ROW EXECUTE FUNCTION execution_plans_immutable_columns();
    END IF;
END
$$;

-- operation_steps: protect the routing/input metadata once persisted. Status,
-- retry, checkpoint, lease, output, error and timestamps remain mutable.
CREATE OR REPLACE FUNCTION operation_steps_immutable_inputs()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.plan_step_id IS DISTINCT FROM OLD.plan_step_id
       OR NEW.step_name IS DISTINCT FROM OLD.step_name
       OR NEW.step_type IS DISTINCT FROM OLD.step_type
       OR NEW.provider_id IS DISTINCT FROM OLD.provider_id
       OR NEW.provider_version IS DISTINCT FROM OLD.provider_version
       OR NEW.provider_digest IS DISTINCT FROM OLD.provider_digest
       OR NEW.provider_protocol_version IS DISTINCT FROM OLD.provider_protocol_version
       OR NEW.input_schema IS DISTINCT FROM OLD.input_schema
       OR NEW.step_input IS DISTINCT FROM OLD.step_input
       OR NEW.secret_references IS DISTINCT FROM OLD.secret_references
       OR NEW.compensation IS DISTINCT FROM OLD.compensation
       OR NEW.target_ref IS DISTINCT FROM OLD.target_ref
       OR NEW.target_kind IS DISTINCT FROM OLD.target_kind
       OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
       OR NEW.max_retries IS DISTINCT FROM OLD.max_retries
       OR NEW.timeout_seconds IS DISTINCT FROM OLD.timeout_seconds
    THEN
        RAISE EXCEPTION 'operation_steps routing/input is immutable: step %, op %', OLD.id, OLD.operation_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'operation_steps_immutable'
    ) THEN
        CREATE TRIGGER operation_steps_immutable
            BEFORE UPDATE ON operation_steps
            FOR EACH ROW EXECUTE FUNCTION operation_steps_immutable_inputs();
    END IF;
END
$$;

COMMIT;
