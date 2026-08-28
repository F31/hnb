-- Rollback: 035_artifact_storage_profiles
-- Development-only after proving no production profile or migration metadata is needed.

DROP TABLE IF EXISTS artifact_profile_migrations;
ALTER TABLE artifacts DROP COLUMN IF EXISTS storage_profile_id;
DROP TABLE IF EXISTS artifact_storage_profiles;
