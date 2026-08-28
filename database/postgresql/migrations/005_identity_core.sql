-- Migration: 005_identity_core
-- Description: Create Tenant, Project, Environment core tables
-- Tiers: All
-- Dependencies: 004_alert_channel_delivery

-- 1. Tenants
CREATE TABLE IF NOT EXISTS tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    display_name TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'suspended', 'deleted')) DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_tenants_status ON tenants(status);

-- 2-4. Legacy Project/Environment hierarchy. Migration 060 replaces this shape
-- with Tenant/Workspace/Namespace, so replay must not recreate retired tables.
DO $$
BEGIN
    IF to_regclass('namespaces') IS NULL OR EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'namespaces'
          AND column_name = 'environment_id'
    ) THEN
        CREATE TABLE IF NOT EXISTS projects (
            id TEXT PRIMARY KEY,
            tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
            name TEXT NOT NULL,
            created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
        );

        CREATE INDEX IF NOT EXISTS idx_projects_tenant_id ON projects(tenant_id);
        CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_tenant_name ON projects(tenant_id, name);

        CREATE TABLE IF NOT EXISTS environments (
            id TEXT PRIMARY KEY,
            tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
            project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
            name TEXT NOT NULL,
            env_type TEXT NOT NULL CHECK (env_type IN ('production', 'staging', 'development')) DEFAULT 'development',
            created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
        );

        CREATE INDEX IF NOT EXISTS idx_environments_tenant_id ON environments(tenant_id);
        CREATE INDEX IF NOT EXISTS idx_environments_project_id ON environments(project_id);
        CREATE UNIQUE INDEX IF NOT EXISTS idx_environments_project_name ON environments(project_id, name);

        CREATE TABLE IF NOT EXISTS namespaces (
            id TEXT PRIMARY KEY DEFAULT gen_random_uuid(),
            tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
            environment_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
            project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
            name TEXT NOT NULL,
            description TEXT,
            status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleted')),
            labels JSONB DEFAULT '{}',
            created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
        );
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_namespaces_tenant_id ON namespaces(tenant_id);
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'namespaces' AND column_name = 'environment_id') THEN
        CREATE INDEX IF NOT EXISTS idx_namespaces_environment_id ON namespaces(environment_id);
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'namespaces' AND column_name = 'project_id') THEN
        CREATE INDEX IF NOT EXISTS idx_namespaces_project_id ON namespaces(project_id);
    END IF;
END $$;
CREATE INDEX IF NOT EXISTS idx_namespaces_status ON namespaces(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_namespaces_tenant_name ON namespaces(tenant_id, name);
