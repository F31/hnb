BEGIN;

ALTER TABLE security_audit_events
    DROP COLUMN IF EXISTS delegation_key_id,
    DROP COLUMN IF EXISTS delegation_token_id,
    DROP COLUMN IF EXISTS actor_membership_id,
    DROP COLUMN IF EXISTS service_subject;

COMMIT;
