-- Migration: 037_artifact_gc
-- Description: Add reference-safe artifact GC metadata: references, tombstones and leased locks.
-- Tiers: All
-- Dependencies: 036_artifact_distribution_targets, 008_operation_engine_core

CREATE TABLE IF NOT EXISTS artifact_references (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    artifact_id UUID NOT NULL REFERENCES artifacts(id) ON DELETE RESTRICT,
    owner_type TEXT NOT NULL CHECK (owner_type IN ('release', 'runtime', 'rollback', 'composition', 'dr_snapshot', 'offline_bundle')),
    owner_id TEXT NOT NULL,
    purpose TEXT NOT NULL DEFAULT 'runtime',
    expires_at TIMESTAMPTZ,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_artifact_references_owner
    ON artifact_references(tenant_id, artifact_id, owner_type, owner_id, purpose);
CREATE INDEX IF NOT EXISTS idx_artifact_references_artifact
    ON artifact_references(tenant_id, artifact_id, expires_at);

CREATE TABLE IF NOT EXISTS artifact_tombstones (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    artifact_id UUID NOT NULL REFERENCES artifacts(id) ON DELETE RESTRICT,
    artifact_digest TEXT NOT NULL CHECK (artifact_digest ~ '^sha256:[0-9a-f]{64}$'),
    state TEXT NOT NULL DEFAULT 'retained' CHECK (state IN ('retained', 'deleting', 'deleted', 'cancelled')),
    delete_after TIMESTAMPTZ NOT NULL,
    operation_id UUID REFERENCES operations(id) ON DELETE SET NULL,
    requested_by TEXT NOT NULL,
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_artifact_tombstones_due
    ON artifact_tombstones(tenant_id, state, delete_after);
CREATE INDEX IF NOT EXISTS idx_artifact_tombstones_artifact
    ON artifact_tombstones(tenant_id, artifact_id, state);

CREATE TABLE IF NOT EXISTS artifact_locks (
    artifact_id UUID PRIMARY KEY REFERENCES artifacts(id) ON DELETE RESTRICT,
    tenant_id TEXT NOT NULL,
    lock_owner TEXT NOT NULL,
    lease_until TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_artifact_locks_tenant_lease
    ON artifact_locks(tenant_id, lease_until);
