-- Migration: 057_node_identity_source_uid
-- Description: Drop the legacy (target_id, name) uniqueness on
--              runtime_target_nodes. RT-008 keys nodes by the target-stable
--              nodeId (source_node_uid), which is enforced by the partial
--              unique index idx_runtime_target_nodes_source_uid created in
--              migration 051. The old name-based uniqueness would reject two
--              nodes sharing a name/empty name and conflicts with
--              nodeId-keyed upserts.
-- Dependencies: 048_runtime_target_nodes, 051_cluster_read_model_projection

BEGIN;

ALTER TABLE runtime_target_nodes DROP CONSTRAINT IF EXISTS runtime_target_nodes_target_id_name_key;

COMMIT;
