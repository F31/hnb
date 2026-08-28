-- Rollback: 044_tenant_id_foreign_keys
ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS subscriptions_tenant_id_fkey;
ALTER TABLE entitlements DROP CONSTRAINT IF EXISTS entitlements_tenant_id_fkey;
ALTER TABLE artifact_profile_migrations DROP CONSTRAINT IF EXISTS artifact_profile_migrations_tenant_id_fkey;
ALTER TABLE artifact_locks DROP CONSTRAINT IF EXISTS artifact_locks_tenant_id_fkey;
ALTER TABLE artifact_tombstones DROP CONSTRAINT IF EXISTS artifact_tombstones_tenant_id_fkey;
ALTER TABLE artifact_references DROP CONSTRAINT IF EXISTS artifact_references_tenant_id_fkey;
ALTER TABLE artifact_distribution_targets DROP CONSTRAINT IF EXISTS artifact_distribution_targets_tenant_id_fkey;
ALTER TABLE artifact_storage_profiles DROP CONSTRAINT IF EXISTS artifact_storage_profiles_tenant_id_fkey;
ALTER TABLE upload_sessions DROP CONSTRAINT IF EXISTS upload_sessions_tenant_id_fkey;
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_tenant_id_fkey;
ALTER TABLE publishers DROP CONSTRAINT IF EXISTS publishers_tenant_id_fkey;
