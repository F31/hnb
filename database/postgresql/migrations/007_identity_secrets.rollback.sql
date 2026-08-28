-- Rollback: 007_identity_secrets

DROP TABLE IF EXISTS secret_versions CASCADE;
DROP TABLE IF EXISTS secret_references CASCADE;