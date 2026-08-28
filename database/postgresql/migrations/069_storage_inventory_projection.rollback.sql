-- 069 rollback intentionally retains last-known storage facts and evidence.
-- Removing the read-path indexes disables the projection query path without
-- deleting audit history or requiring any target-side Kubernetes mutation.

BEGIN;

DROP INDEX IF EXISTS idx_storage_driver_evidence_freshness;
DROP INDEX IF EXISTS idx_storage_driver_evidence_active;
DROP INDEX IF EXISTS idx_storage_inventory_freshness;
DROP INDEX IF EXISTS idx_storage_inventory_driver_active;
DROP INDEX IF EXISTS idx_storage_inventory_target_kind_active;

COMMIT;
