-- Migration: 021_workspace_hierarchy
-- Description: Reconcile the workspace hierarchy with the TEXT identifiers owned by 005
-- Tiers: All
-- Dependencies: 005_identity_core, 010_runtime_target_engine

BEGIN;

CREATE TABLE IF NOT EXISTS workspaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    display_name TEXT,
    labels JSONB NOT NULL DEFAULT '{}',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

UPDATE workspaces SET labels = '{}' WHERE labels IS NULL;
ALTER TABLE workspaces
    ALTER COLUMN labels SET DEFAULT '{}',
    ALTER COLUMN labels SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_workspaces_tenant') THEN
        ALTER TABLE workspaces ADD CONSTRAINT fk_workspaces_tenant
            FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE RESTRICT;
    END IF;
END
$$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_workspaces_name ON workspaces(tenant_id, name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_workspaces_id_tenant ON workspaces(id, tenant_id);

INSERT INTO workspaces (tenant_id, name, display_name)
SELECT id, 'default', 'Default' FROM tenants
ON CONFLICT (tenant_id, name) DO NOTHING;

DO $$
BEGIN
    IF to_regclass('projects') IS NOT NULL THEN
        ALTER TABLE projects
            ADD COLUMN IF NOT EXISTS workspace_id UUID,
            ADD COLUMN IF NOT EXISTS display_name TEXT,
            ADD COLUMN IF NOT EXISTS labels JSONB NOT NULL DEFAULT '{}',
            ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true;

        UPDATE projects p
        SET workspace_id = w.id
        FROM workspaces w
        WHERE p.workspace_id IS NULL
          AND w.tenant_id = p.tenant_id
          AND w.name = 'default';

        ALTER TABLE projects ALTER COLUMN workspace_id SET NOT NULL;
        CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_name ON projects(workspace_id, name);
        CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_tenant_hierarchy
            ON projects(id, tenant_id, workspace_id);

        IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_projects_workspace_tenant') THEN
            ALTER TABLE projects ADD CONSTRAINT fk_projects_workspace_tenant
                FOREIGN KEY (workspace_id, tenant_id)
                REFERENCES workspaces(id, tenant_id) ON DELETE RESTRICT;
        END IF;
    END IF;
END
$$;

DO $$
BEGIN
    IF to_regclass('environments') IS NOT NULL AND to_regclass('projects') IS NOT NULL THEN
        ALTER TABLE environments
            ADD COLUMN IF NOT EXISTS workspace_id UUID,
            ADD COLUMN IF NOT EXISTS type TEXT,
            ADD COLUMN IF NOT EXISTS labels JSONB NOT NULL DEFAULT '{}',
            ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true;

        UPDATE environments e
        SET workspace_id = p.workspace_id,
            type = COALESCE(e.type, e.env_type)
        FROM projects p
        WHERE e.project_id = p.id
          AND e.tenant_id = p.tenant_id
          AND (e.workspace_id IS NULL OR e.type IS NULL);

        ALTER TABLE environments
            ALTER COLUMN workspace_id SET NOT NULL,
            ALTER COLUMN type SET DEFAULT 'development',
            ALTER COLUMN type SET NOT NULL;

        CREATE UNIQUE INDEX IF NOT EXISTS idx_environments_name ON environments(workspace_id, project_id, name);
        CREATE UNIQUE INDEX IF NOT EXISTS idx_environments_tenant_hierarchy
            ON environments(id, tenant_id, project_id, workspace_id);

        IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_environments_project_tenant') THEN
            ALTER TABLE environments ADD CONSTRAINT fk_environments_project_tenant
                FOREIGN KEY (project_id, tenant_id, workspace_id)
                REFERENCES projects(id, tenant_id, workspace_id) ON DELETE RESTRICT;
        END IF;
    END IF;
END
$$;

ALTER TABLE runtime_targets
    ADD COLUMN IF NOT EXISTS workspace_id UUID;

DO $$
BEGIN
    IF to_regclass('projects') IS NOT NULL AND to_regclass('environments') IS NOT NULL THEN
        ALTER TABLE runtime_targets
            ADD COLUMN IF NOT EXISTS project_id TEXT,
            ADD COLUMN IF NOT EXISTS environment_id TEXT;
    END IF;
END
$$;

UPDATE runtime_targets rt
SET workspace_id = w.id
FROM workspaces w
WHERE rt.workspace_id IS NULL
  AND w.tenant_id = rt.tenant_id
  AND w.name = 'default';

ALTER TABLE runtime_targets ALTER COLUMN workspace_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_runtime_targets_workspace_id ON runtime_targets(workspace_id);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_runtime_targets_workspace_tenant') THEN
        ALTER TABLE runtime_targets ADD CONSTRAINT fk_runtime_targets_workspace_tenant
            FOREIGN KEY (workspace_id, tenant_id)
            REFERENCES workspaces(id, tenant_id) ON DELETE RESTRICT;
    END IF;
    IF to_regclass('projects') IS NOT NULL AND to_regclass('environments') IS NOT NULL THEN
        CREATE INDEX IF NOT EXISTS idx_runtime_targets_project_id ON runtime_targets(project_id);
        CREATE INDEX IF NOT EXISTS idx_runtime_targets_environment_id ON runtime_targets(environment_id);

        IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_runtime_targets_project_tenant') THEN
            ALTER TABLE runtime_targets ADD CONSTRAINT fk_runtime_targets_project_tenant
                FOREIGN KEY (project_id, tenant_id, workspace_id)
                REFERENCES projects(id, tenant_id, workspace_id) ON DELETE RESTRICT;
        END IF;
        IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_runtime_targets_environment_tenant') THEN
            ALTER TABLE runtime_targets ADD CONSTRAINT fk_runtime_targets_environment_tenant
                FOREIGN KEY (environment_id, tenant_id, project_id, workspace_id)
                REFERENCES environments(id, tenant_id, project_id, workspace_id) ON DELETE RESTRICT;
        END IF;
        IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_runtime_targets_hierarchy') THEN
            ALTER TABLE runtime_targets ADD CONSTRAINT chk_runtime_targets_hierarchy CHECK (
                (project_id IS NULL AND environment_id IS NULL)
                OR (project_id IS NOT NULL AND (environment_id IS NULL OR workspace_id IS NOT NULL))
            );
        END IF;
    END IF;
END
$$;

COMMIT;
