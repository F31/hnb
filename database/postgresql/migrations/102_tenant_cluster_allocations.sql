-- 066: tenant_cluster_allocations
-- Shared clusters are platform resources. This table is the authoritative
-- tenant-to-cluster access and quota boundary; Workspace remains optional.

BEGIN;

CREATE TABLE IF NOT EXISTS tenant_cluster_allocations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    cluster_id UUID NOT NULL REFERENCES runtime_targets(id) ON DELETE CASCADE,
    quota JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    namespace_prefix TEXT NOT NULL,
    isolation_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_tenant_cluster_allocations UNIQUE (tenant_id, cluster_id)
);
CREATE INDEX IF NOT EXISTS idx_tenant_cluster_allocations_cluster
    ON tenant_cluster_allocations(cluster_id, status);

-- A namespace may be owned directly by a tenant allocation. Existing
-- workspace-bound namespaces remain valid during the compatibility period.
ALTER TABLE namespaces ALTER COLUMN workspace_id DROP NOT NULL;

-- A physical namespace name only needs to be unique within a cluster. This
-- permits the same tenant to use a conventional name (for example "prod") in
-- different clusters.
DROP INDEX IF EXISTS idx_namespaces_tenant_name;
CREATE UNIQUE INDEX IF NOT EXISTS uq_namespaces_tenant_cluster_name
    ON namespaces(tenant_id, COALESCE(cluster_id::text, ''), name);

COMMIT;
