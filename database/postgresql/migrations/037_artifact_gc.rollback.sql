-- Rollback: 037_artifact_gc
-- Development-only after proving no production GC metadata is needed.

DROP TABLE IF EXISTS artifact_locks;
DROP TABLE IF EXISTS artifact_tombstones;
DROP TABLE IF EXISTS artifact_references;
