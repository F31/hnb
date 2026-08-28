-- Migration: 086_clusters_version_scope
-- Description: Add the optimistic-concurrency version and scope columns that the
-- legacy ClusterStore (heartbeat versioning) already reads and writes. The
-- original 017 clusters schema omitted them, so the legacy register/heartbeat
-- path failed at the SQL level.
-- Dependencies: 017_multi_cluster_core

BEGIN;

ALTER TABLE clusters
    ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'tenant';

COMMIT;
