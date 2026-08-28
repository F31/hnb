-- Rollback: 022_extension_framework
-- Retain provider_registry and extensions. provider_registry is owned by 010,
-- and deleting either table can remove provider lifecycle evidence.
SELECT 1;
