-- 065: shared_clusters
-- Description: Enable shared clusters across tenants and workspaces.
--   - runtime_targets.tenant_id becomes nullable (platform-level clusters)
--   - cluster_shares table grants access to other tenants/workspaces
--   - namespaces.cluster_id FK changes from (id,tenant_id) to plain (id)
-- Dependencies: 064_namespace_cluster_edge

BEGIN;

-- 1. Cluster ownership becomes optional (NULL = platform-level shared cluster)
ALTER TABLE runtime_targets ALTER COLUMN tenant_id DROP NOT NULL;

-- 2. Cluster shares table: grant cluster access to tenants/workspaces
CREATE TABLE IF NOT EXISTS cluster_shares (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id UUID NOT NULL REFERENCES runtime_targets(id) ON DELETE CASCADE,
    grantee_tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    grantee_workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    permissions TEXT[] NOT NULL DEFAULT ARRAY['read', 'deploy'] CHECK (
        array_length(permissions, 1) > 0
        AND permissions <@ ARRAY['read', 'deploy', 'admin']
    ),
    k8s_namespace_prefix TEXT NOT NULL DEFAULT 't-{tenant}-w-{workspace}',
    tenant_isolation_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by_subject_id UUID
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_cluster_shares_grantee
    ON cluster_shares(cluster_id, grantee_tenant_id, COALESCE(grantee_workspace_id::text, ''));
CREATE INDEX IF NOT EXISTS idx_cluster_shares_grantee ON cluster_shares(grantee_tenant_id, grantee_workspace_id);
CREATE INDEX IF NOT EXISTS idx_cluster_shares_cluster ON cluster_shares(cluster_id);

-- 3. Relax namespace cluster FK: no longer enforced same-tenant
ALTER TABLE namespaces DROP CONSTRAINT IF EXISTS fk_namespaces_cluster_tenant;
ALTER TABLE namespaces DROP CONSTRAINT IF EXISTS fk_namespaces_cluster;
ALTER TABLE namespaces ADD CONSTRAINT fk_namespaces_cluster
    FOREIGN KEY (cluster_id) REFERENCES runtime_targets(id) ON DELETE SET NULL;

COMMIT;
