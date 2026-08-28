-- Migration: 002_alert_notification_core
-- Description: Create Alert/Notification core tables: AlertRule, AlertInstance, AlertStateAudit
-- Tiers: All
-- Dependencies: 001_nats_jetstream_outbox

-- 1. AlertRule
CREATE TABLE IF NOT EXISTS alert_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_scope TEXT NOT NULL CHECK (tenant_scope IN ('global', 'tenant')),
    source_type TEXT NOT NULL,
    severity TEXT NOT NULL CHECK (severity IN ('critical', 'warning', 'info')),
    expression_ref TEXT,
    labels JSONB DEFAULT '{}',
    annotations JSONB DEFAULT '{}',
    enabled BOOLEAN NOT NULL DEFAULT true,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_alert_rules_tenant_scope ON alert_rules(tenant_scope);
CREATE INDEX IF NOT EXISTS idx_alert_rules_source_type ON alert_rules(source_type);
CREATE INDEX IF NOT EXISTS idx_alert_rules_severity ON alert_rules(severity);
CREATE INDEX IF NOT EXISTS idx_alert_rules_enabled ON alert_rules(enabled);

-- 2. AlertInstance
CREATE TABLE IF NOT EXISTS alert_instances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    project_id TEXT,
    environment_id TEXT,
    rule_id UUID REFERENCES alert_rules(id) ON DELETE SET NULL,
    source TEXT NOT NULL,
    severity TEXT NOT NULL CHECK (severity IN ('critical', 'warning', 'info')),
    resource_ref TEXT,
    fingerprint TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('pending', 'firing', 'acknowledged', 'silenced', 'resolved')),
    summary TEXT NOT NULL,
    first_seen_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    occurrence_count INTEGER NOT NULL DEFAULT 1,
    assignee_id TEXT,
    acknowledged_by TEXT,
    acknowledged_at TIMESTAMPTZ,
    acknowledgement_reason TEXT,
    correlation_id UUID,
    operation_id UUID,
    runbook_ref TEXT,
    source_ref TEXT,
    labels JSONB DEFAULT '{}',
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_alert_instances_fingerprint ON alert_instances(tenant_id, fingerprint) WHERE state != 'resolved';
CREATE INDEX IF NOT EXISTS idx_alert_instances_tenant_id ON alert_instances(tenant_id);
CREATE INDEX IF NOT EXISTS idx_alert_instances_state ON alert_instances(state);
CREATE INDEX IF NOT EXISTS idx_alert_instances_severity ON alert_instances(severity);
CREATE INDEX IF NOT EXISTS idx_alert_instances_last_seen ON alert_instances(last_seen_at);
CREATE INDEX IF NOT EXISTS idx_alert_instances_source ON alert_instances(source);
CREATE INDEX IF NOT EXISTS idx_alert_instances_operation_id ON alert_instances(operation_id);
CREATE INDEX IF NOT EXISTS idx_alert_instances_created_at ON alert_instances(created_at);

-- 3. AlertStateAudit
CREATE TABLE IF NOT EXISTS alert_state_audits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_id UUID NOT NULL REFERENCES alert_instances(id) ON DELETE CASCADE,
    previous_state TEXT NOT NULL,
    new_state TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    reason TEXT,
    version INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_alert_state_audits_alert_id ON alert_state_audits(alert_id);
CREATE INDEX IF NOT EXISTS idx_alert_state_audits_created_at ON alert_state_audits(created_at);