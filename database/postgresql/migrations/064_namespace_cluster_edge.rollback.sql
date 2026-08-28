-- 064 rollback: namespace_cluster_edge

BEGIN;

ALTER TABLE namespaces DROP CONSTRAINT IF EXISTS fk_namespaces_cluster_tenant;
ALTER TABLE namespaces DROP COLUMN IF EXISTS cluster_id;

-- NOTE: only valid after any NULL workspace_id rows are cleaned up.
ALTER TABLE runtime_targets ALTER COLUMN workspace_id SET NOT NULL;

COMMIT;
