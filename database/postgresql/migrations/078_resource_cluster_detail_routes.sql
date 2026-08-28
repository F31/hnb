-- 078: resource_cluster_detail_routes
-- Description: Register backend-owned routes for Resource cluster and node
-- detail pages declared by the resource plugin.
-- Dependencies: 039_console_ui_registry

BEGIN;

INSERT INTO console_routes (
    route_name, path, plugin_id, component_key, permission, sort_order, enabled
) VALUES
    ('resource.clusters.detail', '/resource/clusters/:clusterId', 'resource', 'ClusterDetailRedirect', 'cluster:read', 11, true),
    ('resource.clusters.detail.overview', '/resource/clusters/:clusterId/overview', 'resource', 'ClusterOverviewPage', 'cluster:read', 12, true),
    ('resource.clusters.detail.monitoring', '/resource/clusters/:clusterId/monitoring', 'resource', 'ClusterMonitoringPage', 'cluster:read', 13, true),
    ('resource.clusters.detail.nodes', '/resource/clusters/:clusterId/nodes', 'resource', 'NodeListPage', 'cluster:read', 14, true),
    ('resource.clusters.detail.nodes.redirect', '/resource/clusters/:clusterId/nodes/:nodeId', 'resource', 'NodeDetailRedirect', 'cluster:read', 15, true),
    ('resource.clusters.detail.nodes.basic', '/resource/clusters/:clusterId/nodes/:nodeId/basic', 'resource', 'NodeDetailPage', 'cluster:read', 16, true),
    ('resource.clusters.detail.nodes.monitoring', '/resource/clusters/:clusterId/nodes/:nodeId/monitoring', 'resource', 'NodeDetailPage', 'cluster:read', 17, true),
    ('resource.clusters.detail.nodes.disks', '/resource/clusters/:clusterId/nodes/:nodeId/disks', 'resource', 'NodeDetailPage', 'cluster:read', 18, true),
    ('resource.clusters.detail.nodes.nics', '/resource/clusters/:clusterId/nodes/:nodeId/nics', 'resource', 'NodeDetailPage', 'cluster:read', 19, true),
    ('resource.clusters.detail.nodes.pods', '/resource/clusters/:clusterId/nodes/:nodeId/pods', 'resource', 'NodeDetailPage', 'cluster:read', 20, true),
    ('resource.clusters.detail.nodes.virtual-machines', '/resource/clusters/:clusterId/nodes/:nodeId/virtual-machines', 'resource', 'NodeDetailPage', 'cluster:read', 21, true),
    ('resource.clusters.detail.edge-node-groups', '/resource/clusters/:clusterId/edge-node-groups', 'resource', 'EdgeNodeGroupsPage', 'cluster:read', 22, true),
    ('resource.clusters.detail.tenant-allocations', '/resource/clusters/:clusterId/tenant-allocations', 'resource', 'TenantAllocationPage', 'cluster:read', 23, true),
    ('resource.clusters.detail.plugin-instances', '/resource/clusters/:clusterId/plugin-instances', 'resource', 'PluginInstancesPage', 'cluster:read', 24, true)
ON CONFLICT (route_name) DO UPDATE SET
    path = EXCLUDED.path,
    plugin_id = EXCLUDED.plugin_id,
    component_key = EXCLUDED.component_key,
    schema_id = NULL,
    permission = EXCLUDED.permission,
    enabled = true,
    sort_order = EXCLUDED.sort_order,
    updated_at = now();

INSERT INTO console_navigation_versions (version_key, version_value) VALUES
    ('navigation', 'db-resource-cluster-detail-v1')
ON CONFLICT (version_key) DO UPDATE SET
    version_value = EXCLUDED.version_value,
    updated_at = now();

COMMIT;
