-- 075: storage_provider_metrics
-- Description: Tenant-safe normalized Provider metric read model.
BEGIN;

CREATE TABLE IF NOT EXISTS storage_metric_snapshots (
    tenant_id TEXT NOT NULL,
    target_id UUID NOT NULL,
    provider_id TEXT NOT NULL CHECK (length(provider_id) BETWEEN 1 AND 128),
    resource_kind TEXT NOT NULL CHECK (resource_kind IN ('StorageBackend', 'WorkloadStorageOffering', 'StorageClassBinding', 'StorageClass', 'PersistentVolumeClaim', 'PersistentVolume')),
    resource_uid TEXT NOT NULL CHECK (length(resource_uid) BETWEEN 1 AND 128),
    metrics JSONB NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    stale_after TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, target_id, provider_id, resource_kind, resource_uid),
    FOREIGN KEY (target_id, tenant_id) REFERENCES runtime_targets(id, tenant_id) ON DELETE CASCADE,
    CHECK (jsonb_typeof(metrics) = 'array' AND jsonb_array_length(metrics) = 6)
);

CREATE INDEX IF NOT EXISTS idx_storage_metric_snapshots_target
    ON storage_metric_snapshots(tenant_id, target_id, provider_id, resource_kind, resource_uid);

COMMIT;
