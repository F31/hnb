-- Rollback: 016_node_group_affinity
ALTER TABLE execution_plans
    DROP COLUMN node_group_affinity;