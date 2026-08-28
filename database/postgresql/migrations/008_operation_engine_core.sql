-- Migration: 008_operation_engine_core
-- Description: Create ExecutionPlan, Operation 10-state machine, Step, Checkpoint, Compensation, Audit, ReadModel tables
-- Tiers: All
-- Dependencies: 001_nats_jetstream_outbox (outbox_events, worker_leases), 005_identity_core (tenants, projects, environments)

-- 1. ExecutionPlans (immutable plan generated from ReleaseManifest)
CREATE TABLE IF NOT EXISTS execution_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    release_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    project_id TEXT,
    environment_id TEXT,
    plan_digest TEXT NOT NULL,
    plan_json JSONB NOT NULL,
    policy_result JSONB DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'superseded', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    superseded_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_execution_plans_release_id ON execution_plans(release_id);
CREATE INDEX IF NOT EXISTS idx_execution_plans_tenant_id ON execution_plans(tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_execution_plans_digest ON execution_plans(plan_digest);

-- 2. Operations (10-state machine)
CREATE TABLE IF NOT EXISTS operations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    project_id TEXT,
    environment_id TEXT,
    namespace_id TEXT,
    plan_id UUID REFERENCES execution_plans(id),
    operation_type TEXT NOT NULL CHECK (operation_type IN (
        'deploy', 'upgrade', 'rollback', 'scale', 'backup',
        'restore', 'switchover', 'delete', 'gc', 'ota', 'config_change'
    )),
    status TEXT NOT NULL CHECK (status IN (
        'pending', 'pending_approval', 'queued', 'queued_offline',
        'in_progress', 'paused', 'compensating',
        'succeeded', 'failed', 'cancelled'
    )) DEFAULT 'pending',
    initiated_by TEXT NOT NULL,
    approved_by TEXT,
    correlation_id TEXT,
    idempotency_key TEXT NOT NULL,
    plan_digest TEXT,
    status_reason TEXT,
    total_steps INTEGER NOT NULL DEFAULT 0,
    completed_steps INTEGER NOT NULL DEFAULT 0,
    failed_steps INTEGER NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 0,
    tags JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_operations_tenant_id ON operations(tenant_id);
CREATE INDEX IF NOT EXISTS idx_operations_project_id ON operations(project_id);
CREATE INDEX IF NOT EXISTS idx_operations_environment_id ON operations(environment_id);
CREATE INDEX IF NOT EXISTS idx_operations_status ON operations(status);
CREATE INDEX IF NOT EXISTS idx_operations_plan_id ON operations(plan_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_operations_tenant_idempotency
    ON operations(tenant_id, idempotency_key);
CREATE INDEX IF NOT EXISTS idx_operations_created_at ON operations(created_at DESC);

-- 3. Operation Steps
CREATE TABLE IF NOT EXISTS operation_steps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_id UUID NOT NULL REFERENCES operations(id) ON DELETE CASCADE,
    plan_step_id TEXT,
    step_name TEXT NOT NULL,
    step_type TEXT NOT NULL,
    provider_id TEXT,
    status TEXT NOT NULL CHECK (status IN (
        'pending', 'running', 'succeeded', 'failed', 'skipped', 'compensated'
    )) DEFAULT 'pending',
    idempotency_key TEXT NOT NULL,
    depends_on TEXT[] DEFAULT '{}',
    optional BOOLEAN NOT NULL DEFAULT false,
    retry_count INTEGER NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3 CHECK (max_retries BETWEEN 0 AND 9),
    timeout_seconds INTEGER NOT NULL DEFAULT 300 CHECK (timeout_seconds > 0),
    checkpoint TEXT,
    fencing_token UUID,
    last_heartbeat_at TIMESTAMPTZ,
    step_input JSONB DEFAULT '{}',
    step_output JSONB DEFAULT '{}',
    error_message TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_operation_steps_operation_id ON operation_steps(operation_id);
CREATE INDEX IF NOT EXISTS idx_operation_steps_status ON operation_steps(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_operation_steps_idempotency ON operation_steps(operation_id, idempotency_key);
CREATE UNIQUE INDEX IF NOT EXISTS idx_operation_steps_plan_step
    ON operation_steps(operation_id, plan_step_id) WHERE plan_step_id IS NOT NULL;
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'operation_steps' AND column_name = 'fencing_token'
    ) THEN
        CREATE INDEX IF NOT EXISTS idx_operation_steps_fencing_token ON operation_steps(fencing_token);
    ELSIF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'operation_steps' AND column_name = 'last_lease_id'
    ) THEN
        CREATE INDEX IF NOT EXISTS idx_operation_steps_last_lease_id ON operation_steps(last_lease_id);
    END IF;
END
$$;

-- 4. Step Checkpoints (resume points)
CREATE TABLE IF NOT EXISTS step_checkpoints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_id UUID NOT NULL REFERENCES operations(id) ON DELETE CASCADE,
    step_id UUID NOT NULL REFERENCES operation_steps(id) ON DELETE CASCADE,
    checkpoint_data JSONB NOT NULL DEFAULT '{}',
    sequence INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_step_checkpoints_step_id ON step_checkpoints(step_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_step_checkpoints_step_seq ON step_checkpoints(step_id, sequence);

-- 5. Compensation Records
CREATE TABLE IF NOT EXISTS compensation_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_id UUID NOT NULL REFERENCES operations(id) ON DELETE CASCADE,
    step_id UUID NOT NULL REFERENCES operation_steps(id) ON DELETE CASCADE,
    resource_type TEXT NOT NULL,
    resource_id TEXT,
    compensation_type TEXT NOT NULL CHECK (compensation_type IN (
        'rollback', 'delete', 'retain_mark', 'retain_notify', 'skip'
    )),
    status TEXT NOT NULL CHECK (status IN (
        'pending', 'in_progress', 'succeeded', 'failed', 'skipped'
    )) DEFAULT 'pending',
    compensation_data JSONB DEFAULT '{}',
    result_data JSONB DEFAULT '{}',
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_compensation_records_operation_id ON compensation_records(operation_id);
CREATE INDEX IF NOT EXISTS idx_compensation_records_step_id ON compensation_records(step_id);
CREATE INDEX IF NOT EXISTS idx_compensation_records_status ON compensation_records(status);

-- 6. Operation Audit (evidence chain)
CREATE TABLE IF NOT EXISTS operation_audit (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_id UUID NOT NULL REFERENCES operations(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL CHECK (event_type IN (
        'created', 'approved', 'rejected', 'started', 'step_completed',
        'step_failed', 'compensated', 'paused', 'resumed',
        'cancelled', 'succeeded', 'failed', 'state_changed'
    )),
    actor_id TEXT NOT NULL,
    previous_state TEXT,
    new_state TEXT,
    detail JSONB DEFAULT '{}',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_operation_audit_operation_id ON operation_audit(operation_id);
CREATE INDEX IF NOT EXISTS idx_operation_audit_actor_id ON operation_audit(actor_id);
CREATE INDEX IF NOT EXISTS idx_operation_audit_occurred_at ON operation_audit(occurred_at DESC);

-- 7. Operation Read Model (query projection)
CREATE TABLE IF NOT EXISTS operation_read_model (
    operation_id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    project_id TEXT,
    environment_id TEXT,
    namespace_id TEXT,
    plan_id UUID,
    operation_type TEXT NOT NULL,
    status TEXT NOT NULL,
    total_steps INTEGER NOT NULL DEFAULT 0,
    completed_steps INTEGER NOT NULL DEFAULT 0,
    failed_steps INTEGER NOT NULL DEFAULT 0,
    initiated_by TEXT NOT NULL,
    approved_by TEXT,
    summary TEXT,
    tags JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    last_state_changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_operation_read_model_tenant_id ON operation_read_model(tenant_id);
CREATE INDEX IF NOT EXISTS idx_operation_read_model_project_id ON operation_read_model(project_id);
CREATE INDEX IF NOT EXISTS idx_operation_read_model_environment_id ON operation_read_model(environment_id);
CREATE INDEX IF NOT EXISTS idx_operation_read_model_status ON operation_read_model(status);
CREATE INDEX IF NOT EXISTS idx_operation_read_model_created_at ON operation_read_model(created_at DESC);
