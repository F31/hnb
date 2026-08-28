-- Migration: 085_runtime_target_credential_ref
-- Description: Persist the credential secret reference chosen at create/import
-- time so kubeconfig downloads resolve the exact registered secret instead of
-- the tenant's most recent one (heuristic fallback only for legacy targets).
-- Dependencies: 010_runtime_target_engine, 082_secret_references (secret store)

BEGIN;

ALTER TABLE runtime_targets
    ADD COLUMN IF NOT EXISTS credential_ref JSONB;

COMMENT ON COLUMN runtime_targets.credential_ref IS
    'Secret reference (provider/scope/name/version) bound to this target at creation; used for exact credential resolution (e.g. kubeconfig download).';

COMMIT;
