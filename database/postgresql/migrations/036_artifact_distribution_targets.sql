-- Migration: 036_artifact_distribution_targets
-- Description: Add artifact distribution target metadata for regional mirrors and edge caches.
-- Tiers: All
-- Dependencies: 035_artifact_storage_profiles, 008_operation_engine_core

CREATE TABLE IF NOT EXISTS artifact_distribution_targets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    artifact_id UUID NOT NULL REFERENCES artifacts(id) ON DELETE RESTRICT,
    authority_profile_id UUID NOT NULL REFERENCES artifact_storage_profiles(id) ON DELETE RESTRICT,
    target_profile_id UUID NOT NULL REFERENCES artifact_storage_profiles(id) ON DELETE RESTRICT,
    target_role TEXT NOT NULL CHECK (target_role IN ('regional_mirror', 'edge_cache')),
    desired_digest TEXT NOT NULL CHECK (desired_digest ~ '^sha256:[0-9a-f]{64}$'),
    observed_digest TEXT CHECK (observed_digest IS NULL OR observed_digest ~ '^sha256:[0-9a-f]{64}$'),
    state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'syncing', 'ready', 'stale', 'failed')),
    health TEXT NOT NULL DEFAULT 'unknown' CHECK (health IN ('unknown', 'healthy', 'degraded', 'unavailable')),
    low_watermark_bytes BIGINT NOT NULL DEFAULT 0 CHECK (low_watermark_bytes >= 0),
    high_watermark_bytes BIGINT NOT NULL DEFAULT 0 CHECK (high_watermark_bytes >= 0),
    current_bytes BIGINT NOT NULL DEFAULT 0 CHECK (current_bytes >= 0),
    local_lock BOOLEAN NOT NULL DEFAULT false,
    rebuild_operation_id UUID REFERENCES operations(id) ON DELETE SET NULL,
    last_error TEXT,
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (authority_profile_id <> target_profile_id),
    CHECK (high_watermark_bytes = 0 OR high_watermark_bytes >= low_watermark_bytes)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_artifact_distribution_targets_tenant_idempotency
    ON artifact_distribution_targets(tenant_id, idempotency_key);
CREATE INDEX IF NOT EXISTS idx_artifact_distribution_targets_artifact
    ON artifact_distribution_targets(tenant_id, artifact_id, state);
CREATE INDEX IF NOT EXISTS idx_artifact_distribution_targets_rebuild
    ON artifact_distribution_targets(tenant_id, rebuild_operation_id) WHERE rebuild_operation_id IS NOT NULL;
