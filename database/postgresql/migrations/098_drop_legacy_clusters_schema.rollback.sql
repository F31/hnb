-- Rollback: recreate the empty legacy compatibility schema. Dropped table data
-- cannot be recovered by this rollback.

BEGIN;

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
    version         BIGINT NOT NULL DEFAULT 1,
    scope           TEXT NOT NULL DEFAULT 'tenant',
    UNIQUE (tenant_id, name)
);

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

COMMENT ON TABLE clusters IS 'DEPRECATED: compatibility-only legacy cluster registry. runtime_targets is the authoritative source for Kubernetes targets.';
COMMENT ON TABLE cluster_heartbeats IS 'DEPRECATED: compatibility-only legacy heartbeat history. capability_snapshots and runtime_targets observed state are authoritative.';

COMMIT;
