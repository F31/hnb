-- 030_provider_conformance_core.rollback.sql
BEGIN;

DROP TABLE IF EXISTS provider_compatibility_matrix;
DROP TABLE IF EXISTS provider_manifests;

COMMIT;