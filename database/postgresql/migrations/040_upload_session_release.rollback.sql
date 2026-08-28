-- Rollback: 040_upload_session_release

DROP INDEX IF EXISTS idx_upload_sessions_release_id;
ALTER TABLE upload_sessions DROP COLUMN IF EXISTS release_id;
