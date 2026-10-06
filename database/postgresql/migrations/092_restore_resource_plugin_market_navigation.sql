-- 092: restore_resource_plugin_market_navigation
-- Description: Publish Resource > Plugin Market as a first-level resource menu.
-- The resource plugin already implements PluginMarketPage and the backend
-- plugin-catalog APIs; this migration binds them into DB-driven navigation.
-- Dependencies: 091_seed_default_gslb_demo_service

BEGIN;

INSERT INTO console_routes (
    route_name, path, plugin_id, component_key, permission, sort_order, enabled
) VALUES (
    'resource.plugin-market', '/resource/plugin-market', 'resource', 'PluginMarketPage', 'cluster:read', 70, true
)
ON CONFLICT (route_name) DO UPDATE SET
    path = EXCLUDED.path,
    plugin_id = EXCLUDED.plugin_id,
    component_key = EXCLUDED.component_key,
    schema_id = NULL,
    permission = EXCLUDED.permission,
    enabled = true,
    sort_order = EXCLUDED.sort_order,
    updated_at = now();

INSERT INTO console_navigation_items (
    item_key, parent_key, title, icon, route_name, locale, level, sort_order, enabled
) VALUES
    ('nav.resource.plugin-market', 'nav.resource', '插件市场', 'plugin', 'resource.plugin-market', 'zh-CN', 2, 70, true),
    ('nav.resource.plugin-market', 'nav.resource', 'Plugin Market', 'plugin', 'resource.plugin-market', 'en-US', 2, 70, true)
ON CONFLICT (item_key, locale) DO UPDATE SET
    parent_key = EXCLUDED.parent_key,
    title = EXCLUDED.title,
    icon = EXCLUDED.icon,
    route_name = EXCLUDED.route_name,
    level = EXCLUDED.level,
    sort_order = EXCLUDED.sort_order,
    enabled = true,
    updated_at = now();

INSERT INTO console_navigation_versions (version_key, version_value) VALUES
    ('navigation', 'restore-resource-plugin-market-v1')
ON CONFLICT (version_key) DO UPDATE SET
    version_value = EXCLUDED.version_value,
    updated_at = now();

COMMIT;
