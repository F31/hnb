-- Migration: 009_config_secret_engine
-- Description: Add ConfigSnapshot, ConfigLayer, KMS Provider tables for config layering and secret resolution
-- Tiers: All
-- Dependencies: 007_identity_secrets (secret_references), 008_operation_engine_core (operations)

-- 1. ConfigLayers (layer definitions with priority)
CREATE TABLE IF NOT EXISTS config_layers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    layer_name TEXT NOT NULL CHECK (layer_name IN ('default', 'tier', 'environment', 'tenant', 'instance')),
    priority INTEGER NOT NULL CHECK (priority BETWEEN 1 AND 5),
    entity_type TEXT,
    entity_id TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_config_layers_entity ON config_layers(entity_type, entity_id) WHERE entity_type IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_config_layers_priority ON config_layers(priority);

-- 2. ConfigValues (key-value pairs per layer)
CREATE TABLE IF NOT EXISTS config_values (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    layer_id UUID NOT NULL REFERENCES config_layers(id) ON DELETE CASCADE,
    config_key TEXT NOT NULL,
    config_value TEXT NOT NULL,
    value_type TEXT NOT NULL DEFAULT 'string' CHECK (value_type IN ('string', 'number', 'boolean', 'json', 'secret_ref')),
    secret_ref_id UUID REFERENCES secret_references(id) ON DELETE SET NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_config_values_layer_key ON config_values(layer_id, config_key);
CREATE INDEX IF NOT EXISTS idx_config_values_secret_ref ON config_values(secret_ref_id);

-- 3. ConfigSnapshots (immutable resolved config)
CREATE TABLE IF NOT EXISTS config_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    config_digest TEXT NOT NULL,
    layers_used JSONB NOT NULL DEFAULT '[]',
    snapshot JSONB NOT NULL,
    superseded_by UUID REFERENCES config_snapshots(id),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'superseded', 'archived')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_config_snapshots_digest ON config_snapshots(config_digest);
CREATE INDEX IF NOT EXISTS idx_config_snapshots_entity ON config_snapshots(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_config_snapshots_status ON config_snapshots(status);

-- 4. KMS Providers registry
CREATE TABLE IF NOT EXISTS kms_providers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_type TEXT NOT NULL CHECK (provider_type IN ('local_aes', 'k8s_secret', 'vault', 'hsm', 'custom')),
    name TEXT NOT NULL,
    description TEXT,
    config JSONB NOT NULL DEFAULT '{}',
    is_default BOOLEAN NOT NULL DEFAULT false,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_kms_providers_name ON kms_providers(name);
CREATE INDEX IF NOT EXISTS idx_kms_providers_default ON kms_providers(is_default) WHERE is_default = true;

-- 5. Add provider_id to secret_references
ALTER TABLE IF EXISTS secret_references
    ADD COLUMN IF NOT EXISTS kms_provider_id UUID REFERENCES kms_providers(id);
