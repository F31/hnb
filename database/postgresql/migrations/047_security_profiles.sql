-- Migration: 047_security_profiles.sql
-- Description: Vulnerability scanning configuration, vulnerability DB tracking,
-- and scan report storage for artifact security scanning.
-- Tiers: All
-- Dependencies: 044_tenant_id_foreign_keys

CREATE TABLE IF NOT EXISTS artifact_scan_profiles (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    engine TEXT NOT NULL CHECK (engine IN ('trivy','native')),
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled BOOLEAN NOT NULL DEFAULT true,
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_scan_profiles_default_per_tenant
    ON artifact_scan_profiles(tenant_id) WHERE is_default;

CREATE TABLE IF NOT EXISTS artifact_vulnerability_db (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    profile_id UUID NOT NULL REFERENCES artifact_scan_profiles(id) ON DELETE CASCADE,
    engine TEXT NOT NULL,
    db_label TEXT NOT NULL,
    policy TEXT NOT NULL CHECK (policy IN ('daily','weekly','manual')),
    last_sync_at TIMESTAMPTZ,
    next_sync_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'idle' CHECK (status IN ('idle','syncing','failed','fresh')),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, profile_id, db_label)
);

CREATE TABLE IF NOT EXISTS artifact_scan_reports (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    artifact_id UUID NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    profile_id UUID NOT NULL REFERENCES artifact_scan_profiles(id) ON DELETE RESTRICT,
    state TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','running','completed','failed','cancelled')),
    severity_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    findings JSONB NOT NULL DEFAULT '[]'::jsonb,
    triggered_by TEXT,
    error_message TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_scan_reports_tenant_state
    ON artifact_scan_reports(tenant_id, state, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_scan_reports_artifact
    ON artifact_scan_reports(artifact_id);