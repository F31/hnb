-- 065 rollback: shared_clusters

BEGIN;

ALTER TABLE namespaces DROP CONSTRAINT IF EXISTS fk_namespaces_cluster;

DROP TABLE IF EXISTS cluster_shares;

ALTER TABLE runtime_targets ALTER COLUMN tenant_id SET NOT NULL;

COMMIT;