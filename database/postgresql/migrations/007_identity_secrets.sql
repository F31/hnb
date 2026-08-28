-- Migration: 007_identity_secrets
-- Description: Create SecretReference table with encrypted storage and versioning
-- Tiers: All
-- Dependencies: 005_identity_core

-- 1. SecretReferences
CREATE TABLE IF NOT EXISTS secret_references (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    secret_ref TEXT NOT NULL,
    description TEXT,
    encrypted_value TEXT NOT NULL,
    algorithm TEXT NOT NULL DEFAULT 'AES-256-GCM',
    version INTEGER NOT NULL DEFAULT 1,
    rotation_policy JSONB,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_secret_references_tenant_id ON secret_references(tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_secret_references_tenant_name ON secret_references(tenant_id, name);
CREATE INDEX IF NOT EXISTS idx_secret_references_expires_at ON secret_references(expires_at);

-- 2. SecretVersions
CREATE TABLE IF NOT EXISTS secret_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    secret_id UUID NOT NULL REFERENCES secret_references(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    encrypted_value TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_secret_versions_secret_id ON secret_versions(secret_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_secret_versions_secret_version ON secret_versions(secret_id, version);