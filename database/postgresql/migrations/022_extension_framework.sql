-- Migration: 022_extension_framework
-- Description: Add extension tables and reconcile the provider registry owned by 010
-- Tiers: All
-- Dependencies: 010_runtime_target_engine, 021_workspace_hierarchy

BEGIN;

CREATE TABLE IF NOT EXISTS extensions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    provider_type TEXT NOT NULL,
    workspace_id UUID REFERENCES workspaces(id) ON DELETE RESTRICT,
    target_id TEXT,
    runtime_target_id UUID REFERENCES runtime_targets(id) ON DELETE RESTRICT,
    phase TEXT NOT NULL DEFAULT 'pending'
        CHECK (phase IN ('pending', 'installing', 'ready', 'degraded', 'uninstalling')),
    manifest JSONB NOT NULL DEFAULT '{}',
    labels JSONB DEFAULT '{}',
    health_failures INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE extensions ADD COLUMN IF NOT EXISTS runtime_target_id UUID;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_extensions_runtime_target') THEN
        ALTER TABLE extensions ADD CONSTRAINT fk_extensions_runtime_target
            FOREIGN KEY (runtime_target_id) REFERENCES runtime_targets(id) ON DELETE RESTRICT;
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_extensions_name ON extensions(name);
CREATE INDEX IF NOT EXISTS idx_extensions_phase ON extensions(phase);
CREATE INDEX IF NOT EXISTS idx_extensions_workspace ON extensions(workspace_id);
CREATE INDEX IF NOT EXISTS idx_extensions_runtime_target ON extensions(runtime_target_id);

ALTER TABLE provider_registry
    ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS target_id UUID;

UPDATE provider_registry
SET target_id = runtime_target_id
WHERE target_id IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_provider_registry_compat_target') THEN
        ALTER TABLE provider_registry ADD CONSTRAINT fk_provider_registry_compat_target
            FOREIGN KEY (target_id) REFERENCES runtime_targets(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_provider_registry_target_alias') THEN
        ALTER TABLE provider_registry ADD CONSTRAINT chk_provider_registry_target_alias
            CHECK (target_id IS NULL OR target_id = runtime_target_id);
    END IF;
END
$$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_registry_id ON provider_registry(provider_id);
CREATE INDEX IF NOT EXISTS idx_provider_registry_target ON provider_registry(target_id);

COMMIT;
