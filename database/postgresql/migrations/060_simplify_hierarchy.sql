-- 060: simplify_hierarchy
-- Simplify entity hierarchy: Tenant → Workspace → Namespace
-- Remove Project and Environment (intermediate levels)

BEGIN;

-- 1. Drop FK constraints on scoped_role_bindings referencing old tables
ALTER TABLE IF EXISTS scoped_role_bindings DROP CONSTRAINT IF EXISTS scoped_role_bindings_namespace_id_tenant_id_project_id_env_fkey;
ALTER TABLE IF EXISTS scoped_role_bindings DROP CONSTRAINT IF EXISTS scoped_role_bindings_environment_id_tenant_id_project_id_w_fkey;
ALTER TABLE IF EXISTS scoped_role_bindings DROP CONSTRAINT IF EXISTS scoped_role_bindings_project_id_tenant_id_workspace_id_fkey;

-- 2. Drop FK constraints on runtime_targets referencing old tables
ALTER TABLE IF EXISTS runtime_targets DROP CONSTRAINT IF EXISTS fk_runtime_targets_environment_tenant;
ALTER TABLE IF EXISTS runtime_targets DROP CONSTRAINT IF EXISTS fk_runtime_targets_project_tenant;

-- 3. Drop the chk_runtime_targets_hierarchy constraint
ALTER TABLE IF EXISTS runtime_targets DROP CONSTRAINT IF EXISTS chk_runtime_targets_hierarchy;

-- 4. Replace only the legacy namespace shape. On replay this is already the
-- workspace-scoped table and may contain production data.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'namespaces'
          AND column_name IN ('project_id', 'environment_id')
    ) THEN
        DROP TABLE namespaces CASCADE;
    END IF;
END
$$;

-- 5. Drop environments table
DROP TABLE IF EXISTS environments CASCADE;

-- 6. Drop projects table
DROP TABLE IF EXISTS projects CASCADE;

-- 7. Remove old columns from runtime_targets
ALTER TABLE IF EXISTS runtime_targets DROP COLUMN IF EXISTS project_id;
ALTER TABLE IF EXISTS runtime_targets DROP COLUMN IF EXISTS environment_id;

-- 8. Drop old scope index and drop old columns from scoped_role_bindings
DROP INDEX IF EXISTS idx_scoped_role_bindings_scope;
ALTER TABLE IF EXISTS scoped_role_bindings DROP CONSTRAINT IF EXISTS scoped_role_bindings_check;
ALTER TABLE IF EXISTS scoped_role_bindings DROP CONSTRAINT IF EXISTS scoped_role_bindings_check1;
ALTER TABLE IF EXISTS scoped_role_bindings DROP CONSTRAINT IF EXISTS scoped_role_bindings_check2;
ALTER TABLE IF EXISTS scoped_role_bindings DROP CONSTRAINT IF EXISTS scoped_role_bindings_check3;
ALTER TABLE IF EXISTS scoped_role_bindings DROP CONSTRAINT IF EXISTS scoped_role_bindings_check4;
ALTER TABLE IF EXISTS scoped_role_bindings DROP CONSTRAINT IF EXISTS scoped_role_bindings_scope_kind_check;
ALTER TABLE IF EXISTS scoped_role_bindings DROP CONSTRAINT IF EXISTS scoped_role_bindings_ws_check;
ALTER TABLE IF EXISTS scoped_role_bindings DROP CONSTRAINT IF EXISTS scoped_role_bindings_resource_check;
ALTER TABLE IF EXISTS scoped_role_bindings DROP CONSTRAINT IF EXISTS scoped_role_bindings_ns_check;
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'scoped_role_bindings'
          AND column_name IN ('project_id', 'environment_id')
    ) THEN
        ALTER TABLE scoped_role_bindings DROP COLUMN IF EXISTS namespace_id;
    END IF;
END
$$;
ALTER TABLE IF EXISTS scoped_role_bindings DROP COLUMN IF EXISTS project_id;
ALTER TABLE IF EXISTS scoped_role_bindings DROP COLUMN IF EXISTS environment_id;

-- 9. Update scoped_role_bindings scope_kind CHECK and add new constraints
ALTER TABLE IF EXISTS scoped_role_bindings ADD CONSTRAINT scoped_role_bindings_scope_kind_check
    CHECK (scope_kind IN ('tenant', 'workspace', 'namespace', 'resource'));
ALTER TABLE IF EXISTS scoped_role_bindings ADD CONSTRAINT scoped_role_bindings_ws_check
    CHECK (scope_kind <> 'workspace'::text OR workspace_id IS NOT NULL);
ALTER TABLE IF EXISTS scoped_role_bindings ADD CONSTRAINT scoped_role_bindings_resource_check
    CHECK (scope_kind <> 'resource'::text OR resource_kind IS NOT NULL AND resource_id IS NOT NULL);

-- 10. Create new namespaces table
CREATE TABLE IF NOT EXISTS namespaces (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    labels JSONB DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleted')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_namespaces_tenant_id ON namespaces(tenant_id);
CREATE INDEX IF NOT EXISTS idx_namespaces_workspace_id ON namespaces(workspace_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_namespaces_tenant_name ON namespaces(tenant_id, name);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_namespaces_workspace_tenant') THEN
        ALTER TABLE namespaces ADD CONSTRAINT fk_namespaces_workspace_tenant
            FOREIGN KEY (workspace_id, tenant_id)
            REFERENCES workspaces(id, tenant_id) ON DELETE RESTRICT;
    END IF;
END
$$;

-- 11. Add unique constraint on namespaces(id, tenant_id) for FK reference
CREATE UNIQUE INDEX IF NOT EXISTS idx_namespaces_id_tenant ON namespaces(id, tenant_id);

-- 12. Add namespace_id back to scoped_role_bindings with new FK
ALTER TABLE IF EXISTS scoped_role_bindings ADD COLUMN IF NOT EXISTS namespace_id TEXT;
ALTER TABLE IF EXISTS scoped_role_bindings ADD CONSTRAINT scoped_role_bindings_ns_check
    CHECK (scope_kind <> 'namespace'::text OR namespace_id IS NOT NULL);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_scoped_role_bindings_namespace') THEN
        ALTER TABLE scoped_role_bindings ADD CONSTRAINT fk_scoped_role_bindings_namespace
            FOREIGN KEY (namespace_id, tenant_id)
            REFERENCES namespaces(id, tenant_id) ON DELETE RESTRICT;
    END IF;
END
$$;

-- 13. Recreate scope index
CREATE INDEX IF NOT EXISTS idx_scoped_role_bindings_scope ON scoped_role_bindings(tenant_id, scope_kind, workspace_id);

COMMIT;
