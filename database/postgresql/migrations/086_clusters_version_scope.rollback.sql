-- Rollback: 086_clusters_version_scope
-- WARNING: drops optimistic-concurrency versioning on the legacy clusters path.

BEGIN;

ALTER TABLE clusters
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS scope;

COMMIT;
