-- 096: seed_default_gslb_demo_clusters
-- Description: Seed runtime_targets rows (cluster-a / cluster-b) for tenant
-- 'default' backing the 091 demo GSLB service pools. GSLB members still refer to
-- targets by name; validation and gslb-controller resolve those names from
-- runtime_targets rather than the legacy clusters table.
-- Dependencies: 091_seed_default_gslb_demo_service, 051_cluster_read_model_projection

BEGIN;

-- Self-contained demo tenant: migrations must be replayable, so the demo
-- cluster bindings cannot depend on application-created tenants. Creating the
-- tenant + its default workspace here (idempotently) lets migration 021's
-- backfill give the demo rows a workspace_id when that migration is replayed.
INSERT INTO tenants (id, name, display_name)
VALUES ('default', 'default', 'Default')
ON CONFLICT (id) DO NOTHING;

INSERT INTO workspaces (tenant_id, name, display_name)
SELECT id, 'default', 'Default' FROM tenants WHERE id = 'default'
ON CONFLICT (tenant_id, name) DO NOTHING;

INSERT INTO runtime_targets (
    id, tenant_id, name, display_name, target_type, connection_type,
    connection_endpoint, status, lifecycle_state, health_state,
    connectivity_state, freshness_state, labels, observed_at, is_active,
    created_at, updated_at
) VALUES
    (
        '00000000-0000-4000-8000-0000000000ca', 'default', 'cluster-a', 'cluster-a',
        'kubernetes', 'agent', NULL, 'online', 'ACTIVE',
        'HEALTHY', 'CONNECTED', 'FRESH', '{"gslbDemo":"true"}'::jsonb,
        now(), true, now(), now()
    ),
    (
        '00000000-0000-4000-8000-0000000000cb', 'default', 'cluster-b', 'cluster-b',
        'kubernetes', 'agent', NULL, 'online', 'ACTIVE',
        'HEALTHY', 'CONNECTED', 'FRESH', '{"gslbDemo":"true"}'::jsonb,
        now(), true, now(), now()
    )
ON CONFLICT (tenant_id, name) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    target_type = EXCLUDED.target_type,
    connection_type = EXCLUDED.connection_type,
    connection_endpoint = EXCLUDED.connection_endpoint,
    status = EXCLUDED.status,
    lifecycle_state = EXCLUDED.lifecycle_state,
    health_state = EXCLUDED.health_state,
    connectivity_state = EXCLUDED.connectivity_state,
    freshness_state = EXCLUDED.freshness_state,
    labels = COALESCE(runtime_targets.labels, '{}'::jsonb) || EXCLUDED.labels,
    observed_at = EXCLUDED.observed_at,
    is_active = EXCLUDED.is_active,
    updated_at = now();

COMMIT;
