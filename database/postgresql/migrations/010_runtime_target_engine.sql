-- Migration: 010_runtime_target_engine
-- Description: Add RuntimeTarget, CapabilitySnapshot, ProviderRegistry tables
-- Tiers: All
-- Dependencies: 005_identity_core (tenants)

-- 1. RuntimeTargets (4 classifications)
CREATE TABLE IF NOT EXISTS runtime_targets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    display_name TEXT,
    target_type TEXT NOT NULL CHECK (target_type IN (
        'kubernetes', 'container_engine', 'edge_runtime', 'external_service'
    )),
    connection_type TEXT NOT NULL DEFAULT 'agent' CHECK (connection_type IN ('agent', 'direct', 'cloudhub')),
    connection_endpoint TEXT,
    agent_version TEXT,
    status TEXT NOT NULL DEFAULT 'unknown' CHECK (status IN (
        'online', 'offline', 'unknown', 'degraded', 'decommissioned'
    )),
    labels JSONB DEFAULT '{}',
    observed_at TIMESTAMPTZ,
    stale_threshold_seconds INTEGER NOT NULL DEFAULT 300,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_runtime_targets_tenant_id ON runtime_targets(tenant_id);
CREATE INDEX IF NOT EXISTS idx_runtime_targets_type ON runtime_targets(target_type);
CREATE INDEX IF NOT EXISTS idx_runtime_targets_status ON runtime_targets(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_runtime_targets_tenant_name ON runtime_targets(tenant_id, name);

-- 2. CapabilitySnapshots (versioned capability reports)
CREATE TABLE IF NOT EXISTS capability_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    target_id UUID NOT NULL REFERENCES runtime_targets(id) ON DELETE CASCADE,
    kube_version TEXT,
    arch TEXT,
    cpu_cores INTEGER,
    cpu_model TEXT,
    memory_mb BIGINT,
    storage_gb BIGINT,
    cni_plugins TEXT[] DEFAULT '{}',
    csi_drivers TEXT[] DEFAULT '{}',
    gateway_api_version TEXT,
    gpu_model TEXT,
    gpu_memory_mb BIGINT,
    gpu_count INTEGER,
    features JSONB DEFAULT '{}',
    snapshot_json JSONB NOT NULL DEFAULT '{}',
    observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_capability_snapshots_target_id ON capability_snapshots(target_id);
CREATE INDEX IF NOT EXISTS idx_capability_snapshots_observed_at ON capability_snapshots(target_id, observed_at DESC);

-- 3. ProviderRegistry (maps provider_id to runtime target + provider type)
CREATE TABLE IF NOT EXISTS provider_registry (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id TEXT NOT NULL,
    provider_type TEXT NOT NULL CHECK (provider_type IN (
        'k8s_deploy', 'container_deploy', 'edge_deploy', 'helm', 'terraform', 'custom'
    )),
    runtime_target_id UUID NOT NULL REFERENCES runtime_targets(id) ON DELETE CASCADE,
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    config JSONB DEFAULT '{}',
    capability_pack TEXT,
    is_default BOOLEAN NOT NULL DEFAULT false,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_registry_provider_id ON provider_registry(provider_id);
CREATE INDEX IF NOT EXISTS idx_provider_registry_tenant_id ON provider_registry(tenant_id);
CREATE INDEX IF NOT EXISTS idx_provider_registry_target_id ON provider_registry(runtime_target_id);
CREATE INDEX IF NOT EXISTS idx_provider_registry_type ON provider_registry(provider_type);
CREATE INDEX IF NOT EXISTS idx_provider_registry_default ON provider_registry(is_default) WHERE is_default = true;
