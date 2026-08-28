-- Migration: 004_alert_channel_delivery
-- Description: Create NotificationChannel, NotificationJob, DeliveryRecord, DeliveryAttempt, UserPreferences, and Outbox extension tables
-- Tiers: All
-- Dependencies: 002_alert_notification_core, 003_alert_policy_contact

-- 1. NotificationChannels
CREATE TABLE IF NOT EXISTS notification_channels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('portal', 'email', 'webhook', 'sms')),
    capability JSONB NOT NULL DEFAULT '["accepted"]',
    config_ref TEXT NOT NULL,
    secret_ref TEXT NOT NULL,
    conformance_ref TEXT,
    enabled BOOLEAN NOT NULL DEFAULT true,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notification_channels_tenant_id ON notification_channels(tenant_id);
CREATE INDEX IF NOT EXISTS idx_notification_channels_type ON notification_channels(type);
CREATE INDEX IF NOT EXISTS idx_notification_channels_enabled ON notification_channels(enabled);

-- 2. NotificationJobs
CREATE TABLE IF NOT EXISTS notification_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_id UUID NOT NULL REFERENCES alert_instances(id) ON DELETE CASCADE,
    policy_snapshot JSONB NOT NULL,
    channel_id UUID NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 50 CHECK (priority >= 0 AND priority <= 100),
    scheduled_at TIMESTAMPTZ NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('pending', 'sending', 'accepted', 'delivered', 'read', 'failed', 'suppressed', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_notification_jobs_idempotency ON notification_jobs(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_notification_jobs_alert_id ON notification_jobs(alert_id);
CREATE INDEX IF NOT EXISTS idx_notification_jobs_channel_id ON notification_jobs(channel_id);
CREATE INDEX IF NOT EXISTS idx_notification_jobs_state ON notification_jobs(state);
CREATE INDEX IF NOT EXISTS idx_notification_jobs_priority ON notification_jobs(priority);
CREATE INDEX IF NOT EXISTS idx_notification_jobs_scheduled_at ON notification_jobs(scheduled_at);

-- 3. DeliveryRecords
CREATE TABLE IF NOT EXISTS delivery_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES notification_jobs(id) ON DELETE CASCADE,
    channel_type TEXT NOT NULL CHECK (channel_type IN ('portal', 'email', 'webhook', 'sms')),
    destination_masked TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('pending', 'sending', 'accepted', 'delivered', 'read', 'failed', 'suppressed', 'cancelled')),
    provider_message_id TEXT,
    accepted_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    read_at TIMESTAMPTZ,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_error_class TEXT,
    next_attempt_at TIMESTAMPTZ,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_delivery_records_job_id ON delivery_records(job_id);
CREATE INDEX IF NOT EXISTS idx_delivery_records_state ON delivery_records(state);
CREATE INDEX IF NOT EXISTS idx_delivery_records_channel_type ON delivery_records(channel_type);
CREATE INDEX IF NOT EXISTS idx_delivery_records_next_attempt ON delivery_records(next_attempt_at);

-- 4. DeliveryAttempts
CREATE TABLE IF NOT EXISTS delivery_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_id UUID NOT NULL REFERENCES delivery_records(id) ON DELETE CASCADE,
    attempted_at TIMESTAMPTZ NOT NULL,
    result_class TEXT NOT NULL CHECK (result_class IN ('success', 'transient_failure', 'permanent_failure', 'timeout', 'circuit_open', 'rate_limited')),
    response_code INTEGER CHECK (response_code >= 100 AND response_code <= 599),
    duration TEXT,
    trace_id TEXT
);

CREATE INDEX IF NOT EXISTS idx_delivery_attempts_delivery_id ON delivery_attempts(delivery_id);
CREATE INDEX IF NOT EXISTS idx_delivery_attempts_result_class ON delivery_attempts(result_class);

-- 5. UserNotificationPreferences
CREATE TABLE IF NOT EXISTS user_notification_preferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    language TEXT NOT NULL DEFAULT 'en',
    timezone TEXT NOT NULL DEFAULT 'UTC',
    channels JSONB NOT NULL DEFAULT '["portal"]',
    severity_filters JSONB DEFAULT '["critical", "warning", "info"]',
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_preferences_tenant_user ON user_notification_preferences(tenant_id, user_id);

-- 6. Extend outbox_events with alert notification context
ALTER TABLE IF EXISTS outbox_events
  ADD COLUMN IF NOT EXISTS alert_id UUID,
  ADD COLUMN IF NOT EXISTS delivery_id UUID;

CREATE INDEX IF NOT EXISTS idx_outbox_events_alert_id ON outbox_events(alert_id);
CREATE INDEX IF NOT EXISTS idx_outbox_events_delivery_id ON outbox_events(delivery_id);