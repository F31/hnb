-- 028_plan_digest_tenant_unique
-- Move plan_digest uniqueness from global to tenant-scoped.
-- This enables the same plan_digest across different tenants (safe after
-- P0-B SemanticDigest includes tenant_id in the digest computation).

DROP INDEX IF EXISTS idx_execution_plans_digest;

CREATE UNIQUE INDEX IF NOT EXISTS idx_execution_plans_tenant_digest
    ON execution_plans(tenant_id, plan_digest);