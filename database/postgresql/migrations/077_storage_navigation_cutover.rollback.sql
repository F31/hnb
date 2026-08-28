BEGIN;

DELETE FROM console_routes WHERE route_name = 'container.instances.storage.legacy';

UPDATE console_routes
SET path = '/container/instances/storage', redirect_to = NULL, updated_at = now()
WHERE route_name = 'container.instances.storage';

UPDATE console_routes
SET capability = '', updated_at = now()
WHERE route_name = 'resource.storage' AND capability = 'storage.supply';

UPDATE console_navigation_items
SET capability = '', updated_at = now()
WHERE item_key = 'nav.resource.storage' AND capability = 'storage.supply';

INSERT INTO console_navigation_versions (version_key, version_value) VALUES
    ('navigation', 'db-storage-cutover-rollback-v1')
ON CONFLICT (version_key) DO UPDATE SET version_value = EXCLUDED.version_value, updated_at = now();

COMMIT;
