-- Rollback removes only explicit boundary discriminators; it does not touch App Market data.
BEGIN;
ALTER TABLE storage_class_bindings DROP COLUMN IF EXISTS binding_target;
ALTER TABLE workload_storage_offerings DROP COLUMN IF EXISTS consumption_model;
COMMIT;
