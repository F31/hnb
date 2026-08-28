-- 066: cluster_management_capability_gates
-- Description: Attach staged cluster capability gates to console routes and
--   navigation items so a disabled stage removes the menu/route from the
--   published navigation (KERNEL-016). The server additionally fail-closes
--   every gated API route via capability.Set; this migration only gates the
--   console entry points.
-- Dependencies: 065_shared_clusters

BEGIN;

-- Resource cluster routes are published only when the read stage is enabled.
UPDATE console_routes
SET capability = 'cluster.read', updated_at = now()
WHERE route_name IN (
    'resource.clusters'
)
  AND capability = '';

UPDATE console_routes
SET capability = 'cluster.read', updated_at = now()
WHERE route_name IN (
    'resource.operations'
)
  AND capability = '';

-- Navigation items inherit the gated route capability so the whole entry is
-- removed when the stage is disabled.
UPDATE console_navigation_items
SET capability = 'cluster.read', updated_at = now()
WHERE item_key IN (
    'nav.resource.clusters',
    'nav.resource.operations'
)
  AND capability = '';

INSERT INTO console_navigation_versions (version_key, version_value) VALUES
    ('navigation', 'db-capability-gates-v1')
ON CONFLICT (version_key) DO UPDATE SET version_value = EXCLUDED.version_value, updated_at = now();

COMMIT;
