-- Migration: 026_signing_key_metadata
-- Description: Store signing-key identifiers and lifecycle without private key material
-- Tiers: All
-- Dependencies: 024_scoped_identity, 025_runtime_intent_audit

BEGIN;

CREATE TABLE IF NOT EXISTS signing_key_metadata (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT REFERENCES tenants(id) ON DELETE RESTRICT,
    issuer TEXT NOT NULL,
    key_id TEXT NOT NULL,
    algorithm TEXT NOT NULL,
    signing_provider TEXT NOT NULL,
    signing_key_handle TEXT NOT NULL,
    verification_key_ref TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'retiring', 'revoked', 'expired')),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    not_before TIMESTAMPTZ NOT NULL,
    not_after TIMESTAMPTZ NOT NULL,
    activated_at TIMESTAMPTZ,
    retiring_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    revocation_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (issuer, key_id),
    UNIQUE (id, tenant_id),
    CHECK (not_after > not_before),
    CHECK (status <> 'revoked' OR revoked_at IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_signing_key_metadata_status
    ON signing_key_metadata(issuer, status, not_after);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_security_audit_signing_key') THEN
        ALTER TABLE security_audit_events ADD CONSTRAINT fk_security_audit_signing_key
            FOREIGN KEY (key_id) REFERENCES signing_key_metadata(id) ON DELETE RESTRICT;
    END IF;
END
$$;

CREATE TABLE IF NOT EXISTS signing_key_lifecycle_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    signing_key_id UUID NOT NULL REFERENCES signing_key_metadata(id) ON DELETE RESTRICT,
    event_type TEXT NOT NULL CHECK (event_type IN ('created', 'activated', 'rotation_started', 'retired', 'revoked', 'expired')),
    actor_subject_id UUID REFERENCES identity_subjects(id) ON DELETE RESTRICT,
    correlation_id UUID NOT NULL,
    reason TEXT,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_signing_key_lifecycle_key_time
    ON signing_key_lifecycle_events(signing_key_id, occurred_at DESC);

DROP TRIGGER IF EXISTS signing_key_metadata_no_delete ON signing_key_metadata;
CREATE TRIGGER signing_key_metadata_no_delete
    BEFORE DELETE ON signing_key_metadata
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_row_change();

DROP TRIGGER IF EXISTS signing_key_lifecycle_immutable ON signing_key_lifecycle_events;
CREATE TRIGGER signing_key_lifecycle_immutable
    BEFORE UPDATE OR DELETE ON signing_key_lifecycle_events
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_row_change();

COMMIT;
