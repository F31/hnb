-- Rollback: 002_alert_notification_core
-- Description: Revert Alert/Notification core tables

DROP TABLE IF EXISTS alert_state_audits CASCADE;
DROP TABLE IF EXISTS alert_instances CASCADE;
DROP TABLE IF EXISTS alert_rules CASCADE;