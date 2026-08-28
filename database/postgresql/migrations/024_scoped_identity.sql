-- Migration: 024_scoped_identity
-- Description: Add canonical subjects, memberships, policy versions, and scoped bindings
-- Tiers: All
-- Dependencies: 005_identity_core, 021_workspace_hierarchy

BEGIN;

CREATE TABLE IF NOT EXISTS identity_subjects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    issuer TEXT NOT NULL,
    external_subject TEXT NOT NULL,
    subject_type TEXT NOT NULL CHECK (subject_type IN ('user', 'workload', 'service')),
    display_name TEXT,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    is_disabled BOOLEAN NOT NULL DEFAULT false,
    disabled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (issuer, external_subject),
    CHECK ((is_disabled AND disabled_at IS NOT NULL) OR NOT is_disabled)
);

CREATE TABLE IF NOT EXISTS tenant_memberships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    subject_id UUID NOT NULL REFERENCES identity_subjects(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'revoked')),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    is_default BOOLEAN NOT NULL DEFAULT false,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, subject_id),
    CHECK (valid_until IS NULL OR valid_until > valid_from)
);

CREATE INDEX IF NOT EXISTS idx_tenant_memberships_subject ON tenant_memberships(subject_id);
CREATE INDEX IF NOT EXISTS idx_tenant_memberships_active
    ON tenant_memberships(tenant_id, subject_id) WHERE status = 'active';

CREATE TABLE IF NOT EXISTS authorization_policy_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    policy_key TEXT NOT NULL,
    version BIGINT NOT NULL CHECK (version > 0),
    policy_document JSONB NOT NULL,
    policy_digest TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'retired')),
    created_by_subject_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    activated_at TIMESTAMPTZ,
    retired_at TIMESTAMPTZ,
    UNIQUE (tenant_id, policy_key, version),
    UNIQUE (id, tenant_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_authorization_policy_active
    ON authorization_policy_versions(tenant_id, policy_key) WHERE status = 'active';

CREATE TABLE IF NOT EXISTS scoped_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    permissions JSONB NOT NULL DEFAULT '[]',
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name),
    UNIQUE (id, tenant_id)
);

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'namespaces'
          AND column_name IN ('project_id', 'environment_id')
        GROUP BY table_schema, table_name
        HAVING count(*) = 2
    ) THEN
        CREATE UNIQUE INDEX IF NOT EXISTS idx_namespaces_tenant_hierarchy
            ON namespaces(id, tenant_id, project_id, environment_id);
    END IF;
END
$$;

CREATE TABLE IF NOT EXISTS scoped_role_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    subject_id UUID NOT NULL,
    role_id UUID NOT NULL,
    scope_kind TEXT NOT NULL CHECK (scope_kind IN ('tenant', 'workspace', 'project', 'environment', 'namespace', 'resource')),
    workspace_id UUID,
    project_id TEXT,
    environment_id TEXT,
    namespace_id TEXT,
    resource_kind TEXT,
    resource_id TEXT,
    actions TEXT[] NOT NULL DEFAULT '{}',
    granted_by_subject_id UUID,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    FOREIGN KEY (tenant_id, subject_id)
        REFERENCES tenant_memberships(tenant_id, subject_id) ON DELETE RESTRICT,
    FOREIGN KEY (role_id, tenant_id)
        REFERENCES scoped_roles(id, tenant_id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, tenant_id)
        REFERENCES workspaces(id, tenant_id) ON DELETE RESTRICT,
    FOREIGN KEY (project_id, tenant_id, workspace_id)
        REFERENCES projects(id, tenant_id, workspace_id) ON DELETE RESTRICT,
    FOREIGN KEY (environment_id, tenant_id, project_id, workspace_id)
        REFERENCES environments(id, tenant_id, project_id, workspace_id) ON DELETE RESTRICT,
    FOREIGN KEY (namespace_id, tenant_id, project_id, environment_id)
        REFERENCES namespaces(id, tenant_id, project_id, environment_id) ON DELETE RESTRICT,
    CHECK (scope_kind <> 'workspace' OR workspace_id IS NOT NULL),
    CHECK (scope_kind <> 'project' OR (workspace_id IS NOT NULL AND project_id IS NOT NULL)),
    CHECK (scope_kind <> 'environment' OR environment_id IS NOT NULL),
    CHECK (scope_kind <> 'namespace' OR namespace_id IS NOT NULL),
    CHECK (scope_kind <> 'resource' OR (resource_kind IS NOT NULL AND resource_id IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_scoped_role_bindings_subject
    ON scoped_role_bindings(tenant_id, subject_id) WHERE revoked_at IS NULL;
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'scoped_role_bindings'
          AND column_name IN ('project_id', 'environment_id')
        GROUP BY table_schema, table_name
        HAVING count(*) = 2
    ) THEN
        CREATE INDEX IF NOT EXISTS idx_scoped_role_bindings_scope
            ON scoped_role_bindings(tenant_id, scope_kind, workspace_id, project_id, environment_id, namespace_id);
    END IF;
END
$$;

CREATE TABLE IF NOT EXISTS scoped_policy_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    policy_version_id UUID NOT NULL,
    subject_id UUID,
    role_id UUID,
    scope_kind TEXT NOT NULL CHECK (scope_kind IN ('tenant', 'workspace', 'project', 'environment', 'namespace', 'resource')),
    scope_selector JSONB NOT NULL DEFAULT '{}',
    actions TEXT[] NOT NULL DEFAULT '{}',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (policy_version_id, tenant_id)
        REFERENCES authorization_policy_versions(id, tenant_id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, subject_id)
        REFERENCES tenant_memberships(tenant_id, subject_id) ON DELETE RESTRICT,
    FOREIGN KEY (role_id, tenant_id)
        REFERENCES scoped_roles(id, tenant_id) ON DELETE RESTRICT,
    CHECK (subject_id IS NOT NULL OR role_id IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_scoped_policy_bindings_policy
    ON scoped_policy_bindings(tenant_id, policy_version_id) WHERE is_active;

COMMIT;
