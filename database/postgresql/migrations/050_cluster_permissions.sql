-- Migration: 050_cluster_permissions
-- Description: Gate cluster navigation with the canonical cluster list permission.
-- Dependencies: 049_console_routes_clear_permissions

UPDATE console_routes
SET permission = 'cluster:list', updated_at = now()
WHERE route_name = 'resource.clusters';

UPDATE console_navigation_items
SET permission = 'cluster:list', updated_at = now()
WHERE item_key = 'nav.resource.clusters';
