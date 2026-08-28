-- Migration: 015_edge_runtime_target
-- Description: Add edge_type and edge_config to runtime_targets for EdgeRuntimeTarget
-- Tiers: T3
-- Dependencies: 010_runtime_target_engine (runtime_targets), 014_edge_distribution

ALTER TABLE runtime_targets
    ADD COLUMN IF NOT EXISTS edge_type TEXT CHECK (edge_type IN ('kubeedge', 'openyurt', 'superedge', 'custom')),
    ADD COLUMN IF NOT EXISTS edge_config JSONB DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_runtime_targets_edge_type ON runtime_targets(edge_type);
