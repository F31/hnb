-- 091: seed_default_gslb_demo_service
-- Description: Seed a default-tenant GSLB service with active/backup pools so
-- the Resource > GSLB page can exercise drill, approval and failover closure in
-- a fresh development environment. Member health and the read model are left at
-- their unprobed defaults (healthy=false, empty targets) — the gslb-controller
-- writes real probe results; no fabricated health is seeded.
-- Dependencies: 090_restore_resource_menu_page_bindings

BEGIN;

INSERT INTO gslb_services (
    id, tenant_id, name, domain, routing_mode, active_pool_id,
    lifecycle_state, require_approval, created_at, updated_at
) VALUES (
    '00000000-0000-4000-8000-0000000000a1',
    'default',
    'demo-api-gateway',
    'api.demo.hnb.cloud',
    'dns',
    '00000000-0000-4000-8000-0000000000b1',
    'Active',
    true,
    now(),
    now()
) ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    name = EXCLUDED.name,
    domain = EXCLUDED.domain,
    routing_mode = EXCLUDED.routing_mode,
    active_pool_id = EXCLUDED.active_pool_id,
    lifecycle_state = EXCLUDED.lifecycle_state,
    require_approval = EXCLUDED.require_approval,
    updated_at = now();

INSERT INTO gslb_pools (id, service_id, name, priority) VALUES
    ('00000000-0000-4000-8000-0000000000b1', '00000000-0000-4000-8000-0000000000a1', 'primary', 0),
    ('00000000-0000-4000-8000-0000000000b2', '00000000-0000-4000-8000-0000000000a1', 'standby', 1)
ON CONFLICT (id) DO UPDATE SET
    service_id = EXCLUDED.service_id,
    name = EXCLUDED.name,
    priority = EXCLUDED.priority;

-- Members reference the runtime_targets demo entries (096); health stays at the
-- column default (false) until the controller probes them.
INSERT INTO gslb_pool_members (id, pool_id, cluster_id, weight, enabled) VALUES
    ('00000000-0000-4000-8000-0000000000c1', '00000000-0000-4000-8000-0000000000b1', 'cluster-a', 100, true),
    ('00000000-0000-4000-8000-0000000000c2', '00000000-0000-4000-8000-0000000000b2', 'cluster-b', 100, true)
ON CONFLICT (id) DO UPDATE SET
    pool_id = EXCLUDED.pool_id,
    cluster_id = EXCLUDED.cluster_id,
    weight = EXCLUDED.weight,
    enabled = EXCLUDED.enabled;

-- Read model starts unprobed (empty healthy pools / dns targets); the
-- controller populates real state on its first reconcile.
INSERT INTO gslb_read_model (
    service_id, tenant_id, domain, active_pool_id, lifecycle_state,
    healthy_pools, current_dns_targets, observed_at
) VALUES (
    '00000000-0000-4000-8000-0000000000a1',
    'default',
    'api.demo.hnb.cloud',
    '00000000-0000-4000-8000-0000000000b1',
    'Active',
    '{}'::text[],
    '{}'::text[],
    now()
) ON CONFLICT (service_id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    domain = EXCLUDED.domain,
    active_pool_id = EXCLUDED.active_pool_id,
    lifecycle_state = EXCLUDED.lifecycle_state,
    healthy_pools = EXCLUDED.healthy_pools,
    current_dns_targets = EXCLUDED.current_dns_targets,
    observed_at = now();

COMMIT;
