-- Migration: 006_identity_rbac
-- Description: Create Role, UserRole, ApprovalPolicy tables
-- Tiers: All
-- Dependencies: 005_identity_core

-- 1. Roles
CREATE TABLE IF NOT EXISTS roles (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (name IN ('platform_admin', 'tenant_admin', 'project_admin', 'operator', 'publisher', 'readonly')),
    permissions JSONB NOT NULL DEFAULT '[]',
    inherits_from TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_roles_tenant_id ON roles(tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_roles_tenant_name ON roles(tenant_id, name);

-- 2. UserRoles
CREATE TABLE IF NOT EXISTS user_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id TEXT,
    role_id TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    granted_by TEXT NOT NULL,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_user_roles_user_id ON user_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_tenant_id ON user_roles(tenant_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_role_id ON user_roles(role_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_roles_active ON user_roles(user_id, tenant_id, project_id, role_id) WHERE revoked_at IS NULL;

-- 3. ApprovalPolicies
CREATE TABLE IF NOT EXISTS approval_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    operation_type TEXT NOT NULL,
    required_roles JSONB NOT NULL DEFAULT '[]',
    max_pending_duration TEXT NOT NULL DEFAULT '1h',
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_approval_policies_tenant_id ON approval_policies(tenant_id);
CREATE INDEX IF NOT EXISTS idx_approval_policies_operation_type ON approval_policies(operation_type);
CREATE UNIQUE INDEX IF NOT EXISTS idx_approval_policies_tenant_operation ON approval_policies(tenant_id, operation_type);