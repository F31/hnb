-- 088: fix_cluster_list_schema_endpoint
-- Description: Migration 079 seeded the cluster-list PageSchema with an
-- endpoint path of "/api/v1/clusters", the legacy route that forwards to
-- platform-api and is not reachable from the console. The canonical BFF route
-- is "/api/v1/resources/clusters".
--
-- The schema is served with an ETag derived from the page's active_revision,
-- so correcting the payload in place does NOT invalidate browser caches (a
-- 304 is still served and the stale "/api/v1/clusters" path keeps being used,
-- surfacing as a spurious "会话已过期" on the console). Publishing a NEW
-- revision unconditionally bumps the ETag (page-cluster-list-r2), forcing
-- clients to refetch the corrected schema regardless of the current active
-- revision's payload (including deployments that were hand-corrected in place).
-- Dependencies: 079_ui_page_registry

BEGIN;

-- Build revision N+1 from the current active revision, forcing the
-- clusters.list endpoint path and bumping metadata.revision.
INSERT INTO ui_page_versions (page_id, revision, payload, created_by)
SELECT
    p.page_id,
    p.active_revision + 1,
    jsonb_set(
        jsonb_set(
            base.payload,
            '{metadata,revision}',
            to_jsonb(p.active_revision + 1)
        ),
        '{spec,endpoints}',
        (
            SELECT jsonb_agg(
                CASE WHEN ep->>'id' = 'clusters.list'
                     THEN jsonb_set(ep, '{path}', to_jsonb('/api/v1/resources/clusters'::text))
                     ELSE ep
                END
            )
            FROM jsonb_array_elements(base.payload->'spec'->'endpoints') ep
        ),
        false
    ),
    'system'
FROM ui_pages p
JOIN ui_page_versions base
  ON base.page_id = p.page_id AND base.revision = p.active_revision
WHERE p.page_id = 'cluster-list';

-- Promote the new revision as active so the served ETag changes.
UPDATE ui_pages
SET active_revision = active_revision + 1, updated_at = now()
WHERE page_id = 'cluster-list'
  AND EXISTS (
      SELECT 1 FROM ui_page_versions v
      WHERE v.page_id = ui_pages.page_id AND v.revision = ui_pages.active_revision + 1
  );

-- Bump the navigation version so clients refresh their cached catalog and
-- re-fetch the corrected schema (V2.6 §15.3 版本化缓存键).
INSERT INTO console_navigation_versions (version_key, version_value) VALUES
    ('navigation', 'fix-cluster-list-endpoint-v2')
ON CONFLICT (version_key) DO UPDATE SET
    version_value = EXCLUDED.version_value,
    updated_at = now();

COMMIT;
