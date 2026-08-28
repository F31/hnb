-- Migration: 044_tenant_id_foreign_keys
-- Description: Enforce tenant referential integrity for app-market tables so
-- tenant_id values cannot dangle when a tenant is removed.
-- Tiers: All
-- Dependencies: 011_app_market_engine, 042_artifact_storage_profile_and_updated_at

ALTER TABLE publishers DROP CONSTRAINT IF EXISTS publishers_tenant_id_fkey;
ALTER TABLE publishers
    ADD CONSTRAINT publishers_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_tenant_id_fkey;
ALTER TABLE artifacts
    ADD CONSTRAINT artifacts_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE upload_sessions DROP CONSTRAINT IF EXISTS upload_sessions_tenant_id_fkey;
ALTER TABLE upload_sessions
    ADD CONSTRAINT upload_sessions_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE artifact_storage_profiles DROP CONSTRAINT IF EXISTS artifact_storage_profiles_tenant_id_fkey;
ALTER TABLE artifact_storage_profiles
    ADD CONSTRAINT artifact_storage_profiles_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE artifact_distribution_targets DROP CONSTRAINT IF EXISTS artifact_distribution_targets_tenant_id_fkey;
ALTER TABLE artifact_distribution_targets
    ADD CONSTRAINT artifact_distribution_targets_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE artifact_references DROP CONSTRAINT IF EXISTS artifact_references_tenant_id_fkey;
ALTER TABLE artifact_references
    ADD CONSTRAINT artifact_references_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE artifact_tombstones DROP CONSTRAINT IF EXISTS artifact_tombstones_tenant_id_fkey;
ALTER TABLE artifact_tombstones
    ADD CONSTRAINT artifact_tombstones_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE artifact_locks DROP CONSTRAINT IF EXISTS artifact_locks_tenant_id_fkey;
ALTER TABLE artifact_locks
    ADD CONSTRAINT artifact_locks_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE artifact_profile_migrations DROP CONSTRAINT IF EXISTS artifact_profile_migrations_tenant_id_fkey;
ALTER TABLE artifact_profile_migrations
    ADD CONSTRAINT artifact_profile_migrations_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE entitlements DROP CONSTRAINT IF EXISTS entitlements_tenant_id_fkey;
ALTER TABLE entitlements
    ADD CONSTRAINT entitlements_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS subscriptions_tenant_id_fkey;
ALTER TABLE subscriptions
    ADD CONSTRAINT subscriptions_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
