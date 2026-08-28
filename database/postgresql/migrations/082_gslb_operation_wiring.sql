-- Migration: 082_gslb_operation_wiring
-- Description: GSLB 平台 Operation 统一接线与演练报告落库
--   （OpenSpec change gslb-traffic-resilience，GSLB-005/009/010 补强）。
--   1) gslb_switch_requests.operation_id 关联平台 operations 行（Operation Center 统一观测）；
--   2) gslb_switch_requests.dr_group_ref：DRProtectionGroup 编排对接缝（GSLB-009，只记录引用）；
--   3) operations.operation_type 扩展 gslb_* 类型；
--   4) gslb_drill_reports 结构化只读演练报告（GSLB-010）。
-- Tiers: T2
-- Dependencies: 081_gslb_traffic_resilience, 008_operation_engine_core

BEGIN;

-- 1. 平台 Operation 行关联：每次受控流量变更在 operations 表有对应行，
--    由 gslb 写路径同事务创建、控制器执行结果同步状态。
ALTER TABLE gslb_switch_requests
    ADD COLUMN IF NOT EXISTS operation_id UUID REFERENCES operations(id);

-- 2. DR 保护组引用（GSLB-009 对接缝）：DRProtectionGroup 编排流量层步骤时
--    在意图中携带 drGroupRef，仅作审计/追踪引用，不携带任何执行细节。
ALTER TABLE gslb_switch_requests
    ADD COLUMN IF NOT EXISTS dr_group_ref TEXT;

-- 3. operation_type 扩展 GSLB 类型（008 的内联 CHECK 需重建）
ALTER TABLE operations DROP CONSTRAINT IF EXISTS operations_operation_type_check;
ALTER TABLE operations ADD CONSTRAINT operations_operation_type_check CHECK (operation_type IN (
    'deploy', 'upgrade', 'rollback', 'scale', 'backup',
    'restore', 'switchover', 'delete', 'gc', 'ota', 'config_change',
    'gslb_failover', 'gslb_switchback', 'gslb_weight_update', 'gslb_drill'
));

-- 4. 结构化演练报告（GSLB-010）：只读演练的计算结果独立落库，
--    供查询 API 与 Operation 详情展示。
CREATE TABLE IF NOT EXISTS gslb_drill_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    service_id UUID NOT NULL REFERENCES gslb_services(id) ON DELETE CASCADE,
    request_id UUID NOT NULL REFERENCES gslb_switch_requests(id) ON DELETE CASCADE,
    verdict TEXT NOT NULL CHECK (verdict IN ('Ready', 'Degraded', 'Blocked')),
    report JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gslb_drill_reports_service
    ON gslb_drill_reports(service_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_gslb_drill_reports_tenant
    ON gslb_drill_reports(tenant_id, created_at DESC);

-- 5. 最近演练写入 Read Model（GSLB-010：演练报告写入 Read Model）
ALTER TABLE gslb_read_model
    ADD COLUMN IF NOT EXISTS last_drill_report_id UUID,
    ADD COLUMN IF NOT EXISTS last_drill_verdict TEXT,
    ADD COLUMN IF NOT EXISTS last_drill_at TIMESTAMPTZ;

COMMIT;
