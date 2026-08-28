-- Rollback: 036_artifact_distribution_targets
-- Development-only after proving no production distribution metadata is needed.

DROP TABLE IF EXISTS artifact_distribution_targets;
