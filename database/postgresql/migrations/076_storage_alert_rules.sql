-- 076: storage_alert_rules
-- Description: Add executable storage rules to the canonical alert models.
BEGIN;

ALTER TABLE alert_rules
    ADD COLUMN IF NOT EXISTS tenant_id TEXT,
    ADD COLUMN IF NOT EXISTS name TEXT,
    ADD COLUMN IF NOT EXISTS description TEXT,
    ADD COLUMN IF NOT EXISTS target_id UUID,
    ADD COLUMN IF NOT EXISTS resource_kind TEXT,
    ADD COLUMN IF NOT EXISTS resource_uid TEXT,
    ADD COLUMN IF NOT EXISTS resource_namespace TEXT,
    ADD COLUMN IF NOT EXISTS resource_name TEXT,
    ADD COLUMN IF NOT EXISTS provider_id TEXT,
    ADD COLUMN IF NOT EXISTS metric_kind TEXT,
    ADD COLUMN IF NOT EXISTS metric_unit TEXT,
    ADD COLUMN IF NOT EXISTS metric_source TEXT,
    ADD COLUMN IF NOT EXISTS metric_fresh_for INTERVAL,
    ADD COLUMN IF NOT EXISTS comparison_operator TEXT,
    ADD COLUMN IF NOT EXISTS threshold DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS duration INTERVAL NOT NULL DEFAULT interval '5 minutes',
    ADD COLUMN IF NOT EXISTS channel_refs JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE alert_instances
    ADD COLUMN IF NOT EXISTS target_id UUID,
    ADD COLUMN IF NOT EXISTS resource_kind TEXT,
    ADD COLUMN IF NOT EXISTS resource_uid TEXT,
    ADD COLUMN IF NOT EXISTS resource_namespace TEXT,
    ADD COLUMN IF NOT EXISTS resource_name TEXT,
    ADD COLUMN IF NOT EXISTS binding_id UUID,
    ADD COLUMN IF NOT EXISTS offering_id UUID;

ALTER TABLE notification_channels
    ADD COLUMN IF NOT EXISTS secret_reference JSONB;

CREATE INDEX IF NOT EXISTS idx_storage_alert_rules_scope
    ON alert_rules(tenant_id, target_id, resource_kind, resource_uid)
    WHERE source_type = 'storage-metric';
CREATE INDEX IF NOT EXISTS idx_storage_alert_instances_resource
    ON alert_instances(tenant_id, target_id, resource_kind, resource_uid);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'ck_storage_alert_rule_complete') THEN
        ALTER TABLE alert_rules ADD CONSTRAINT ck_storage_alert_rule_complete CHECK (
            source_type <> 'storage-metric' OR (
                tenant_scope = 'tenant' AND tenant_id IS NOT NULL AND target_id IS NOT NULL
                AND resource_kind IS NOT NULL AND resource_uid IS NOT NULL
                AND provider_id IS NOT NULL AND metric_kind IS NOT NULL
                AND metric_unit IS NOT NULL AND metric_source IS NOT NULL
                AND metric_fresh_for > interval '0 seconds'
                AND comparison_operator IN ('gt', 'gte', 'lt', 'lte')
                AND threshold IS NOT NULL AND jsonb_typeof(channel_refs) = 'array'
            )
        );
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'ck_notification_channel_secret_reference') THEN
        ALTER TABLE notification_channels ADD CONSTRAINT ck_notification_channel_secret_reference CHECK (
            secret_reference IS NULL OR (
                jsonb_typeof(secret_reference) = 'object'
                AND secret_reference ? 'provider' AND secret_reference ? 'scope'
                AND secret_reference ? 'name'
                AND NOT (secret_reference ?| ARRAY['value', 'token', 'password', 'secret'])
            )
        );
    END IF;
END $$;

COMMIT;
