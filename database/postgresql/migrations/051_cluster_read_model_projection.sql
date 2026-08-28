-- Migration: 051_cluster_read_model_projection
-- Description: Add the tenant-safe cluster observation projection and query indexes.
-- Dependencies: 010_runtime_target_engine, 025_runtime_intent_audit,
--               048_runtime_target_nodes, 050_cluster_permissions

BEGIN;

ALTER TABLE runtime_targets
    ADD COLUMN IF NOT EXISTS lifecycle_state TEXT,
    ADD COLUMN IF NOT EXISTS health_state TEXT,
    ADD COLUMN IF NOT EXISTS connectivity_state TEXT,
    ADD COLUMN IF NOT EXISTS freshness_state TEXT,
    ADD COLUMN IF NOT EXISTS last_known_state_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS observation_source TEXT,
    ADD COLUMN IF NOT EXISTS observation_source_id TEXT,
    ADD COLUMN IF NOT EXISTS observation_generation BIGINT,
    ADD COLUMN IF NOT EXISTS observation_revision BIGINT,
    ADD COLUMN IF NOT EXISTS projection_version BIGINT NOT NULL DEFAULT 0;

-- Legacy state is deliberately mapped conservatively. No legacy value implies
-- healthy or connected unless it explicitly represented an online target.
UPDATE runtime_targets
SET lifecycle_state = COALESCE(lifecycle_state, CASE status
        WHEN 'online' THEN 'ACTIVE'
        WHEN 'decommissioned' THEN 'TERMINATED'
        ELSE 'UNKNOWN'
    END),
    health_state = COALESCE(health_state, CASE status
        WHEN 'online' THEN 'HEALTHY'
        WHEN 'degraded' THEN 'DEGRADED'
        ELSE 'UNKNOWN'
    END),
    connectivity_state = COALESCE(connectivity_state, CASE status
        WHEN 'online' THEN 'CONNECTED'
        WHEN 'offline' THEN 'DISCONNECTED'
        ELSE 'UNKNOWN'
    END),
    freshness_state = COALESCE(freshness_state, CASE
        WHEN observed_at IS NULL THEN 'UNKNOWN'
        WHEN observed_at + stale_threshold_seconds * interval '1 second' >= now() THEN 'FRESH'
        ELSE 'STALE'
    END),
    last_known_state_at = COALESCE(last_known_state_at, observed_at),
    observation_source = COALESCE(observation_source, CASE connection_type
        WHEN 'agent' THEN 'agent'
        WHEN 'cloudhub' THEN 'cloudcore'
        ELSE NULL
    END),
    observation_source_id = COALESCE(observation_source_id, CASE
        WHEN connection_type IN ('agent', 'cloudhub') THEN 'legacy:' || id::text
        ELSE NULL
    END),
    observation_generation = COALESCE(observation_generation, CASE WHEN observed_at IS NOT NULL THEN 0 END),
    observation_revision = COALESCE(observation_revision, CASE WHEN observed_at IS NOT NULL THEN 0 END);

ALTER TABLE runtime_targets
    ALTER COLUMN lifecycle_state SET DEFAULT 'UNKNOWN',
    ALTER COLUMN health_state SET DEFAULT 'UNKNOWN',
    ALTER COLUMN connectivity_state SET DEFAULT 'UNKNOWN',
    ALTER COLUMN freshness_state SET DEFAULT 'UNKNOWN';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_runtime_targets_lifecycle_state') THEN
        ALTER TABLE runtime_targets ADD CONSTRAINT chk_runtime_targets_lifecycle_state
            CHECK (lifecycle_state IS NULL OR lifecycle_state IN
                ('UNKNOWN','REGISTERING','PROVISIONING','ACTIVE','UPGRADING','FAILED','DELETING','TERMINATED'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_runtime_targets_health_state') THEN
        ALTER TABLE runtime_targets ADD CONSTRAINT chk_runtime_targets_health_state
            CHECK (health_state IS NULL OR health_state IN ('UNKNOWN','HEALTHY','DEGRADED','UNHEALTHY'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_runtime_targets_connectivity_state') THEN
        ALTER TABLE runtime_targets ADD CONSTRAINT chk_runtime_targets_connectivity_state
            CHECK (connectivity_state IS NULL OR connectivity_state IN ('UNKNOWN','CONNECTED','DISCONNECTED'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_runtime_targets_freshness_state') THEN
        ALTER TABLE runtime_targets ADD CONSTRAINT chk_runtime_targets_freshness_state
            CHECK (freshness_state IS NULL OR freshness_state IN ('UNKNOWN','FRESH','STALE'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_runtime_targets_observation_source') THEN
        ALTER TABLE runtime_targets ADD CONSTRAINT chk_runtime_targets_observation_source
            CHECK (observation_source IS NULL OR observation_source IN ('agent','cloudcore'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_runtime_targets_observation_revision') THEN
        ALTER TABLE runtime_targets ADD CONSTRAINT chk_runtime_targets_observation_revision
            CHECK ((observation_generation IS NULL AND observation_revision IS NULL) OR
                   (observation_generation >= 0 AND observation_revision >= 0));
    END IF;
END
$$;

ALTER TABLE capability_snapshots
    ADD COLUMN IF NOT EXISTS tenant_id TEXT,
    ADD COLUMN IF NOT EXISTS target_kind TEXT,
    ADD COLUMN IF NOT EXISTS observation_source TEXT,
    ADD COLUMN IF NOT EXISTS observation_source_id TEXT,
    ADD COLUMN IF NOT EXISTS observation_generation BIGINT,
    ADD COLUMN IF NOT EXISTS observation_revision BIGINT,
    ADD COLUMN IF NOT EXISTS content_digest TEXT;

UPDATE capability_snapshots cs
SET tenant_id = rt.tenant_id,
    target_kind = CASE rt.target_type
        WHEN 'kubernetes' THEN 'KubernetesTarget'
        WHEN 'edge_runtime' THEN 'EdgeRuntimeTarget'
        ELSE NULL
    END,
    observation_source = COALESCE(cs.observation_source, rt.observation_source),
    observation_source_id = COALESCE(cs.observation_source_id, rt.observation_source_id),
    observation_generation = COALESCE(cs.observation_generation, rt.observation_generation),
    observation_revision = COALESCE(cs.observation_revision, rt.observation_revision)
FROM runtime_targets rt
WHERE rt.id = cs.target_id AND cs.tenant_id IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_capability_snapshots_target_tenant') THEN
        ALTER TABLE capability_snapshots ADD CONSTRAINT fk_capability_snapshots_target_tenant
            FOREIGN KEY (target_id, tenant_id) REFERENCES runtime_targets(id, tenant_id) ON DELETE CASCADE;
    END IF;
END
$$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_capability_snapshots_source_revision
    ON capability_snapshots(tenant_id, target_id, observation_source, observation_source_id,
                            observation_generation, observation_revision)
    WHERE observation_source IS NOT NULL AND observation_source_id IS NOT NULL
      AND observation_generation IS NOT NULL AND observation_revision IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_capability_snapshots_target_digest
    ON capability_snapshots(tenant_id, target_id, content_digest)
    WHERE content_digest IS NOT NULL;

CREATE TABLE IF NOT EXISTS runtime_target_projection_quarantine (
    id BIGSERIAL PRIMARY KEY,
    source_table TEXT NOT NULL,
    source_row_id TEXT NOT NULL,
    reason TEXT NOT NULL,
    detail JSONB NOT NULL DEFAULT '{}',
    reported_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source_table, source_row_id, reason)
);

ALTER TABLE runtime_target_nodes
    ADD COLUMN IF NOT EXISTS tenant_id TEXT,
    ADD COLUMN IF NOT EXISTS source_node_uid TEXT,
    ADD COLUMN IF NOT EXISTS observed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_known_state_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS lifecycle_state TEXT,
    ADD COLUMN IF NOT EXISTS health_state TEXT,
    ADD COLUMN IF NOT EXISTS connectivity_state TEXT,
    ADD COLUMN IF NOT EXISTS observation_source TEXT,
    ADD COLUMN IF NOT EXISTS observation_source_id TEXT,
    ADD COLUMN IF NOT EXISTS observation_generation BIGINT,
    ADD COLUMN IF NOT EXISTS observation_revision BIGINT,
    ADD COLUMN IF NOT EXISTS labels JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS capacity JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

INSERT INTO runtime_target_projection_quarantine (source_table, source_row_id, reason, detail)
SELECT 'runtime_target_nodes', n.id::text, 'broken_target_ownership',
       jsonb_build_object('targetId', n.target_id)
FROM runtime_target_nodes n
LEFT JOIN runtime_targets rt ON rt.id = n.target_id
WHERE rt.id IS NULL
ON CONFLICT DO NOTHING;

UPDATE runtime_target_nodes n
SET tenant_id = rt.tenant_id,
    source_node_uid = COALESCE(n.source_node_uid, n.id::text),
    observed_at = COALESCE(n.observed_at, n.last_heartbeat_at, n.updated_at),
    last_known_state_at = COALESCE(n.last_known_state_at, n.last_heartbeat_at, n.updated_at),
    lifecycle_state = COALESCE(n.lifecycle_state, 'UNKNOWN'),
    health_state = COALESCE(n.health_state, CASE n.node_status
        WHEN 'Ready' THEN 'HEALTHY'
        WHEN 'NotReady' THEN 'UNHEALTHY'
        ELSE 'UNKNOWN'
    END),
    connectivity_state = COALESCE(n.connectivity_state, CASE n.node_status
        WHEN 'Ready' THEN 'CONNECTED'
        WHEN 'NotReady' THEN 'DISCONNECTED'
        ELSE 'UNKNOWN'
    END),
    observation_source = COALESCE(n.observation_source, rt.observation_source),
    observation_source_id = COALESCE(n.observation_source_id, rt.observation_source_id),
    observation_generation = COALESCE(n.observation_generation, rt.observation_generation, 0),
    observation_revision = COALESCE(n.observation_revision, rt.observation_revision, 0)
FROM runtime_targets rt
WHERE rt.id = n.target_id;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_runtime_target_nodes_target_tenant') THEN
        ALTER TABLE runtime_target_nodes ADD CONSTRAINT fk_runtime_target_nodes_target_tenant
            FOREIGN KEY (target_id, tenant_id) REFERENCES runtime_targets(id, tenant_id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_runtime_target_nodes_source') THEN
        ALTER TABLE runtime_target_nodes ADD CONSTRAINT chk_runtime_target_nodes_source
            CHECK (observation_source IS NULL OR observation_source IN ('agent','cloudcore'));
    END IF;
END
$$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_runtime_target_nodes_source_uid
    ON runtime_target_nodes(tenant_id, target_id, source_node_uid)
    WHERE source_node_uid IS NOT NULL;

CREATE TABLE IF NOT EXISTS runtime_target_observation_cursors (
    tenant_id TEXT NOT NULL,
    target_id UUID NOT NULL,
    observation_source TEXT NOT NULL CHECK (observation_source IN ('agent','cloudcore')),
    observation_source_id TEXT NOT NULL,
    observation_generation BIGINT NOT NULL CHECK (observation_generation >= 0),
    observation_revision BIGINT NOT NULL CHECK (observation_revision >= 0),
    payload_digest TEXT NOT NULL,
    last_message_id UUID NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, target_id, observation_source, observation_source_id),
    FOREIGN KEY (target_id, tenant_id) REFERENCES runtime_targets(id, tenant_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS runtime_target_observation_inbox (
    message_id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    target_id UUID NOT NULL,
    observation_source TEXT NOT NULL CHECK (observation_source IN ('agent','cloudcore')),
    observation_source_id TEXT NOT NULL,
    observation_generation BIGINT NOT NULL CHECK (observation_generation >= 0),
    observation_revision BIGINT NOT NULL CHECK (observation_revision >= 0),
    payload_digest TEXT NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ,
    processing_error TEXT,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (target_id, tenant_id) REFERENCES runtime_targets(id, tenant_id) ON DELETE CASCADE,
    UNIQUE (tenant_id, target_id, observation_source, observation_source_id,
            observation_generation, observation_revision)
);

CREATE INDEX IF NOT EXISTS idx_runtime_targets_cluster_stable
    ON runtime_targets(tenant_id, updated_at DESC, id DESC)
    WHERE is_active AND target_type IN ('kubernetes','edge_runtime');
CREATE INDEX IF NOT EXISTS idx_runtime_targets_cluster_states
    ON runtime_targets(tenant_id, target_type, lifecycle_state, health_state,
                       connectivity_state, freshness_state, updated_at DESC, id DESC)
    WHERE is_active AND target_type IN ('kubernetes','edge_runtime');
CREATE INDEX IF NOT EXISTS idx_runtime_target_nodes_tenant_target_name
    ON runtime_target_nodes(tenant_id, target_id, name, source_node_uid)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_runtime_target_nodes_tenant_target_status
    ON runtime_target_nodes(tenant_id, target_id, lifecycle_state, health_state,
                            connectivity_state, observed_at DESC, name, source_node_uid)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_runtime_target_observation_inbox_pending
    ON runtime_target_observation_inbox(received_at)
    WHERE processed_at IS NULL;

-- Existing JSON tags remain the compatibility storage. These expression indexes
-- support canonical targetId/targetKind filtering without rewriting history.
CREATE INDEX IF NOT EXISTS idx_operations_target_tags
    ON operations(tenant_id, (tags->>'targetId'), (tags->>'targetKind'));
CREATE INDEX IF NOT EXISTS idx_operation_read_model_target_tags
    ON operation_read_model(tenant_id, (tags->>'targetId'), (tags->>'targetKind'));

COMMIT;
