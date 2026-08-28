-- Rollback: 041_release_manifest_digest_index
-- Requires releases.manifest_digest values to be unique before rollback.

DROP INDEX IF EXISTS idx_releases_manifest_digest;
CREATE UNIQUE INDEX IF NOT EXISTS idx_releases_manifest_digest ON releases(manifest_digest);
