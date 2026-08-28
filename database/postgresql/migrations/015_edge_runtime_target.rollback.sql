-- Rollback: 015_edge_runtime_target
ALTER TABLE runtime_targets
    DROP COLUMN edge_type,
    DROP COLUMN edge_config;