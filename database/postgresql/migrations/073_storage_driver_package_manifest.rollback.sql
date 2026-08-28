BEGIN;

ALTER TABLE provider_manifests
    DROP COLUMN IF EXISTS storage_driver_package;

COMMIT;
