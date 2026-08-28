-- Migration: 083_dr_protection_group
-- Description: DRProtectionGroup 容灾编排（OpenSpec change dr-protection-group，OBS-008）。
--   保护组（租户隔离）+ 成员（数据层引用 / 流量层 GSLBService）+ 切换运行（顺序编排与审计）。
-- Tiers: T2
-- Dependencies: 082_gslb_operation_wiring

BEGIN;

CREATE TABLE IF NOT EXISTS dr_protection_groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    primary_region TEXT NOT NULL,
    standby_region TEXT NOT NULL,
    lifecycle_state TEXT NOT NULL DEFAULT 'Ready' CHECK (lifecycle_state IN (
        'Ready', 'Switching', 'FailedOver', 'SwitchingBack'
    )),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);

CREATE TABLE IF NOT EXISTS dr_group_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id UUID NOT NULL REFERENCES dr_protection_groups(id) ON DELETE CASCADE,
    member_type TEXT NOT NULL CHECK (member_type IN ('gslb_service', 'data_layer_ref')),
    ref_id TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (group_id, member_type, ref_id)
);

CREATE INDEX IF NOT EXISTS idx_dr_group_members_group
    ON dr_group_members(group_id);

-- 切换运行：数据层确认门 → 流量层 gslb 意图（drGroupRef）；
-- operation_id 关联平台 operations 行（Operation Center 统一观测），
-- traffic_request_ids 追踪全部子 gslb 切换请求。
CREATE TABLE IF NOT EXISTS dr_switch_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id UUID NOT NULL REFERENCES dr_protection_groups(id) ON DELETE CASCADE,
    tenant_id TEXT NOT NULL,
    direction TEXT NOT NULL CHECK (direction IN ('failover', 'switchback')),
    status TEXT NOT NULL DEFAULT 'DataLayerPending' CHECK (status IN (
        'DataLayerPending', 'DataLayerCompleted', 'TrafficDispatched',
        'AwaitingApproval', 'Completed', 'Failed', 'Cancelled'
    )),
    idempotency_key TEXT NOT NULL,
    correlation_id UUID NOT NULL,
    operation_id UUID REFERENCES operations(id),
    traffic_request_ids UUID[] NOT NULL DEFAULT '{}',
    reason TEXT,
    error TEXT,
    actor_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (group_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_dr_switch_runs_group
    ON dr_switch_runs(group_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dr_switch_runs_status
    ON dr_switch_runs(status);

COMMIT;
