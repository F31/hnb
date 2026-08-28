-- Rollback: 084_runtime_target_description
ALTER TABLE runtime_targets DROP COLUMN IF EXISTS description;
