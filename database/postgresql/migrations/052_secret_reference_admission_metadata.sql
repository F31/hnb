BEGIN;

ALTER TABLE secret_references
    ADD COLUMN IF NOT EXISTS scope TEXT,
    ADD COLUMN IF NOT EXISTS purpose TEXT,
    ADD COLUMN IF NOT EXISTS allowed_lifecycle_provider_id TEXT,
    ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true;

UPDATE secret_references
SET scope = COALESCE(scope, 'tenant:' || tenant_id),
    purpose = COALESCE(purpose, 'legacy-unknown');

ALTER TABLE secret_references
    ALTER COLUMN scope SET NOT NULL,
    ALTER COLUMN purpose SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_secret_references_admission
    ON secret_references(tenant_id, name, scope, purpose, allowed_lifecycle_provider_id)
    WHERE is_active;

COMMIT;
