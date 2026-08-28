-- Migration: 020_provider_bindings
-- Description: Add provider_bindings table for CNI provider lifecycle management
-- Tiers: All
-- Dependencies: 010_runtime_target_engine (runtime_targets)

CREATE TABLE IF NOT EXISTS provider_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    version TEXT NOT NULL DEFAULT '',
    phase TEXT NOT NULL DEFAULT 'pending'
        CHECK (phase IN ('pending', 'installing', 'ready', 'degraded', 'uninstalling')),
    ref_count INTEGER NOT NULL DEFAULT 0,
    health_failures INTEGER NOT NULL DEFAULT 0,
    last_health_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_provider_bindings_cluster ON provider_bindings(cluster_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_bindings_cluster_provider ON provider_bindings(cluster_id, provider);
CREATE INDEX IF NOT EXISTS idx_provider_bindings_phase ON provider_bindings(phase);