-- Rollback: 004_alert_channel_delivery

ALTER TABLE IF EXISTS outbox_events DROP COLUMN IF EXISTS alert_id;
ALTER TABLE IF EXISTS outbox_events DROP COLUMN IF EXISTS delivery_id;

DROP TABLE IF EXISTS delivery_attempts CASCADE;
DROP TABLE IF EXISTS delivery_records CASCADE;
DROP TABLE IF EXISTS notification_jobs CASCADE;
DROP TABLE IF EXISTS notification_channels CASCADE;
DROP TABLE IF EXISTS user_notification_preferences CASCADE;