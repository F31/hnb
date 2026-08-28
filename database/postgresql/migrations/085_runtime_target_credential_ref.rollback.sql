-- Rollback: 085_runtime_target_credential_ref
-- WARNING: drops the credential binding recorded at target creation.
-- kubeconfig downloads fall back to the tenant-latest heuristic.

BEGIN;

ALTER TABLE runtime_targets DROP COLUMN IF EXISTS credential_ref;

COMMIT;
