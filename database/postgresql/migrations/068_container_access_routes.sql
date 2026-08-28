-- 068: container_access_routes
-- Description: Register hidden create/edit routes for container access resources.
-- Dependencies: 039_console_ui_registry

BEGIN;

INSERT INTO console_routes (route_name, path, plugin_id, component_key, permission, sort_order) VALUES
    ('container.instances.access.service.create', '/container/instances/access/service/create', 'container', 'AccessForm', '', 41),
    ('container.instances.access.service.edit', '/container/instances/access/service/:name/edit', 'container', 'AccessForm', '', 42),
    ('container.instances.access.ingress.create', '/container/instances/access/ingress/create', 'container', 'AccessForm', '', 43),
    ('container.instances.access.ingress.edit', '/container/instances/access/ingress/:name/edit', 'container', 'AccessForm', '', 44),
    ('container.instances.access.network-policy.create', '/container/instances/access/network-policy/create', 'container', 'AccessForm', '', 45),
    ('container.instances.access.network-policy.edit', '/container/instances/access/network-policy/:name/edit', 'container', 'AccessForm', '', 46)
ON CONFLICT (route_name) DO UPDATE SET
    path = EXCLUDED.path,
    plugin_id = EXCLUDED.plugin_id,
    component_key = EXCLUDED.component_key,
    permission = EXCLUDED.permission,
    sort_order = EXCLUDED.sort_order,
    enabled = true,
    updated_at = now();

INSERT INTO console_navigation_versions (version_key, version_value) VALUES
    ('navigation', 'db-container-access-v1')
ON CONFLICT (version_key) DO UPDATE SET version_value = EXCLUDED.version_value, updated_at = now();

COMMIT;
