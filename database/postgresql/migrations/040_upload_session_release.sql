-- Migration: 040_upload_session_release
-- Description: Associate artifact upload sessions with draft releases.
-- Tiers: All
-- Dependencies: 032_app_market_upload_sessions, 034_release_artifacts

ALTER TABLE upload_sessions
    ADD COLUMN IF NOT EXISTS release_id UUID REFERENCES releases(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_upload_sessions_release_id ON upload_sessions(release_id);
