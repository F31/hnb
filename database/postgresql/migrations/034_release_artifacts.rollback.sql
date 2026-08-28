-- Rollback: 034_release_artifacts
-- Development-only after proving no production release reference metadata is needed.

DROP TABLE IF EXISTS release_artifacts;
