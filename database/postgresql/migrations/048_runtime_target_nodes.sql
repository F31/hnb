-- Migration: 048_runtime_target_nodes
-- Description: Add runtime_target_nodes and bff_runtime_intents tables for the
--              cluster-management Read Model and the apiserver BFF RuntimeIntent
--              receipt (standalone/dev path). The canonical runtime_intents table
--              is owned by the platform-api (migration 025) and is NOT recreated here.
-- Tiers: All
-- Dependencies: 010_runtime_target_engine (runtime_targets)

-- 1. RuntimeTargetNodes (agent / CloudCore reported node read-model, RT-006/RT-007)
CREATE TABLE IF NOT EXISTS runtime_target_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    target_id UUID NOT NULL REFERENCES runtime_targets(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'worker' CHECK (role IN ('control_plane', 'worker', 'edge')),
    node_status TEXT NOT NULL DEFAULT 'Unknown' CHECK (node_status IN ('Ready', 'NotReady', 'Unknown')),
    ip_address TEXT,
    os TEXT NOT NULL DEFAULT '',
    arch TEXT NOT NULL DEFAULT '',
    cpu_allocatable TEXT NOT NULL DEFAULT '',
    memory_allocatable TEXT NOT NULL DEFAULT '',
    kubelet_version TEXT NOT NULL DEFAULT '',
    last_heartbeat_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (target_id, name)
);

CREATE INDEX IF NOT EXISTS idx_runtime_target_nodes_target_id ON runtime_target_nodes(target_id);
CREATE INDEX IF NOT EXISTS idx_runtime_target_nodes_status ON runtime_target_nodes(node_status);

-- 2. BFF runtime intents (apiserver-side receipt used only when platform-api is
--    not configured, i.e. standalone/dev. Never a write bypass: when platform-api
--    is present the BFF forwards to POST /v1/intents and this table is unused.)
CREATE TABLE IF NOT EXISTS bff_runtime_intents (
    intent_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    api_version TEXT NOT NULL,
    kind TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    target_ref TEXT NOT NULL,
    scope_ref TEXT NOT NULL,
    parameters JSONB NOT NULL DEFAULT '{}',
    secret_references JSONB NOT NULL DEFAULT '[]',
    status TEXT NOT NULL DEFAULT 'received' CHECK (status IN ('received', 'validated', 'planned', 'operation_committed', 'rejected')),
    operation_id UUID,
    semantic_digest TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_bff_runtime_intents_tenant_id ON bff_runtime_intents(tenant_id);
CREATE INDEX IF NOT EXISTS idx_bff_runtime_intents_kind ON bff_runtime_intents(kind);
CREATE INDEX IF NOT EXISTS idx_bff_runtime_intents_status ON bff_runtime_intents(status);
