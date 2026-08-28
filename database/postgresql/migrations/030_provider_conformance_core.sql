-- 030_provider_conformance_core.sql
-- Provider conformance: manifests, compatibility matrix, certification status

BEGIN;

CREATE TABLE IF NOT EXISTS provider_manifests (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id     TEXT NOT NULL,
    name            TEXT NOT NULL,
    version         TEXT NOT NULL,
    protocol_version TEXT NOT NULL,
    capabilities    TEXT[] NOT NULL DEFAULT '{}',
    actions         TEXT[] NOT NULL DEFAULT '{}',
    permissions     JSONB DEFAULT '{}',
    resource_requirements JSONB DEFAULT '{}',
    dependencies    JSONB DEFAULT '[]',
    compatibility   JSONB DEFAULT '{}',
    conformance_level TEXT NOT NULL DEFAULT 'none',
    conformance_evidence JSONB DEFAULT '[]',
    conformance_expires_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_provider_manifests_provider ON provider_manifests(provider_id);

CREATE TABLE IF NOT EXISTS provider_compatibility_matrix (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    core_version        TEXT NOT NULL,
    provider_id         TEXT NOT NULL,
    provider_version    TEXT NOT NULL,
    runtime_target_type TEXT NOT NULL,
    compatible          BOOLEAN NOT NULL DEFAULT true,
    constraint_reason   TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_comp_matrix_lookup ON provider_compatibility_matrix(core_version, provider_id, provider_version);

CREATE UNIQUE INDEX IF NOT EXISTS idx_comp_matrix_unique ON provider_compatibility_matrix(core_version, provider_id, provider_version, runtime_target_type);

COMMIT;