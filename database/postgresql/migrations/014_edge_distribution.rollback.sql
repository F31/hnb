-- Rollback: 014_edge_distribution
ALTER TABLE runtime_targets
    DROP COLUMN distribution;