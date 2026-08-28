BEGIN;

INSERT INTO tenants (id, name, display_name) VALUES
    ('storage-test-a', 'storage-test-a', 'Storage Test A'),
    ('storage-test-b', 'storage-test-b', 'Storage Test B')
ON CONFLICT (id) DO NOTHING;

INSERT INTO runtime_targets (id, tenant_id, name, target_type) VALUES
    ('70000000-0000-0000-0000-000000000001', 'storage-test-a', 'target-a', 'kubernetes'),
    ('70000000-0000-0000-0000-000000000002', 'storage-test-b', 'target-b', 'kubernetes')
ON CONFLICT (id) DO NOTHING;

INSERT INTO storage_backends (
    id, tenant_id, provider_type, provider_schema_version, backend_id, display_name,
    secret_provider, secret_scope, secret_name
) VALUES (
    '71000000-0000-0000-0000-000000000001', 'storage-test-a', 'generic-csi', '1.0.0', 'backend-a', 'Backend A',
	'platform-secret', 'tenant:storage-test-a', 'backend-a-credentials'
);

INSERT INTO workload_storage_offerings (
    id, tenant_id, backend_id, name, service_mode, access_modes,
    volume_expansion, snapshots, clones, protection_class
) VALUES (
    '72000000-0000-0000-0000-000000000001', 'storage-test-a',
    '71000000-0000-0000-0000-000000000001', 'fast', 'Block', '["ReadWriteOnce"]',
    'Supported', 'Unknown', 'Unknown', 'standard'
);

INSERT INTO storage_class_bindings (
    id, tenant_id, offering_id, offering_version, target_id,
    storage_class_name, storage_class_uid, storage_class_resource_version,
    sync_state, source, observed_at, freshness
) VALUES (
    '73000000-0000-0000-0000-000000000001', 'storage-test-a',
    '72000000-0000-0000-0000-000000000001', 1,
    '70000000-0000-0000-0000-000000000001', 'fast', 'sc-fast', '42',
    'Imported', 'runtime_target_storage_inventory', now(), 'Fresh'
);

DO $$
BEGIN
    IF (SELECT version FROM storage_backends WHERE id = '71000000-0000-0000-0000-000000000001') <> 1
       OR (SELECT version FROM workload_storage_offerings WHERE id = '72000000-0000-0000-0000-000000000001') <> 1
       OR (SELECT version FROM storage_class_bindings WHERE id = '73000000-0000-0000-0000-000000000001') <> 1
       OR (SELECT conditions FROM storage_class_bindings WHERE id = '73000000-0000-0000-0000-000000000001') <> '[]'::jsonb THEN
        RAISE EXCEPTION 'storage desired-state versions were not initialized';
    END IF;
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'storage_backends'
          AND column_name ~ '(secret|credential).*(value|data|material)|(value|data|material).*(secret|credential)'
    ) THEN
        RAISE EXCEPTION 'storage backend secret value column detected';
    END IF;

    BEGIN
        UPDATE workload_storage_offerings SET consumption_model = 'ObjectBucket'
        WHERE id = '72000000-0000-0000-0000-000000000001';
        RAISE EXCEPTION 'object bucket was accepted as a workload storage offering';
    EXCEPTION WHEN check_violation THEN
        NULL;
    END;

    BEGIN
        UPDATE storage_class_bindings SET binding_target = 'ObjectBucket'
        WHERE id = '73000000-0000-0000-0000-000000000001';
        RAISE EXCEPTION 'object bucket was accepted as a StorageClass binding';
    EXCEPTION WHEN check_violation THEN
        NULL;
    END;

    BEGIN
        INSERT INTO workload_storage_offerings (
            tenant_id, backend_id, name, service_mode, access_modes,
            volume_expansion, snapshots, clones, protection_class
        ) VALUES (
            'storage-test-b', '71000000-0000-0000-0000-000000000001', 'cross-tenant',
            'Block', '["ReadWriteOnce"]', 'Unknown', 'Unknown', 'Unknown', 'standard'
        );
        RAISE EXCEPTION 'cross-tenant backend reference was accepted';
    EXCEPTION WHEN foreign_key_violation THEN
        NULL;
    END;

    UPDATE storage_backends SET display_name = 'should-not-change', version = version + 1
    WHERE id = '71000000-0000-0000-0000-000000000001' AND tenant_id = 'storage-test-a' AND version = 99;
    IF FOUND THEN
        RAISE EXCEPTION 'stale optimistic version updated storage backend';
    END IF;

	BEGIN
		UPDATE storage_class_bindings
		SET observation_generation = 2, observation_revision = NULL
		WHERE id = '73000000-0000-0000-0000-000000000001';
		RAISE EXCEPTION 'partial storage observation fence was accepted';
	EXCEPTION WHEN check_violation THEN
		NULL;
	END;
END
$$;

ROLLBACK;
