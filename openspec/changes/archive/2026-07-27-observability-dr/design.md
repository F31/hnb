## Context

平台已有基础可观测性（健康检查 /healthz、/livez、/readyz、/startupz）和告警通知框架（Alert Store + NATS），但缺乏统一的跨平面遥测上下文、Operation SLO 监控、自动化备份恢复和故障演练能力。

## Goals / Non-Goals

**Goals:**
- 统一遥测上下文：所有组件输出结构化字段 (Tenant, CorrelationID, OperationID, ResourceID)
- Operation SLO 监控：非终态 Operation 配置最大滞留时间，超时告警
- 备份恢复产品化：平台元数据库的版本化备份策略与可执行恢复操作
- 故障演练框架：档位定义的故障场景与演练验证
- 性能预算门禁：发布前 P95/P99 基线检查

**Non-Goals:**
- 替换现有健康检查端点
- 实现边缘遥测缓存（OBS-006 由 Edge Pack 负责）
- 引入新中间件（复用现有 PostgreSQL + NATS）

## Decisions

### Decision 1: Operation SLO 使用 PostgreSQL 定时检查
定期查询 `operations` 表中非终态 Operation，比较 `last_state_changed_at` 与当前时间，超过阈值触发告警。无需引入新中间件。

### Decision 2: 备份恢复使用 PostgreSQL pg_dump/pg_restore
平台元数据全部存储在 PostgreSQL 中，备份策略使用 pg_dump 生成版本化备份文件，恢复操作通过 Runbook 执行。

### Decision 3: 故障演练定义为自动化测试
故障演练以 Go 测试形式实现，模拟 Pod 故障、节点故障、数据库主实例故障等场景。

## Data Model

### operation_slo_config 表
```sql
CREATE TABLE operation_slo_config (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_type  TEXT NOT NULL,
    max_duration    INTERVAL NOT NULL,
    alert_severity  TEXT NOT NULL DEFAULT 'warning',
    escalation_delay INTERVAL DEFAULT '5m',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### operation_slo_alerts 表
```sql
CREATE TABLE operation_slo_alerts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_id    TEXT NOT NULL,
    operation_type  TEXT NOT NULL,
    status          TEXT NOT NULL,
    stalled_since   TIMESTAMPTZ NOT NULL,
    alert_sent_at   TIMESTAMPTZ,
    escalated_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## Risks / Trade-offs

| 风险 | 缓解措施 |
|------|---------|
| SLO 检查增加数据库负载 | 低频率（每分钟）查询，配合索引 |
| 备份恢复测试影响生产 | 在测试环境演练，不触及生产 |