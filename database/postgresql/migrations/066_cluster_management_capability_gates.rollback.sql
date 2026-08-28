-- 066: cluster_management_capability_gates (rollback)
-- Description: Revert staged cluster capability gates attached to console
--   routes and navigation items.
-- Dependencies: 065_shared_clusters

BEGIN;

UPDATE console_routes
SET capability = '', updated_at = now()
WHERE route_name IN ('resource.clusters', 'resource.operations')
  AND capability = 'cluster.read';

UPDATE console_navigation_items
SET capability = '', updated_at = now()
WHERE item_key IN ('nav.resource.clusters', 'nav.resource.operations')
  AND capability = 'cluster.read';

UPDATE console_navigation_versions SET version_value = 'db-seed-v1', updated_at = now()
WHERE version_key = 'navigation';

COMMIT;
