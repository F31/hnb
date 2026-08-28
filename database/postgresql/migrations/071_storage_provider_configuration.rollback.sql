BEGIN;

ALTER TABLE storage_backends
    DROP CONSTRAINT IF EXISTS storage_backends_attributes_object,
    DROP COLUMN IF EXISTS attributes,
    DROP COLUMN IF EXISTS provider_schema_version;

COMMIT;
