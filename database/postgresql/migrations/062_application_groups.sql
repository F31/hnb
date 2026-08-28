-- 062: application_groups
-- Application groups for microservice deployment model
-- A group is the tenant isolation boundary for microservices

CREATE TABLE IF NOT EXISTS application_groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    workspace_id UUID REFERENCES workspaces(id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    description TEXT,
    namespace TEXT,
    group_type TEXT NOT NULL DEFAULT 'custom' CHECK (group_type IN ('springcloud', 'istio', 'custom')),
    labels JSONB DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleted')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_app_groups_tenant ON application_groups(tenant_id);
CREATE INDEX IF NOT EXISTS idx_app_groups_workspace ON application_groups(workspace_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_app_groups_tenant_name ON application_groups(tenant_id, name);

ALTER TABLE applications ADD COLUMN IF NOT EXISTS group_id UUID REFERENCES application_groups(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_applications_group ON applications(group_id);