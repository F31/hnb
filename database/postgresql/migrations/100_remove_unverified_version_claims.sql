-- Withdraw unsupported release claims without removing operator-owned entries.
UPDATE platform_settings
SET value = jsonb_set(value, '{items}', COALESCE((
    SELECT jsonb_agg(
        CASE WHEN COALESCE(item->>'integrationReport', '') = ''
             THEN item || '{"integrationTestStatus":"pending","offlineBundleAvailable":false}'::jsonb
             WHEN COALESCE(item->>'offlineReport', '') = '' OR COALESCE(item->>'bundleDigest', '') = ''
             THEN item || '{"offlineBundleAvailable":false}'::jsonb
             ELSE item END
    ) FROM jsonb_array_elements(value->'items') AS item
), '[]'::jsonb)), updated_at = now()
WHERE key = 'resource.cluster.kubernetesVersions' AND jsonb_typeof(value->'items') = 'array';
