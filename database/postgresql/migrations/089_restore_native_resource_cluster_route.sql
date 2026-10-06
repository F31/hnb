-- 089: restore_native_resource_cluster_route
-- Description: Restore the resource.clusters route to the developed native
-- ClusterList component. The cluster-list schema is only a runtime fixture and
-- RouterManager gives schema_id precedence over component_key, which bypasses
-- the full resource cluster management page.
-- Dependencies: 088_fix_schema_revision_uniqueness

BEGIN;

UPDATE console_routes
SET schema_id = NULL, updated_at = now()
WHERE route_name = 'resource.clusters'
  AND enabled = true;

-- Bump the navigation version so clients refresh cached route metadata and stop
-- routing /resource/clusters through SchemaPage.
INSERT INTO console_navigation_versions (version_key, version_value) VALUES
    ('navigation', 'native-resource-cluster-list-v1')
ON CONFLICT (version_key) DO UPDATE SET
    version_value = EXCLUDED.version_value,
    updated_at = now();

COMMIT;
