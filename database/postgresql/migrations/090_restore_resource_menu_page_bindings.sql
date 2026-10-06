-- 090: restore_resource_menu_page_bindings
-- Description: Restore developed resource submenu pages that were still bound
-- to early stub components in the database-backed navigation catalog.
-- Dependencies: 089_restore_native_resource_cluster_route

BEGIN;

UPDATE console_routes
SET component_key = 'NetworkManagementPage', updated_at = now()
WHERE route_name = 'resource.network'
  AND component_key = 'Network'
  AND enabled = true;

INSERT INTO console_navigation_versions (version_key, version_value) VALUES
    ('navigation', 'restore-resource-menu-pages-v1')
ON CONFLICT (version_key) DO UPDATE SET
    version_value = EXCLUDED.version_value,
    updated_at = now();

COMMIT;
