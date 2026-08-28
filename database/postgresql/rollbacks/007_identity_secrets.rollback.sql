-- Rollback: 007_identity_secrets
-- Reverts: secret_references, secret_versions

DROP TABLE IF EXISTS secret_versions CASCADE;
DROP TABLE IF EXISTS secret_references CASCADE;
