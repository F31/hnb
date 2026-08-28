-- 077: storage_navigation_cutover
-- Description: Gate storage supply navigation and retain a query-preserving
-- compatibility route for the canonical Container storage consumption view.
-- Dependencies: 039_console_ui_registry

BEGIN;

ALTER TABLE console_routes ADD COLUMN IF NOT EXISTS redirect_to TEXT;

UPDATE console_routes
SET path = '/container/storage', redirect_to = NULL, updated_at = now()
WHERE route_name = 'container.instances.storage';

INSERT INTO console_routes (
    route_name, path, plugin_id, component_key, permission, capability,
    redirect_to, sort_order, enabled
)
SELECT
    'container.instances.storage.legacy', '/container/instances/storage',
    plugin_id, component_key, permission, capability, '/container/storage',
    sort_order + 1, true
FROM console_routes
WHERE route_name = 'container.instances.storage'
ON CONFLICT (route_name) DO UPDATE SET
    path = EXCLUDED.path,
    plugin_id = EXCLUDED.plugin_id,
    component_key = EXCLUDED.component_key,
    permission = EXCLUDED.permission,
    capability = EXCLUDED.capability,
    redirect_to = EXCLUDED.redirect_to,
    sort_order = EXCLUDED.sort_order,
    enabled = true,
    updated_at = now();

UPDATE console_routes
SET capability = 'storage.supply', updated_at = now()
WHERE route_name = 'resource.storage' AND capability = '';

UPDATE console_navigation_items
SET capability = 'storage.supply', updated_at = now()
WHERE item_key = 'nav.resource.storage' AND capability = '';

INSERT INTO console_navigation_versions (version_key, version_value) VALUES
    ('navigation', 'db-storage-cutover-v1')
ON CONFLICT (version_key) DO UPDATE SET version_value = EXCLUDED.version_value, updated_at = now();

COMMIT;
