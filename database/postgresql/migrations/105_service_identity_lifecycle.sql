-- 105: owning-plane service identity lifecycle and one-time enrollment state.
-- Additive: no existing identity or token table is rewritten. Short-lived
-- service credentials remain signed JWTs; only issuance/revocation metadata is
-- persisted. Enrollment secrets are stored as SHA-256 hashes and consumed
-- atomically by (tenant, target, expected subject).
BEGIN;

CREATE TABLE IF NOT EXISTS service_identities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    subject_id UUID NOT NULL REFERENCES identity_subjects(id) ON DELETE RESTRICT,
    audience TEXT NOT NULL CHECK (length(audience) BETWEEN 1 AND 256 AND audience <> '*'),
    permission JSONB NOT NULL CHECK (jsonb_typeof(permission) = 'object'),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by_subject_id UUID,
    revoked_by_subject_id UUID,
    revocation_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    UNIQUE (tenant_id, subject_id, audience),
    CHECK ((status = 'active' AND revoked_at IS NULL) OR (status = 'revoked' AND revoked_at IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_service_identities_tenant_status
    ON service_identities(tenant_id, status);

CREATE TABLE IF NOT EXISTS service_credentials (
    id UUID PRIMARY KEY,
    service_identity_id UUID NOT NULL REFERENCES service_identities(id) ON DELETE RESTRICT,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    subject_id UUID NOT NULL REFERENCES identity_subjects(id) ON DELETE RESTRICT,
    audience TEXT NOT NULL,
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    revoked_by_subject_id UUID,
    revocation_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (expires_at > issued_at),
    UNIQUE (id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_service_credentials_identity
    ON service_credentials(tenant_id, service_identity_id, expires_at);
CREATE INDEX IF NOT EXISTS idx_service_credentials_active
    ON service_credentials(tenant_id, expires_at) WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS service_enrollment_credentials (
    id UUID PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE CHECK (length(token_hash) = 64),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    target_id TEXT NOT NULL CHECK (length(target_id) BETWEEN 1 AND 256),
    expected_subject TEXT NOT NULL CHECK (length(expected_subject) BETWEEN 1 AND 256),
    issued_by_subject_id UUID,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    consumed_by_subject TEXT,
    revoked_at TIMESTAMPTZ,
    revoked_by_subject_id UUID,
    created_at TIMESTAMPTZ NOT NULL,
    CHECK (expires_at > created_at),
    CHECK (consumed_at IS NULL OR consumed_by_subject IS NOT NULL),
    CHECK (revoked_at IS NULL OR consumed_at IS NULL)
);

CREATE INDEX IF NOT EXISTS idx_service_enrollment_target
    ON service_enrollment_credentials(tenant_id, target_id, expires_at);
CREATE INDEX IF NOT EXISTS idx_service_enrollment_pending
    ON service_enrollment_credentials(tenant_id, target_id) WHERE consumed_at IS NULL AND revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS agent_sessions (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    target_id TEXT NOT NULL CHECK (length(target_id) BETWEEN 1 AND 256),
    subject TEXT NOT NULL CHECK (length(subject) BETWEEN 1 AND 256),
    generation BIGINT NOT NULL CHECK (generation > 0),
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    revoked_by_subject_id UUID,
    created_at TIMESTAMPTZ NOT NULL,
    CHECK (expires_at > issued_at),
    CHECK (revoked_at IS NULL OR revoked_at >= issued_at)
);

CREATE INDEX IF NOT EXISTS idx_agent_sessions_active
    ON agent_sessions(tenant_id, target_id, expires_at) WHERE revoked_at IS NULL;

COMMIT;
