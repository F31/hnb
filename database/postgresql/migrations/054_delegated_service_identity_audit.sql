-- Migration: 054_delegated_service_identity_audit
-- Description: Preserve service caller and signed actor delegation evidence
-- Dependencies: 025_runtime_intent_audit, 053_runtime_intent_commitment

BEGIN;

ALTER TABLE security_audit_events
    ADD COLUMN IF NOT EXISTS service_subject TEXT,
    ADD COLUMN IF NOT EXISTS actor_membership_id TEXT,
    ADD COLUMN IF NOT EXISTS delegation_token_id TEXT,
    ADD COLUMN IF NOT EXISTS delegation_key_id TEXT;

COMMIT;
