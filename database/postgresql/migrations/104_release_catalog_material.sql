-- 104: owning-plane release material catalog. The same content-addressed
-- offline material closure, tied to one installer release lock, is registered
-- under each install target role so the console and CLI present one directory.
BEGIN;

CREATE TABLE IF NOT EXISTS release_catalog_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    release_id TEXT NOT NULL,
    release_version TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('console', 'control-plane', 'worker')),
    bundle_id TEXT NOT NULL,
    lock_digest TEXT NOT NULL CHECK (lock_digest ~ '^sha256:[0-9a-f]{64}$'),
    content_digest TEXT NOT NULL CHECK (content_digest ~ '^sha256:[0-9a-f]{64}$'),
    imported_by TEXT NOT NULL,
    registered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (release_id, release_version, role)
);
CREATE INDEX IF NOT EXISTS idx_release_catalog_release ON release_catalog_entries(release_id, release_version);

COMMIT;