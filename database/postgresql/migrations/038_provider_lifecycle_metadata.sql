-- Migration: 038_provider_lifecycle_metadata
-- Description: Add provider lifecycle, capability and raw navigation metadata snapshots.
-- Tiers: T1
-- Dependencies: 022_extension_framework, 030_provider_conformance_core, 008_operation_engine_core

CREATE TABLE IF NOT EXISTS provider_lifecycle_states (
    provider_id TEXT NOT NULL,
    provider_version TEXT NOT NULL,
    phase TEXT NOT NULL CHECK (phase IN ('candidate', 'installing', 'enabled', 'degraded', 'rolling_back', 'disabled', 'uninstalling')),
    bundle_digest TEXT NOT NULL CHECK (bundle_digest ~ '^sha256:[0-9a-f]{64}$'),
    operation_id UUID REFERENCES operations(id) ON DELETE SET NULL,
    idempotency_key TEXT NOT NULL,
    previous_version TEXT,
    health TEXT NOT NULL DEFAULT 'unknown' CHECK (health IN ('unknown', 'healthy', 'degraded', 'unavailable')),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (provider_id, provider_version)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_lifecycle_idempotency
    ON provider_lifecycle_states(provider_id, idempotency_key);

CREATE TABLE IF NOT EXISTS provider_capability_registrations (
    provider_id TEXT NOT NULL,
    provider_version TEXT NOT NULL,
    capability_id TEXT NOT NULL,
    capability_version TEXT NOT NULL DEFAULT 'v1',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (provider_id, provider_version, capability_id),
    FOREIGN KEY (provider_id, provider_version) REFERENCES provider_lifecycle_states(provider_id, provider_version) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS provider_navigation_metadata (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id TEXT NOT NULL,
    provider_version TEXT NOT NULL,
    route_path TEXT NOT NULL,
    menu_title TEXT NOT NULL,
    permission TEXT NOT NULL,
    plugin_id TEXT NOT NULL,
    component_key TEXT NOT NULL,
    locale TEXT NOT NULL DEFAULT 'zh-CN',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (provider_id, provider_version) REFERENCES provider_lifecycle_states(provider_id, provider_version) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_provider_navigation_metadata_provider
    ON provider_navigation_metadata(provider_id, provider_version);
