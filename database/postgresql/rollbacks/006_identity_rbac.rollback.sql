-- Rollback: 006_identity_rbac
-- Reverts: roles, user_roles, approval_policies

DROP TABLE IF EXISTS user_roles CASCADE;
DROP TABLE IF EXISTS approval_policies CASCADE;
DROP TABLE IF EXISTS roles CASCADE;
