BEGIN;

INSERT INTO tenants (id, name, display_name) VALUES
    ('storage-alert-a', 'storage-alert-a', 'Storage Alert A'),
    ('storage-alert-b', 'storage-alert-b', 'Storage Alert B')
ON CONFLICT (id) DO NOTHING;
INSERT INTO runtime_targets (id, tenant_id, name, target_type) VALUES
    ('76000000-0000-0000-0000-000000000001', 'storage-alert-a', 'target-a', 'kubernetes')
ON CONFLICT (id) DO NOTHING;

INSERT INTO alert_rules (
    id, tenant_scope, tenant_id, name, source_type, severity, target_id,
    resource_kind, resource_uid, resource_namespace, resource_name, provider_id,
    metric_kind, metric_unit, metric_source, metric_fresh_for,
    comparison_operator, threshold, channel_refs
) VALUES (
    '76100000-0000-0000-0000-000000000001', 'tenant', 'storage-alert-a', 'PVC Pending',
    'storage-metric', 'warning', '76000000-0000-0000-0000-000000000001',
    'PersistentVolumeClaim', 'pvc-uid-a', 'payments', 'ledger-data', 'kubernetes',
    'health', '1', 'kube_state_metrics', interval '5 minutes', 'lt', 1,
    '[{"type":"webhook","configReference":"channel-a","secretReference":{"provider":"platform-secrets","scope":"tenant:storage-alert-a","name":"hook-a"}}]'
);

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name IN ('alert_events', 'alert_notifications')) THEN
        RAISE EXCEPTION 'private alert tables still exist';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM alert_rules WHERE tenant_id='storage-alert-a' AND target_id='76000000-0000-0000-0000-000000000001'
          AND resource_kind='PersistentVolumeClaim' AND resource_uid='pvc-uid-a'
          AND resource_namespace='payments' AND resource_name='ledger-data'
    ) THEN RAISE EXCEPTION 'stable PVC navigation identity was not stored'; END IF;
    IF EXISTS (SELECT 1 FROM alert_rules WHERE tenant_id='storage-alert-b' AND id='76100000-0000-0000-0000-000000000001') THEN
        RAISE EXCEPTION 'cross-tenant alert rule visible';
    END IF;
    IF (SELECT channel_refs::text FROM alert_rules WHERE id='76100000-0000-0000-0000-000000000001') ~* '(password|token|secretValue)' THEN
        RAISE EXCEPTION 'channel secret leaked';
    END IF;
END $$;

ROLLBACK;
