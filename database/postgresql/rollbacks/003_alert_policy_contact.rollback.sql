-- Rollback: 003_alert_policy_contact

DROP TABLE IF EXISTS schedules CASCADE;
DROP TABLE IF EXISTS contact_groups CASCADE;
DROP TABLE IF EXISTS notification_policies CASCADE;
DROP TABLE IF EXISTS maintenance_windows CASCADE;
DROP TABLE IF EXISTS silences CASCADE;