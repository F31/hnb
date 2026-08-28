-- Rollback: 042_artifact_storage_profile_and_updated_at
ALTER TABLE artifacts DROP COLUMN IF EXISTS updated_at;
