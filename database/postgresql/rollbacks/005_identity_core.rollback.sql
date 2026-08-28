-- Rollback: 005_identity_core
-- Reverts: tenants, projects, environments, namespaces

DROP TABLE IF EXISTS namespaces CASCADE;
DROP TABLE IF EXISTS environments CASCADE;
DROP TABLE IF EXISTS projects CASCADE;
DROP TABLE IF EXISTS tenants CASCADE;
