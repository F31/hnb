-- 064: namespace_cluster_edge
-- Description: Add the cluster edge to namespaces (Workspace → Cluster → Namespace)
--   and fix the runtime_targets.workspace_id NOT NULL vs. unbind-NULL conflict.
-- Dependencies: 021_workspace_hierarchy, 060_simplify_hierarchy

BEGIN;

-- 1. Allow unbound clusters. Migration 021 set workspace_id NOT NULL but
--    UnbindWorkspaceCluster sets it to NULL; the column must be nullable.
ALTER TABLE runtime_targets ALTER COLUMN workspace_id DROP NOT NULL;

-- 2. Add cluster_id to namespaces (nullable: a namespace may exist without a
--    concrete cluster while one is being provisioned).
ALTER TABLE namespaces ADD COLUMN IF NOT EXISTS cluster_id UUID;

-- 3. Enable a composite FK on (id, tenant_id) so the DB enforces that a
--    namespace's cluster belongs to the same tenant (id is already PK).
CREATE UNIQUE INDEX IF NOT EXISTS idx_runtime_targets_id_tenant ON runtime_targets(id, tenant_id);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_namespaces_cluster_tenant') THEN
        ALTER TABLE namespaces ADD CONSTRAINT fk_namespaces_cluster_tenant
            FOREIGN KEY (cluster_id, tenant_id)
            REFERENCES runtime_targets(id, tenant_id) ON DELETE SET NULL;
    END IF;
END
$$;

COMMIT;
