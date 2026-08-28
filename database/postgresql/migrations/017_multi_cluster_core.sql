-- 017_multi_cluster_core.sql
-- Multi-cluster governance: cluster registry, heartbeats

BEGIN;

-- Cluster registry
CREATE TABLE IF NOT EXISTS clusters (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    tenant_id       TEXT NOT NULL,
    cluster_type    TEXT NOT NULL DEFAULT 'karmada',
    api_endpoint    TEXT NOT NULL,
    kubeconfig_ref  TEXT,
    region          TEXT,
    zone            TEXT,
    labels          JSONB DEFAULT '{}',
    status          TEXT NOT NULL DEFAULT 'pending',
    last_heartbeat  TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);

-- Cluster heartbeat records
CREATE TABLE IF NOT EXISTS cluster_heartbeats (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id      UUID NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'healthy',
    version         TEXT,
    node_count      INT,
    capacity        JSONB DEFAULT '{}',
    observed_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_clusters_tenant ON clusters(tenant_id);
CREATE INDEX IF NOT EXISTS idx_clusters_status ON clusters(status);
CREATE INDEX IF NOT EXISTS idx_cluster_heartbeats_cluster ON cluster_heartbeats(cluster_id, observed_at DESC);

COMMIT;
