-- Rollback: 050_cluster_permissions

UPDATE console_routes
SET permission = '', updated_at = now()
WHERE route_name = 'resource.clusters';

UPDATE console_navigation_items
SET permission = '', updated_at = now()
WHERE item_key = 'nav.resource.clusters';
