BEGIN;
DROP INDEX IF EXISTS idx_storage_alert_instances_resource;
DROP INDEX IF EXISTS idx_storage_alert_rules_scope;
ALTER TABLE notification_channels DROP CONSTRAINT IF EXISTS ck_notification_channel_secret_reference;
ALTER TABLE notification_channels DROP COLUMN IF EXISTS secret_reference;
ALTER TABLE alert_instances
    DROP COLUMN IF EXISTS offering_id, DROP COLUMN IF EXISTS binding_id,
    DROP COLUMN IF EXISTS resource_name, DROP COLUMN IF EXISTS resource_namespace,
    DROP COLUMN IF EXISTS resource_uid, DROP COLUMN IF EXISTS resource_kind,
    DROP COLUMN IF EXISTS target_id;
ALTER TABLE alert_rules DROP CONSTRAINT IF EXISTS ck_storage_alert_rule_complete;
ALTER TABLE alert_rules
    DROP COLUMN IF EXISTS channel_refs, DROP COLUMN IF EXISTS duration,
    DROP COLUMN IF EXISTS threshold, DROP COLUMN IF EXISTS comparison_operator,
    DROP COLUMN IF EXISTS metric_fresh_for, DROP COLUMN IF EXISTS metric_source,
    DROP COLUMN IF EXISTS metric_unit, DROP COLUMN IF EXISTS metric_kind,
    DROP COLUMN IF EXISTS provider_id, DROP COLUMN IF EXISTS resource_name,
    DROP COLUMN IF EXISTS resource_namespace, DROP COLUMN IF EXISTS resource_uid,
    DROP COLUMN IF EXISTS resource_kind, DROP COLUMN IF EXISTS target_id,
    DROP COLUMN IF EXISTS description, DROP COLUMN IF EXISTS name,
    DROP COLUMN IF EXISTS tenant_id;
COMMIT;
