-- 080: console_route_schema_cluster_list
-- Description: Point the resource.clusters list route at the database-backed
-- PageSchema "cluster-list" so the SchemaPage runtime (V2.6 §7) renders the
-- standard list page end-to-end. The plugin component_key is preserved as a
-- fallback; RouterManager prefers schemaId (SchemaPage) when present.
-- Dependencies: 079_ui_page_registry

BEGIN;

UPDATE console_routes
SET schema_id = 'cluster-list', updated_at = now()
WHERE route_name = 'resource.clusters' AND enabled = true;

-- Bump the navigation version so clients refresh their cached catalog and
-- pick up the schemaId route (V2.6 §15.3 版本化缓存键).
INSERT INTO console_navigation_versions (version_key, version_value) VALUES
    ('navigation', 'db-schema-cluster-list-v1')
ON CONFLICT (version_key) DO UPDATE SET
    version_value = EXCLUDED.version_value,
    updated_at = now();

COMMIT;
