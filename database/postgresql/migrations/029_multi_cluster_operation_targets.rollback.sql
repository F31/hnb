-- 029_multi_cluster_operation_targets.rollback.sql
BEGIN;

ALTER TABLE operations DROP COLUMN IF EXISTS target_cluster_ids;
ALTER TABLE operation_read_model DROP COLUMN IF EXISTS target_cluster_ids;

COMMIT;