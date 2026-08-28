-- 070 rollback removes only desired-state records introduced by this migration.
-- Observed inventory and target-side Kubernetes resources are unaffected.

BEGIN;

DROP TABLE IF EXISTS storage_class_bindings;
DROP TABLE IF EXISTS workload_storage_offerings;
DROP TABLE IF EXISTS storage_backends;

COMMIT;
