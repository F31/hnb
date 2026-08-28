-- Migration: 014_edge_distribution
-- Description: Add distribution field to runtime_targets for K3s/KubeEdge/standard detection
-- Tiers: All
-- Dependencies: 010_runtime_target_engine (runtime_targets)

ALTER TABLE runtime_targets
    ADD COLUMN IF NOT EXISTS distribution TEXT NOT NULL DEFAULT 'standard'
    CHECK (distribution IN ('standard', 'k3s', 'kubeedge', 'other'));

CREATE INDEX IF NOT EXISTS idx_runtime_targets_distribution ON runtime_targets(distribution);
