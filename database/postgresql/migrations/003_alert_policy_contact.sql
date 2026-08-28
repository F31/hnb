-- Migration: 003_alert_policy_contact
-- Description: Create Silence, MaintenanceWindow, NotificationPolicy, ContactGroup, Schedule tables
-- Tiers: All
-- Dependencies: 002_alert_notification_core

-- 1. Silences
CREATE TABLE IF NOT EXISTS silences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    matchers JSONB NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    reason TEXT NOT NULL,
    created_by TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'expired', 'pending')),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_silences_tenant_id ON silences(tenant_id);
CREATE INDEX IF NOT EXISTS idx_silences_status ON silences(status);
CREATE INDEX IF NOT EXISTS idx_silences_starts_at ON silences(starts_at);
CREATE INDEX IF NOT EXISTS idx_silences_ends_at ON silences(ends_at);

-- 2. MaintenanceWindows
CREATE TABLE IF NOT EXISTS maintenance_windows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    matchers JSONB NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    reason TEXT NOT NULL,
    created_by TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('scheduled', 'active', 'ended')),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_maintenance_windows_tenant_id ON maintenance_windows(tenant_id);
CREATE INDEX IF NOT EXISTS idx_maintenance_windows_status ON maintenance_windows(status);
CREATE INDEX IF NOT EXISTS idx_maintenance_windows_starts_at ON maintenance_windows(starts_at);
CREATE INDEX IF NOT EXISTS idx_maintenance_windows_ends_at ON maintenance_windows(ends_at);

-- 3. NotificationPolicies
CREATE TABLE IF NOT EXISTS notification_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_scope TEXT NOT NULL CHECK (tenant_scope IN ('global', 'tenant', 'project')),
    matchers JSONB NOT NULL DEFAULT '[]',
    contact_group_id UUID NOT NULL,
    channels JSONB NOT NULL DEFAULT '[]',
    repeat_interval TEXT NOT NULL DEFAULT '5m',
    escalation_steps JSONB DEFAULT '[]',
    active_schedule JSONB,
    recovery_notification BOOLEAN NOT NULL DEFAULT true,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notification_policies_tenant_scope ON notification_policies(tenant_scope);
CREATE INDEX IF NOT EXISTS idx_notification_policies_contact_group ON notification_policies(contact_group_id);

-- 4. ContactGroups
CREATE TABLE IF NOT EXISTS contact_groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    members JSONB NOT NULL DEFAULT '[]',
    schedule_ref UUID,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_contact_groups_tenant_id ON contact_groups(tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_contact_groups_tenant_name ON contact_groups(tenant_id, name);

-- 5. Schedules
CREATE TABLE IF NOT EXISTS schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    timezone TEXT NOT NULL,
    shifts JSONB DEFAULT '[]',
    exceptions JSONB DEFAULT '[]',
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_schedules_tenant_id ON schedules(tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_schedules_tenant_name ON schedules(tenant_id, name);