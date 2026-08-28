-- Migration: 027_trusted_identity_tokens
-- Description: Persist legacy-user identity bridges and hashed refresh credentials
-- Dependencies: 006_identity_rbac, 024_scoped_identity

BEGIN;

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    email TEXT,
    display_name TEXT,
    password_hash TEXT NOT NULL,
    source TEXT NOT NULL,
    source_id TEXT,
    is_active BOOLEAN NOT NULL,
    labels JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Existing deployments use this explicit issuer for the legacy local-user bridge.
-- IAMDBStore.Migrate repeats this operation with the configured production issuer.
INSERT INTO identity_subjects (
    issuer, external_subject, subject_type, display_name, is_disabled, disabled_at
)
SELECT
    'https://iam.hnb.local', u.id, 'user',
    COALESCE(NULLIF(u.display_name, ''), u.username), NOT u.is_active,
    CASE WHEN u.is_active THEN NULL ELSE now() END
FROM users u
ON CONFLICT (issuer, external_subject) DO NOTHING;

INSERT INTO tenant_memberships (tenant_id, subject_id, status, is_default)
SELECT ur.tenant_id, s.id, 'active',
       COUNT(*) OVER (PARTITION BY ur.user_id) = 1
FROM (
    SELECT DISTINCT user_id, tenant_id
    FROM user_roles
    WHERE revoked_at IS NULL
) ur
JOIN identity_subjects s
  ON s.issuer = 'https://iam.hnb.local' AND s.external_subject = ur.user_id
ON CONFLICT (tenant_id, subject_id) DO NOTHING;

CREATE TABLE IF NOT EXISTS auth_refresh_tokens (
    token_hash TEXT PRIMARY KEY CHECK (length(token_hash) = 64),
    purpose TEXT NOT NULL CHECK (purpose = 'refresh'),
    subject_id UUID NOT NULL REFERENCES identity_subjects(id) ON DELETE RESTRICT,
    membership_id UUID NOT NULL REFERENCES tenant_memberships(id) ON DELETE RESTRICT,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_auth_refresh_tokens_active
    ON auth_refresh_tokens(subject_id, membership_id, expires_at)
    WHERE consumed_at IS NULL;

COMMIT;
