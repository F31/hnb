-- Migration: 016_node_group_affinity
-- Description: Add node_group_affinity to execution_plans for edge node group routing
-- Tiers: T3
-- Dependencies: 008_operation_engine_core (execution_plans)

ALTER TABLE execution_plans
    ADD COLUMN IF NOT EXISTS node_group_affinity TEXT[] DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_execution_plans_node_group_affinity ON execution_plans USING GIN(node_group_affinity);
