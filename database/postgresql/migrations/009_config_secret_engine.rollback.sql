-- Rollback: 009_config_secret_engine
ALTER TABLE IF EXISTS secret_references DROP COLUMN IF EXISTS kms_provider_id;
DROP TABLE IF EXISTS kms_providers CASCADE;
DROP TABLE IF EXISTS config_snapshots CASCADE;
DROP TABLE IF EXISTS config_values CASCADE;
DROP TABLE IF EXISTS config_layers CASCADE;
