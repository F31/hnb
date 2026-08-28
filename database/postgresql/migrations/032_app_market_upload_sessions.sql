-- Migration: 032_app_market_upload_sessions
-- Description: Add upload_sessions table for direct-to-Harbor artifact upload flow (ART-003 fix)
-- Tiers: All
-- Dependencies: 011_app_market_engine (artifacts), 005_identity_core (tenants)

CREATE TABLE IF NOT EXISTS upload_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    filename TEXT NOT NULL,
    artifact_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN (
        'pending', 'uploading', 'completed', 'expired', 'failed'
    )),
    harbor_url TEXT NOT NULL,
    robot_id INTEGER NOT NULL,
    robot_name TEXT,
    artifact_id UUID REFERENCES artifacts(id) ON DELETE SET NULL,
    digest TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_upload_sessions_tenant_id ON upload_sessions(tenant_id);
CREATE INDEX IF NOT EXISTS idx_upload_sessions_status ON upload_sessions(status);
CREATE INDEX IF NOT EXISTS idx_upload_sessions_expires_at ON upload_sessions(expires_at);
