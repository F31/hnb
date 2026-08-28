-- 017_multi_cluster_core.rollback.sql
BEGIN;

DROP TABLE IF EXISTS cluster_heartbeats;
DROP TABLE IF EXISTS clusters;

COMMIT;