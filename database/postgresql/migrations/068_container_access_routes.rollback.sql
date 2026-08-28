BEGIN;

DELETE FROM console_routes WHERE route_name IN (
    'container.instances.access.service.create',
    'container.instances.access.service.edit',
    'container.instances.access.ingress.create',
    'container.instances.access.ingress.edit',
    'container.instances.access.network-policy.create',
    'container.instances.access.network-policy.edit'
);

INSERT INTO console_navigation_versions (version_key, version_value) VALUES
    ('navigation', 'db-container-access-rollback-v1')
ON CONFLICT (version_key) DO UPDATE SET version_value = EXCLUDED.version_value, updated_at = now();

COMMIT;
