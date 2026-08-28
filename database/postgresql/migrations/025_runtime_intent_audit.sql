-- Migration: 025_runtime_intent_audit
-- Description: Add immutable runtime intents and append-only security audit linkage
-- Tiers: All
-- Dependencies: 008_operation_engine_core, 010_runtime_target_engine, 024_scoped_identity

BEGIN;

CREATE UNIQUE INDEX IF NOT EXISTS idx_execution_plans_id_tenant ON execution_plans(id, tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_operations_id_tenant ON operations(id, tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_runtime_targets_id_tenant ON runtime_targets(id, tenant_id);

CREATE TABLE IF NOT EXISTS runtime_intents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    subject_id UUID NOT NULL,
    intent_kind TEXT NOT NULL,
    api_version TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    semantic_digest TEXT NOT NULL,
    intent_document JSONB NOT NULL,
    release_id UUID REFERENCES releases(id) ON DELETE RESTRICT,
    runtime_target_id UUID,
    execution_plan_id UUID NOT NULL,
    operation_id UUID NOT NULL,
    policy_version_id UUID,
    correlation_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, intent_kind, idempotency_key),
    UNIQUE (id, tenant_id),
    UNIQUE (execution_plan_id),
    UNIQUE (operation_id),
    FOREIGN KEY (tenant_id, subject_id)
        REFERENCES tenant_memberships(tenant_id, subject_id) ON DELETE RESTRICT,
    FOREIGN KEY (runtime_target_id, tenant_id)
        REFERENCES runtime_targets(id, tenant_id) ON DELETE RESTRICT,
    FOREIGN KEY (execution_plan_id, tenant_id)
        REFERENCES execution_plans(id, tenant_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (operation_id, tenant_id)
        REFERENCES operations(id, tenant_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (policy_version_id, tenant_id)
        REFERENCES authorization_policy_versions(id, tenant_id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_runtime_intents_subject ON runtime_intents(tenant_id, subject_id);
CREATE INDEX IF NOT EXISTS idx_runtime_intents_correlation ON runtime_intents(correlation_id);

ALTER TABLE execution_plans ADD COLUMN IF NOT EXISTS runtime_intent_id UUID;
ALTER TABLE operations ADD COLUMN IF NOT EXISTS runtime_intent_id UUID;
ALTER TABLE applications ADD COLUMN IF NOT EXISTS runtime_intent_id UUID;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_execution_plans_runtime_intent') THEN
        ALTER TABLE execution_plans ADD CONSTRAINT fk_execution_plans_runtime_intent
            FOREIGN KEY (runtime_intent_id, tenant_id)
            REFERENCES runtime_intents(id, tenant_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_operations_runtime_intent') THEN
        ALTER TABLE operations ADD CONSTRAINT fk_operations_runtime_intent
            FOREIGN KEY (runtime_intent_id, tenant_id)
            REFERENCES runtime_intents(id, tenant_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_applications_runtime_intent') THEN
        ALTER TABLE applications ADD CONSTRAINT fk_applications_runtime_intent
            FOREIGN KEY (runtime_intent_id, tenant_id)
            REFERENCES runtime_intents(id, tenant_id) ON DELETE RESTRICT;
    END IF;
END
$$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_execution_plans_runtime_intent
    ON execution_plans(runtime_intent_id) WHERE runtime_intent_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_operations_runtime_intent
    ON operations(runtime_intent_id) WHERE runtime_intent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_applications_runtime_intent ON applications(runtime_intent_id);

CREATE TABLE IF NOT EXISTS security_audit_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    subject_id UUID,
    event_type TEXT NOT NULL,
    decision TEXT CHECK (decision IN ('allow', 'deny', 'not_applicable')),
    reason_code TEXT,
    action TEXT,
    resource_kind TEXT,
    resource_id TEXT,
    scope JSONB NOT NULL DEFAULT '{}',
    policy_version_id UUID,
    key_id UUID,
    runtime_intent_id UUID,
    execution_plan_id UUID,
    operation_id UUID,
    correlation_id UUID NOT NULL,
    trace_id TEXT,
    outcome TEXT NOT NULL,
    detail JSONB NOT NULL DEFAULT '{}',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, subject_id)
        REFERENCES tenant_memberships(tenant_id, subject_id) ON DELETE RESTRICT,
    FOREIGN KEY (policy_version_id, tenant_id)
        REFERENCES authorization_policy_versions(id, tenant_id) ON DELETE RESTRICT,
    FOREIGN KEY (runtime_intent_id, tenant_id)
        REFERENCES runtime_intents(id, tenant_id) ON DELETE RESTRICT,
    FOREIGN KEY (execution_plan_id, tenant_id)
        REFERENCES execution_plans(id, tenant_id) ON DELETE RESTRICT,
    FOREIGN KEY (operation_id, tenant_id)
        REFERENCES operations(id, tenant_id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_security_audit_tenant_time
    ON security_audit_events(tenant_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_security_audit_correlation ON security_audit_events(correlation_id);
CREATE INDEX IF NOT EXISTS idx_security_audit_operation ON security_audit_events(operation_id);

CREATE OR REPLACE FUNCTION reject_immutable_row_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$;

DROP TRIGGER IF EXISTS runtime_intents_immutable ON runtime_intents;
CREATE TRIGGER runtime_intents_immutable
    BEFORE UPDATE OR DELETE ON runtime_intents
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_row_change();

DROP TRIGGER IF EXISTS security_audit_events_immutable ON security_audit_events;
CREATE TRIGGER security_audit_events_immutable
    BEFORE UPDATE OR DELETE ON security_audit_events
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_row_change();

COMMIT;
