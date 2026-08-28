-- Application rollback is non-destructive: stop new descriptor writers and keep
-- these columns so verified identity and tenant ownership are not lost.
-- The statements below are for empty development databases only.

ALTER TABLE upload_sessions DROP COLUMN IF EXISTS repository;
DROP INDEX IF EXISTS idx_artifacts_tenant_created;
DROP INDEX IF EXISTS idx_artifacts_tenant_digest;
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_lifecycle_state_check;
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_verification_status_check;
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_digest_sha256_check;
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_artifact_type_check;
ALTER TABLE artifacts DROP COLUMN IF EXISTS lifecycle_state;
ALTER TABLE artifacts DROP COLUMN IF EXISTS verification_status;
ALTER TABLE artifacts DROP COLUMN IF EXISTS repository;
ALTER TABLE artifacts DROP COLUMN IF EXISTS media_type;
ALTER TABLE artifacts DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE artifacts ALTER COLUMN package_id SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_artifacts_digest ON artifacts(digest);
ALTER TABLE artifacts ADD CONSTRAINT artifacts_artifact_type_check CHECK (artifact_type IN (
    'oci_image', 'helm_chart', 'container_image', 'terraform_module', 'generic'
));
