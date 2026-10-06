-- 093: hide_resource_gpu_navigation
-- Description: Remove GPU from the Resource first-level navigation while
-- leaving the route/component intact for direct links and future reuse.
-- Dependencies: 092_restore_resource_plugin_market_navigation

BEGIN;

UPDATE console_navigation_items
SET enabled = false, updated_at = now()
WHERE item_key = 'nav.resource.gpu';

INSERT INTO console_navigation_versions (version_key, version_value) VALUES
    ('navigation', 'hide-resource-gpu-v1')
ON CONFLICT (version_key) DO UPDATE SET
    version_value = EXCLUDED.version_value,
    updated_at = now();

COMMIT;
