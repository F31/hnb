-- Migration: 035_artifact_storage_profiles
-- Description: Add control-plane ArtifactStorageProfile metadata and migration requests.
-- Tiers: All
-- Dependencies: 033_artifact_descriptors, 008_operation_engine_core, 009_config_secret_engine

CREATE TABLE IF NOT EXISTS artifact_storage_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    backend TEXT NOT NULL CHECK (backend IN ('local', 'pvc', 's3', 'oci')),
    service_tier TEXT NOT NULL CHECK (service_tier IN ('minimal', 'lite_ha', 'standard', 'enterprise')),
    authority_role TEXT NOT NULL CHECK (authority_role IN ('authoritative', 'mirror', 'cache')),
    secret_reference TEXT,
    endpoint TEXT,
    region TEXT,
    rpo_seconds INTEGER NOT NULL DEFAULT 0 CHECK (rpo_seconds >= 0),
    rto_seconds INTEGER NOT NULL DEFAULT 0 CHECK (rto_seconds >= 0),
    lifecycle_state TEXT NOT NULL DEFAULT 'active' CHECK (lifecycle_state IN ('active', 'migrating', 'disabled')),
    metadata JSONB NOT NULL DEFAULT '{}',
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_artifact_storage_profiles_tenant_name
    ON artifact_storage_profiles(tenant_id, name);
CREATE INDEX IF NOT EXISTS idx_artifact_storage_profiles_tenant_backend
    ON artifact_storage_profiles(tenant_id, backend, authority_role);

ALTER TABLE artifacts
    ADD COLUMN IF NOT EXISTS storage_profile_id UUID REFERENCES artifact_storage_profiles(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS artifact_profile_migrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    artifact_id UUID NOT NULL REFERENCES artifacts(id) ON DELETE RESTRICT,
    source_profile_id UUID NOT NULL REFERENCES artifact_storage_profiles(id) ON DELETE RESTRICT,
    target_profile_id UUID NOT NULL REFERENCES artifact_storage_profiles(id) ON DELETE RESTRICT,
    artifact_digest TEXT NOT NULL CHECK (artifact_digest ~ '^sha256:[0-9a-f]{64}$'),
    status TEXT NOT NULL DEFAULT 'requested' CHECK (status IN ('requested', 'queued', 'in_progress', 'succeeded', 'failed', 'cancelled')),
    operation_id UUID REFERENCES operations(id) ON DELETE SET NULL,
    checkpoint JSONB NOT NULL DEFAULT '{}',
    idempotency_key TEXT NOT NULL,
    requested_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (source_profile_id <> target_profile_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_artifact_profile_migrations_tenant_idempotency
    ON artifact_profile_migrations(tenant_id, idempotency_key);
CREATE INDEX IF NOT EXISTS idx_artifact_profile_migrations_artifact
    ON artifact_profile_migrations(tenant_id, artifact_id, status);
