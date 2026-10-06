-- Migration: 098_drop_legacy_clusters_schema
-- Description: Drop legacy clusters/cluster_heartbeats tables after the
-- platform-api /v1/clusters compatibility API was removed. RuntimeTarget and
-- capability_snapshots are the authoritative model.
-- Dependencies: 017_multi_cluster_core, 086_clusters_version_scope

BEGIN;

DROP TABLE IF EXISTS cluster_heartbeats;
DROP TABLE IF EXISTS clusters;

COMMIT;
