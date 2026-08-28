-- 031_observability_dr_core.sql
-- Operation SLO config and alert tracking tables

BEGIN;

CREATE TABLE IF NOT EXISTS operation_slo_config (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_type  TEXT NOT NULL,
    max_duration    INTERVAL NOT NULL,
    alert_severity  TEXT NOT NULL DEFAULT 'warning',
    escalation_delay INTERVAL DEFAULT '5m',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(operation_type)
);

CREATE TABLE IF NOT EXISTS operation_slo_alerts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_id    TEXT NOT NULL,
    tenant_id       TEXT NOT NULL,
    operation_type  TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'stalled',
    stalled_since   TIMESTAMPTZ NOT NULL,
    alert_sent_at   TIMESTAMPTZ,
    escalated_at    TIMESTAMPTZ,
    resolved_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_slo_alerts_status ON operation_slo_alerts(status);
CREATE INDEX IF NOT EXISTS idx_slo_alerts_operation ON operation_slo_alerts(operation_id);

COMMIT;