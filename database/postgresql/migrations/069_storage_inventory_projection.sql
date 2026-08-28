-- 069: storage_inventory_projection
-- Description: Add tenant-safe read models for ordered Kubernetes storage observations.
-- Dependencies: 051_cluster_read_model_projection

BEGIN;

CREATE TABLE IF NOT EXISTS runtime_target_storage_inventory (
    tenant_id TEXT NOT NULL,
    target_id UUID NOT NULL,
    resource_kind TEXT NOT NULL CHECK (resource_kind IN (
        'StorageClass', 'CSIDriver', 'CSINode', 'CSIStorageCapacity',
        'VolumeAttachment', 'VolumeSnapshotClass', 'VolumeSnapshot',
        'VolumeSnapshotContent'
    )),
    resource_uid TEXT NOT NULL,
    resource_version TEXT NOT NULL,
    name TEXT NOT NULL,
    namespace TEXT,
    driver_name TEXT,
    source TEXT NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    stale_after TIMESTAMPTZ NOT NULL,
    observation_source TEXT NOT NULL CHECK (observation_source = 'agent'),
    observation_source_id TEXT NOT NULL,
    observation_generation BIGINT NOT NULL CHECK (observation_generation >= 1),
    observation_revision BIGINT NOT NULL CHECK (observation_revision >= 1),
    attributes JSONB NOT NULL DEFAULT '{}',
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, target_id, resource_kind, resource_uid),
    FOREIGN KEY (target_id, tenant_id) REFERENCES runtime_targets(id, tenant_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS runtime_target_storage_snapshot_api (
    tenant_id TEXT NOT NULL,
    target_id UUID NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('Installed', 'NotInstalled', 'Unsupported')),
    api_version TEXT,
    source TEXT NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    stale_after TIMESTAMPTZ NOT NULL,
    observation_source TEXT NOT NULL CHECK (observation_source = 'agent'),
    observation_source_id TEXT NOT NULL,
    observation_generation BIGINT NOT NULL CHECK (observation_generation >= 1),
    observation_revision BIGINT NOT NULL CHECK (observation_revision >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, target_id),
    FOREIGN KEY (target_id, tenant_id) REFERENCES runtime_targets(id, tenant_id) ON DELETE CASCADE,
    CHECK (status <> 'Installed' OR api_version IS NOT NULL)
);

-- Evidence remains separate so registration, node presence, and references can
-- be evaluated together later without any one row asserting driver readiness.
CREATE TABLE IF NOT EXISTS runtime_target_storage_driver_evidence (
    tenant_id TEXT NOT NULL,
    target_id UUID NOT NULL,
    evidence_kind TEXT NOT NULL CHECK (evidence_kind IN (
        'CSIDriverRegistration', 'CSINodeRegistration', 'StorageClassReference',
        'VolumeAttachmentReference', 'VolumeSnapshotClassReference',
        'VolumeSnapshotContentReference'
    )),
    resource_kind TEXT NOT NULL,
    resource_uid TEXT NOT NULL,
    driver_name TEXT NOT NULL,
    source TEXT NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    stale_after TIMESTAMPTZ NOT NULL,
    observation_source TEXT NOT NULL CHECK (observation_source = 'agent'),
    observation_source_id TEXT NOT NULL,
    observation_generation BIGINT NOT NULL CHECK (observation_generation >= 1),
    observation_revision BIGINT NOT NULL CHECK (observation_revision >= 1),
    details JSONB NOT NULL DEFAULT '{}',
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, target_id, evidence_kind, resource_uid, driver_name),
    FOREIGN KEY (target_id, tenant_id) REFERENCES runtime_targets(id, tenant_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_storage_inventory_target_kind_active
    ON runtime_target_storage_inventory(tenant_id, target_id, resource_kind, name, resource_uid)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_storage_inventory_driver_active
    ON runtime_target_storage_inventory(tenant_id, target_id, driver_name, resource_kind)
    WHERE deleted_at IS NULL AND driver_name IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_storage_inventory_freshness
    ON runtime_target_storage_inventory(tenant_id, stale_after, target_id)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_storage_driver_evidence_active
    ON runtime_target_storage_driver_evidence(tenant_id, target_id, driver_name, evidence_kind)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_storage_driver_evidence_freshness
    ON runtime_target_storage_driver_evidence(tenant_id, stale_after, target_id)
    WHERE deleted_at IS NULL;

COMMIT;
