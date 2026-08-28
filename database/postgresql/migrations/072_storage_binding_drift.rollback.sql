BEGIN;

ALTER TABLE storage_class_bindings
    DROP CONSTRAINT IF EXISTS storage_class_bindings_observation_fence,
    DROP CONSTRAINT IF EXISTS storage_class_bindings_conditions_array,
    DROP COLUMN IF EXISTS observation_revision,
    DROP COLUMN IF EXISTS observation_generation,
    DROP COLUMN IF EXISTS conditions;

COMMIT;
