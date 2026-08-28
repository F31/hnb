-- Migration: 023_app_market_applications
-- Description: Add runtime application instances without cascading away their history
-- Tiers: All
-- Dependencies: 008_operation_engine_core, 011_app_market_engine, 021_workspace_hierarchy

CREATE TABLE IF NOT EXISTS applications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    workspace_id UUID REFERENCES workspaces(id) ON DELETE RESTRICT,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    release_id UUID NOT NULL REFERENCES releases(id) ON DELETE RESTRICT,
    execution_plan_id UUID REFERENCES execution_plans(id) ON DELETE SET NULL,
    operation_id UUID REFERENCES operations(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    namespace TEXT,
    status TEXT NOT NULL DEFAULT 'deploying' CHECK (status IN ('deploying', 'ready', 'degraded', 'uninstalling')),
    config JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_applications_tenant ON applications(tenant_id);
CREATE INDEX IF NOT EXISTS idx_applications_workspace ON applications(workspace_id);
CREATE INDEX IF NOT EXISTS idx_applications_product ON applications(product_id);
CREATE INDEX IF NOT EXISTS idx_applications_release ON applications(release_id);
CREATE INDEX IF NOT EXISTS idx_applications_execution_plan ON applications(execution_plan_id);
CREATE INDEX IF NOT EXISTS idx_applications_operation ON applications(operation_id);
CREATE INDEX IF NOT EXISTS idx_applications_status ON applications(status);
