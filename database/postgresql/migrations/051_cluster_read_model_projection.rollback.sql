-- Migration: 051_cluster_read_model_projection (rollback-safe companion)
-- Projection data and additive columns are intentionally retained so an
-- emergency application rollback cannot destroy last-known state or evidence.
-- Dropping query indexes disables the new read path; replaying 051 recreates it.

DROP INDEX IF EXISTS idx_operation_read_model_target_tags;
DROP INDEX IF EXISTS idx_operations_target_tags;
DROP INDEX IF EXISTS idx_runtime_target_observation_inbox_pending;
DROP INDEX IF EXISTS idx_runtime_target_nodes_tenant_target_status;
DROP INDEX IF EXISTS idx_runtime_target_nodes_tenant_target_name;
DROP INDEX IF EXISTS idx_runtime_targets_cluster_states;
DROP INDEX IF EXISTS idx_runtime_targets_cluster_stable;
