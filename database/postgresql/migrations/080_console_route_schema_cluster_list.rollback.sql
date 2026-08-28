-- Migration: 080_console_route_schema_cluster_list (rollback)
-- Description: Restore resource.clusters to plugin component rendering.

BEGIN;

UPDATE console_routes
SET schema_id = NULL, updated_at = now()
WHERE route_name = 'resource.clusters';

INSERT INTO console_navigation_versions (version_key, version_value) VALUES
    ('navigation', 'db-seed-v1')
ON CONFLICT (version_key) DO UPDATE SET
    version_value = EXCLUDED.version_value,
    updated_at = now();

COMMIT;
