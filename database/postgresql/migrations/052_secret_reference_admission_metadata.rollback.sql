BEGIN;
DROP INDEX IF EXISTS idx_secret_references_admission;
ALTER TABLE secret_references
    DROP COLUMN IF EXISTS is_active,
    DROP COLUMN IF EXISTS allowed_lifecycle_provider_id,
    DROP COLUMN IF EXISTS purpose,
    DROP COLUMN IF EXISTS scope;
COMMIT;
