-- 059: platform_settings
-- Global platform settings stored as key-value pairs with JSONB values.

CREATE TABLE IF NOT EXISTS platform_settings (
    key         TEXT PRIMARY KEY,
    value       JSONB NOT NULL DEFAULT '{}',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE platform_settings IS 'Global platform configuration key-value store';
COMMENT ON COLUMN platform_settings.key IS 'Setting key, e.g. platform.name, notification.email, security.session_timeout';
COMMENT ON COLUMN platform_settings.value IS 'Setting value as arbitrary JSON';