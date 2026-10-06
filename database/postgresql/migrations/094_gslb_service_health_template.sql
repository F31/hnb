-- Migration: 094_gslb_service_health_template
-- Description: GSLB 三期/四期：服务级健康检查模板（protocol/path/interval/
--   timeout/failure_threshold，apiserver 创建 API 写入，gslb-controller 探测
--   时按服务读取）+ 租户内 domain 唯一约束（服务创建幂等/防重的数据库兜底）。
-- Dependencies: 081_gslb_traffic_resilience

BEGIN;

ALTER TABLE gslb_services
    ADD COLUMN IF NOT EXISTS health_protocol TEXT NOT NULL DEFAULT 'http'
        CHECK (health_protocol IN ('http', 'https')),
    ADD COLUMN IF NOT EXISTS health_path TEXT NOT NULL DEFAULT '/healthz'
        CHECK (health_path LIKE '/%'),
    ADD COLUMN IF NOT EXISTS health_interval_seconds INTEGER NOT NULL DEFAULT 10
        CHECK (health_interval_seconds BETWEEN 5 AND 300),
    ADD COLUMN IF NOT EXISTS health_timeout_seconds INTEGER NOT NULL DEFAULT 3
        CHECK (health_timeout_seconds BETWEEN 1 AND 30),
    ADD COLUMN IF NOT EXISTS health_failure_threshold INTEGER NOT NULL DEFAULT 3
        CHECK (health_failure_threshold BETWEEN 1 AND 10);

-- 租户内入口域名唯一（服务创建 API 幂等键；同租户同 domain 重复创建返回既有服务）
CREATE UNIQUE INDEX IF NOT EXISTS gslb_services_tenant_domain_key
    ON gslb_services(tenant_id, domain);

COMMIT;
