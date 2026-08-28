-- Migration: 081_gslb_traffic_resilience
-- Description: GSLB 流量层容灾数据面（OpenSpec change gslb-traffic-resilience，GSLB-005/007/008）。
--   服务/池/成员/健康检查 + 只读投影 + 审批门控的流量变更请求（受控写路径）。
-- Tiers: T2
-- Dependencies: 010_runtime_target_engine

BEGIN;

CREATE TABLE IF NOT EXISTS gslb_services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    domain TEXT NOT NULL,
    routing_mode TEXT NOT NULL DEFAULT 'dns' CHECK (routing_mode IN ('dns', 'anycast')),
    active_pool_id UUID,
    lifecycle_state TEXT NOT NULL DEFAULT 'Inactive' CHECK (lifecycle_state IN (
        'Inactive', 'Provisioning', 'Active', 'Degraded',
        'FailingOver', 'Paused', 'Disabled', 'Terminated'
    )),
    require_approval BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);

CREATE TABLE IF NOT EXISTS gslb_pools (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES gslb_services(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (service_id, name)
);

CREATE TABLE IF NOT EXISTS gslb_pool_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pool_id UUID NOT NULL REFERENCES gslb_pools(id) ON DELETE CASCADE,
    cluster_id TEXT NOT NULL,
    weight INTEGER NOT NULL DEFAULT 100 CHECK (weight BETWEEN 0 AND 100),
    enabled BOOLEAN NOT NULL DEFAULT true,
    healthy BOOLEAN NOT NULL DEFAULT false,
    last_health_at TIMESTAMPTZ,
    UNIQUE (pool_id, cluster_id)
);

CREATE TABLE IF NOT EXISTS gslb_health_checks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES gslb_services(id) ON DELETE CASCADE,
    probe_type TEXT NOT NULL CHECK (probe_type IN ('apiserver', 'http', 'karmada', 'manual')),
    interval_seconds INTEGER NOT NULL DEFAULT 30,
    timeout_seconds INTEGER NOT NULL DEFAULT 5,
    failure_threshold INTEGER NOT NULL DEFAULT 3,
    cool_down_seconds INTEGER NOT NULL DEFAULT 60,
    config JSONB NOT NULL DEFAULT '{}'
);

-- 只读投影：平台查询只读本表（GSLB-007），由 gslb-controller 投影器更新
CREATE TABLE IF NOT EXISTS gslb_read_model (
    service_id UUID PRIMARY KEY REFERENCES gslb_services(id) ON DELETE CASCADE,
    tenant_id TEXT NOT NULL,
    domain TEXT NOT NULL,
    active_pool_id UUID,
    lifecycle_state TEXT NOT NULL,
    healthy_pools TEXT[] NOT NULL DEFAULT '{}',
    current_dns_targets TEXT[] NOT NULL DEFAULT '{}',
    last_switch_request_id UUID,
    last_switch_at TIMESTAMPTZ,
    observed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 审批门控的流量变更请求（GSLB-005 受控写路径）：
-- 意图 + 不可变计划快照 + 审批生命周期；DNS 执行经 Outbox 命令投递给
-- gslb-controller 执行器（禁止控制器直写 DNS 数据面）。
CREATE TABLE IF NOT EXISTS gslb_switch_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    service_id UUID NOT NULL REFERENCES gslb_services(id) ON DELETE CASCADE,
    intent_kind TEXT NOT NULL CHECK (intent_kind IN (
        'gslb.failover', 'gslb.switchback', 'gslb.weight-update', 'gslb.drill'
    )),
    intent_digest TEXT NOT NULL,
    plan_snapshot JSONB NOT NULL,
    idempotency_key TEXT NOT NULL,
    correlation_id UUID NOT NULL,
    require_approval BOOLEAN NOT NULL DEFAULT true,
    status TEXT NOT NULL DEFAULT 'PendingApproval' CHECK (status IN (
        'PendingApproval', 'Approved', 'Rejected', 'Dispatched', 'Succeeded', 'Failed', 'DrillCompleted'
    )),
    actor_id TEXT,
    approved_by TEXT,
    approved_at TIMESTAMPTZ,
    reason TEXT,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, service_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_gslb_switch_requests_service
    ON gslb_switch_requests(service_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_gslb_switch_requests_status
    ON gslb_switch_requests(status);
CREATE INDEX IF NOT EXISTS idx_gslb_pools_service
    ON gslb_pools(service_id);
CREATE INDEX IF NOT EXISTS idx_gslb_members_pool
    ON gslb_pool_members(pool_id);

COMMIT;
