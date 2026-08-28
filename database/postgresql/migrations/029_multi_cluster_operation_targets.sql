-- 029_multi_cluster_operation_targets.sql
-- Add target_cluster_ids to operations and operation_read_model for cross-cluster tracking

BEGIN;

ALTER TABLE operations ADD COLUMN IF NOT EXISTS target_cluster_ids TEXT[] DEFAULT '{}';
ALTER TABLE operation_read_model ADD COLUMN IF NOT EXISTS target_cluster_ids TEXT[] DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_operations_target_clusters ON operations USING GIN(target_cluster_ids);
CREATE INDEX IF NOT EXISTS idx_operation_rm_target_clusters ON operation_read_model USING GIN(target_cluster_ids);

COMMIT;