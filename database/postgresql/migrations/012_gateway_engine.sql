-- Migration: 012_gateway_engine
-- Description: Add Gateway API tables: gateway_classes, gateways, gateway_profiles, http_routes, reference_grants, gateway_capability_snapshots
-- Tiers: All
-- Dependencies: 005_identity_core (tenants)

-- 1. GatewayClasses
CREATE TABLE IF NOT EXISTS gateway_classes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    controller_name TEXT NOT NULL,
    description TEXT,
    parameters_ref JSONB,
    is_default BOOLEAN NOT NULL DEFAULT false,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_gateway_classes_name ON gateway_classes(name);

-- 2. Gateways
CREATE TABLE IF NOT EXISTS gateways (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    gateway_class_id UUID NOT NULL REFERENCES gateway_classes(id),
    gw_type TEXT NOT NULL DEFAULT 'standard' CHECK (gw_type IN ('standard', 'api_management', 'mesh', 'ai')),
    listeners JSONB NOT NULL DEFAULT '[]',
    addresses JSONB DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'ready', 'error', 'decommissioned')),
    namespace TEXT NOT NULL DEFAULT 'default',
    labels JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gateways_tenant_id ON gateways(tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_gateways_tenant_name ON gateways(tenant_id, name);

-- 3. GatewayProfiles (user-facing traffic config)
CREATE TABLE IF NOT EXISTS gateway_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    project_id TEXT,
    environment_id TEXT,
    name TEXT NOT NULL,
    gw_type TEXT NOT NULL DEFAULT 'standard' CHECK (gw_type IN ('standard', 'api_management', 'mesh', 'ai')),
    profile_json JSONB NOT NULL DEFAULT '{}',
    profile_digest TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gateway_profiles_tenant_id ON gateway_profiles(tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_gateway_profiles_tenant_name ON gateway_profiles(tenant_id, name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_gateway_profiles_digest ON gateway_profiles(profile_digest);

-- 4. HTTPRoutes
CREATE TABLE IF NOT EXISTS http_routes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    gateway_profile_id UUID NOT NULL REFERENCES gateway_profiles(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    hostnames TEXT[] DEFAULT '{}',
    rules JSONB NOT NULL DEFAULT '[]',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'rejected', 'orphaned')),
    status_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_http_routes_profile_id ON http_routes(gateway_profile_id);
CREATE INDEX IF NOT EXISTS idx_http_routes_status ON http_routes(status);

-- 5. ReferenceGrants (cross-namespace authorization)
CREATE TABLE IF NOT EXISTS reference_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    from_namespace TEXT NOT NULL,
    to_namespace TEXT NOT NULL,
    from_group TEXT NOT NULL DEFAULT 'gateway.networking.k8s.io',
    from_kind TEXT NOT NULL DEFAULT 'HTTPRoute',
    to_group TEXT NOT NULL DEFAULT '',
    to_kind TEXT NOT NULL DEFAULT 'Secret',
    to_name TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_reference_grants_tenant_id ON reference_grants(tenant_id);
CREATE INDEX IF NOT EXISTS idx_reference_grants_from ON reference_grants(from_namespace, from_kind);
CREATE INDEX IF NOT EXISTS idx_reference_grants_to ON reference_grants(to_namespace, to_kind);

-- 6. GatewayCapabilitySnapshots
CREATE TABLE IF NOT EXISTS gateway_capability_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    gateway_class_id UUID NOT NULL REFERENCES gateway_classes(id) ON DELETE CASCADE,
    supported_routes TEXT[] NOT NULL DEFAULT '{}',
    core_features TEXT[] DEFAULT '{}',
    extended_features TEXT[] DEFAULT '{}',
    snapshot_json JSONB NOT NULL DEFAULT '{}',
    observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gw_cap_snapshots_class_id ON gateway_capability_snapshots(gateway_class_id);
CREATE INDEX IF NOT EXISTS idx_gw_cap_snapshots_observed_at ON gateway_capability_snapshots(gateway_class_id, observed_at DESC);
