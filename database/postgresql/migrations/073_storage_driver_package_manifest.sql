BEGIN;

ALTER TABLE provider_manifests
    ADD COLUMN IF NOT EXISTS storage_driver_package JSONB NOT NULL DEFAULT 'null'::jsonb;

COMMIT;
