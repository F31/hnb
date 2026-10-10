-- 101: secret_reference_admission_unique
-- Give secrets_bridge.StoreKubeconfig its ON CONFLICT target. The provider
-- inserts with (tenant_id, name, scope, purpose, allowed_lifecycle_provider_id)
-- and previously relied on an index that does not back ON CONFLICT (the 052
-- unique index is partial WHERE is_active). Without a full unique constraint
-- the INSERT fails at runtime with "ON CONFLICT ... does not match any unique
-- or exclusion constraint".
BEGIN;

-- Backfill any legacy rows whose column set collides (e.g. a duplicate name
-- previously stored without purpose/scope) so the constraint can be created.
UPDATE secret_references
SET scope = COALESCE(scope, 'tenant:' || tenant_id),
    purpose = COALESCE(purpose, 'legacy-unknown')
WHERE scope IS NULL OR purpose IS NULL;

-- Keep only the most recent row when a duplicate would violate the constraint;
-- this is a de-dup guard, not a data-loss path (each row still has its own id).
WITH ranked AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY tenant_id, name, scope, purpose, allowed_lifecycle_provider_id
               ORDER BY updated_at DESC, created_at DESC
           ) AS rn
    FROM secret_references
)
DELETE FROM secret_references
WHERE id IN (SELECT id FROM ranked WHERE rn > 1);

CREATE UNIQUE INDEX IF NOT EXISTS uq_secret_references_admission
    ON secret_references(tenant_id, name, scope, purpose, allowed_lifecycle_provider_id);

COMMIT;