-- 071: storage_provider_configuration
-- Description: Persist only versioned, server-validated provider extension attributes.
-- Dependencies: 070_storage_desired_state

BEGIN;

ALTER TABLE storage_backends
    ADD COLUMN IF NOT EXISTS provider_schema_version TEXT,
    ADD COLUMN IF NOT EXISTS attributes JSONB NOT NULL DEFAULT '{}';

ALTER TABLE storage_backends
    ADD CONSTRAINT storage_backends_attributes_object
        CHECK (jsonb_typeof(attributes) = 'object');

COMMIT;
