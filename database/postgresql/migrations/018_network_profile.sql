-- Migration: 018_network_profile
-- Description: Add network_profiles table for CNI lifecycle management
-- Tiers: All
-- Dependencies: 010_runtime_target_engine (runtime_targets)

CREATE TABLE IF NOT EXISTS network_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    target_id UUID NOT NULL REFERENCES runtime_targets(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    ip_version TEXT NOT NULL DEFAULT 'ipv4',
    pod_cidr TEXT,
    service_cidr TEXT,
    encap_mode TEXT NOT NULL DEFAULT 'vxlan',
    routing_mode TEXT NOT NULL DEFAULT 'tunnel',
    mtu INTEGER DEFAULT 0,
    enable_policy BOOLEAN NOT NULL DEFAULT true,
    enable_hubble BOOLEAN NOT NULL DEFAULT false,
    kube_proxy_replacement TEXT NOT NULL DEFAULT 'disabled',
    ipam_mode TEXT NOT NULL DEFAULT 'cluster-pool',
    version TEXT NOT NULL DEFAULT '',
    extra_config JSONB DEFAULT '{}',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_network_profiles_tenant_id ON network_profiles(tenant_id);
CREATE INDEX IF NOT EXISTS idx_network_profiles_target_id ON network_profiles(target_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_network_profiles_target_provider ON network_profiles(target_id, provider);

CREATE TABLE IF NOT EXISTS cilium_network_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    target_id UUID NOT NULL REFERENCES runtime_targets(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    namespace TEXT NOT NULL DEFAULT 'default',
    spec JSONB NOT NULL DEFAULT '{}',
    labels JSONB DEFAULT '{}',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_cilium_policies_tenant_id ON cilium_network_policies(tenant_id);
CREATE INDEX IF NOT EXISTS idx_cilium_policies_target_id ON cilium_network_policies(target_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_cilium_policies_name ON cilium_network_policies(target_id, namespace, name);