-- Migration: 019_cni_capability
-- Description: Add structured CNI capability columns to capability_snapshots
-- Tiers: All
-- Dependencies: 010_runtime_target_engine (capability_snapshots)

ALTER TABLE capability_snapshots
ADD COLUMN IF NOT EXISTS cni_details JSONB DEFAULT '[]'::jsonb;