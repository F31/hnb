-- 088: fix_cluster_list_schema_endpoint
-- Description: Migration 079 seeded the cluster-list PageSchema with an
-- endpoint path of "/api/v1/clusters", which is the legacy route that forwards
-- to platform-api and is not reachable from the console. The canonical BFF
-- route is "/api/v1/resources/clusters". This corrects any already-seeded
-- cluster-list schema so SchemaPage renders against the working endpoint.
-- Dependencies: 079_ui_page_registry

BEGIN;

UPDATE ui_page_versions
SET payload = jsonb_set(
    payload,
    '{spec,endpoints}',
    (
        SELECT jsonb_agg(
            CASE WHEN ep->>'id' = 'clusters.list'
                 THEN jsonb_set(ep, '{path}', to_jsonb('/api/v1/resources/clusters'::text))
                 ELSE ep
            END
        )
        FROM jsonb_array_elements(payload->'spec'->'endpoints') ep
    ),
    false
)
WHERE page_id = 'cluster-list'
  AND EXISTS (
      SELECT 1
      FROM jsonb_array_elements(payload->'spec'->'endpoints') ep
      WHERE ep->>'id' = 'clusters.list' AND ep->>'path' = '/api/v1/clusters'
  );

-- Bump the navigation version so clients refresh their cached catalog and
-- re-fetch the corrected schema (V2.6 §15.3 版本化缓存键).
INSERT INTO console_navigation_versions (version_key, version_value) VALUES
    ('navigation', 'fix-cluster-list-endpoint-v2')
ON CONFLICT (version_key) DO UPDATE SET
    version_value = EXCLUDED.version_value,
    updated_at = now();

COMMIT;

