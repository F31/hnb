-- 107: bounded application verification result, separate from Operation terminal state.
BEGIN;

CREATE TABLE IF NOT EXISTS application_delivery_verifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    binding_id UUID NOT NULL REFERENCES application_delivery_bindings(id) ON DELETE CASCADE,
    tenant_id TEXT NOT NULL,
    target_id UUID NOT NULL,
    release_id TEXT NOT NULL,
    release_version TEXT NOT NULL,
    revision BIGINT NOT NULL CHECK (revision > 0),
    operation_id UUID,
    resource_uid TEXT NOT NULL CHECK (length(resource_uid) BETWEEN 1 AND 256),
    observation_generation BIGINT CHECK (observation_generation IS NULL OR observation_generation >= 0),
    observation_revision BIGINT CHECK (observation_revision IS NULL OR observation_revision >= 0),
    observation_freshness TEXT NOT NULL CHECK (observation_freshness IN ('FRESH','STALE','UNKNOWN')),
    execution_status TEXT NOT NULL CHECK (execution_status IN ('succeeded','failed','running','unknown')),
    external_probe_status TEXT NOT NULL CHECK (external_probe_status IN ('passed','failed','not_run')),
    external_probe_url TEXT,
    volume_status TEXT NOT NULL CHECK (volume_status IN ('passed','failed','not_run')),
    pvc_name TEXT,
    marker_sha256 TEXT,
    ready BOOLEAN NOT NULL DEFAULT false,
    reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
    checked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (binding_id, revision)
);

CREATE INDEX IF NOT EXISTS idx_application_delivery_verification_tenant
    ON application_delivery_verifications(tenant_id, target_id, checked_at DESC);

COMMIT;
