-- 067: parent/child runtime target batches and platform extension instances
BEGIN;

CREATE TABLE IF NOT EXISTS operation_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('BatchDeleteRuntimeTargets')),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','partial_succeeded','succeeded','failed','cancelled')),
    initiated_by TEXT NOT NULL,
    correlation_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    total_children INTEGER NOT NULL DEFAULT 0,
    succeeded_children INTEGER NOT NULL DEFAULT 0,
    failed_children INTEGER NOT NULL DEFAULT 0,
    cancelled_children INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, idempotency_key)
);

CREATE TABLE IF NOT EXISTS operation_batch_children (
    batch_id UUID NOT NULL REFERENCES operation_batches(id) ON DELETE CASCADE,
    operation_id UUID NOT NULL REFERENCES operations(id) ON DELETE RESTRICT,
    target_id UUID NOT NULL REFERENCES runtime_targets(id) ON DELETE RESTRICT,
    ordinal INTEGER NOT NULL,
    PRIMARY KEY (batch_id, operation_id),
    UNIQUE (batch_id, target_id),
    UNIQUE (batch_id, ordinal)
);
CREATE INDEX IF NOT EXISTS idx_operation_batch_children_operation ON operation_batch_children(operation_id);

-- Platform extension instance has no Workspace ownership. Installation is
-- represented separately so one instance can be bound to several clusters.
CREATE TABLE IF NOT EXISTS platform_extension_instances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    provider_type TEXT NOT NULL,
    phase TEXT NOT NULL DEFAULT 'pending',
    manifest JSONB NOT NULL DEFAULT '{}',
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (name, version)
);
CREATE TABLE IF NOT EXISTS platform_extension_cluster_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id UUID NOT NULL REFERENCES platform_extension_instances(id) ON DELETE CASCADE,
    cluster_id UUID NOT NULL REFERENCES runtime_targets(id) ON DELETE RESTRICT,
    phase TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (instance_id, cluster_id)
);
CREATE TABLE IF NOT EXISTS platform_extension_tenant_entitlements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id UUID NOT NULL REFERENCES platform_extension_instances(id) ON DELETE CASCADE,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    namespace_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_platform_extension_entitlement_scope
    ON platform_extension_tenant_entitlements(instance_id, tenant_id,
        COALESCE(workspace_id::text, ''), COALESCE(namespace_id, ''));

COMMIT;
