-- Migration: 033_artifact_descriptors
-- Description: Extend artifacts into tenant-scoped, verified ArtifactDescriptors.
-- Rollout: additive columns first; existing rows remain pending until reconciled.

ALTER TABLE artifacts
    ADD COLUMN IF NOT EXISTS tenant_id TEXT,
    ADD COLUMN IF NOT EXISTS media_type TEXT,
    ADD COLUMN IF NOT EXISTS repository TEXT,
    ADD COLUMN IF NOT EXISTS verification_status TEXT NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS lifecycle_state TEXT NOT NULL DEFAULT 'active';

UPDATE artifacts a
SET tenant_id = pub.tenant_id
FROM packages pkg
JOIN products p ON p.id = pkg.product_id
JOIN publishers pub ON pub.id = p.publisher_id
WHERE a.package_id = pkg.id AND a.tenant_id IS NULL;

ALTER TABLE artifacts ALTER COLUMN package_id DROP NOT NULL;
ALTER TABLE artifacts ALTER COLUMN tenant_id SET NOT NULL;

ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_artifact_type_check;
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_digest_sha256_check;
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_verification_status_check;
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_lifecycle_state_check;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_artifact_type_check CHECK (artifact_type IN (
    'oci_image', 'helm_chart', 'container_image', 'terraform_module', 'jar', 'war',
    'operator', 'configuration', 'model', 'prompt', 'guardrail', 'evaluation',
    'sbom', 'offline_bundle', 'generic'
));
ALTER TABLE artifacts ADD CONSTRAINT artifacts_digest_sha256_check
    CHECK (digest ~ '^sha256:[0-9a-f]{64}$') NOT VALID;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_verification_status_check
    CHECK (verification_status IN ('pending', 'verified', 'failed'));
ALTER TABLE artifacts ADD CONSTRAINT artifacts_lifecycle_state_check
    CHECK (lifecycle_state IN ('active', 'tombstoned', 'deleting', 'deleted'));

DROP INDEX IF EXISTS idx_artifacts_digest;
CREATE UNIQUE INDEX IF NOT EXISTS idx_artifacts_tenant_digest
    ON artifacts(tenant_id, digest);
CREATE INDEX IF NOT EXISTS idx_artifacts_tenant_created
    ON artifacts(tenant_id, created_at DESC);

ALTER TABLE upload_sessions ADD COLUMN IF NOT EXISTS repository TEXT;
UPDATE upload_sessions
SET repository = CASE
    WHEN lower(filename) LIKE '%.jar' THEN 'hnb/jars'
    WHEN lower(filename) LIKE '%.war' THEN 'hnb/wars'
    WHEN lower(filename) LIKE '%.tar.gz' OR lower(filename) LIKE '%.tgz' THEN 'hnb/charts'
    WHEN lower(filename) LIKE '%.zip' THEN 'hnb/zips'
    ELSE 'hnb/generic'
END
WHERE repository IS NULL;
ALTER TABLE upload_sessions ALTER COLUMN repository SET NOT NULL;
