-- 072: storage_binding_drift
-- Description: Persist ordered StorageClassBinding drift evidence.
-- Dependencies: 071_storage_provider_configuration

BEGIN;

ALTER TABLE storage_class_bindings
    ADD COLUMN IF NOT EXISTS conditions JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN IF NOT EXISTS observation_generation BIGINT,
    ADD COLUMN IF NOT EXISTS observation_revision BIGINT;

ALTER TABLE storage_class_bindings
    ADD CONSTRAINT storage_class_bindings_conditions_array
        CHECK (jsonb_typeof(conditions) = 'array'),
    ADD CONSTRAINT storage_class_bindings_observation_fence
        CHECK (
            (observation_generation IS NULL AND observation_revision IS NULL)
            OR (
                observation_generation IS NOT NULL AND observation_revision IS NOT NULL
                AND observation_generation >= 1 AND observation_revision >= 1
            )
        );

COMMIT;
