## Context

HNB 当前为单集群架构。生产环境需要：
- 跨集群工作负载调度（同城双活、异地灾备）
- 多集群统一控制面（Karmada 作为 K8s 多集群管理）
- 全局流量分发与故障转移（GSLB）

Karmada 作为 CNCF 毕业项目，提供 PropagationPolicy、OverridePolicy 等原生多集群原语。HNB 不重复实现多集群调度引擎，而是将 Karmada 作为 Provider 接入 Operation Engine。

## Goals / Non-Goals

**Goals:**
- 集群注册表：HNB 管理成员集群的生命周期（注册、心跳、摘除）
- Karmada 集成：通过 Karmada API 将 Operation 下发到多集群
- GSLB 流量分发：基于 DNS 的健康感知流量调度
- 跨集群 Operation 追踪：单 Operation 可跨集群执行，结果聚合

**Non-Goals:**
- 实现多集群调度引擎（Karmada 已有）
- 跨集群网络（CNI 多集群方案由基础设施团队管理）
- 多集群 Secret 同步（Karmada 已有 ResourceTemplate）
- 非 K8s 集群管理（仅限于 Karmada 成员集群）

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                     HNB Control Plane                        │
│  Portal → Platform API → Operation Worker → Karmada Provider │
└──────────────────────────────────────────────────────────────┘
                              │
                    ┌─────────┴──────────┐
                    ▼                    ▼
           ┌────────────────┐  ┌──────────────────┐
           │  Karmada API   │  │  GSLB Controller  │
           │  (Control      │  │  (CoreDNS +       │
           │   Plane)       │  │   External-DNS)   │
           └───────┬────────┘  └────────┬─────────┘
                   │                    │
         ┌─────────┼─────────┐         │
         ▼         ▼         ▼         ▼
    ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐
    │Cluster │ │Cluster │ │Cluster │ │  DNS   │
    │  A     │ │  B     │ │  C     │ │Record  │
    └────────┘ └────────┘ └────────┘ └────────┘
```

## Data Model

### clusters 表
```sql
CREATE TABLE clusters (
    id              UUID PRIMARY KEY,
    name            TEXT NOT NULL,
    tenant_id       TEXT NOT NULL,
    cluster_type    TEXT NOT NULL DEFAULT 'karmada',  -- karmada | standalone
    api_endpoint    TEXT NOT NULL,
    kubeconfig_ref  TEXT,                              -- SecretReference
    region          TEXT,
    zone            TEXT,
    labels          JSONB,
    status          TEXT NOT NULL DEFAULT 'pending',   -- pending | active | inactive | removed
    last_heartbeat  TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);
```

### cluster_heartbeats 表
```sql
CREATE TABLE cluster_heartbeats (
    id              UUID PRIMARY KEY,
    cluster_id      UUID NOT NULL REFERENCES clusters(id),
    status          TEXT NOT NULL,    -- healthy | degraded | unreachable
    version         TEXT,
    node_count      INT,
    capacity        JSONB,
    observed_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## Decisions

### Decision 1: Karmada 作为默认多集群控制面
Karmada 提供 PropagationPolicy、OverridePolicy、ResourceTemplate 等原生多集群原语，与 HNB 的 Provider 架构天然契合。HNB 不重复实现调度引擎。

### Decision 2: 集群注册表在 HNB 数据库中
成员集群信息存储在 HNB PostgreSQL 中，通过 Platform API 管理。Karmada 控制面本身不管理集群注册，而是由 HNB 统一管理。

### Decision 3: GSLB 基于 CoreDNS + External-DNS + 健康检查
GSLB 不引入新中间件，复用 CoreDNS + External-DNS 实现 DNS 级流量分发。健康检查通过 Karmada 或独立探针实现。

### Decision 4: 跨集群 Operation 通过 PropagationPolicy 实现
Operation Worker 在为多集群目标生成 ExecutionPlan 时，附加 PropagationPolicy 注解。Karmada Provider 将 Operation 翻译为 Karmada ResourceBinding。

## Risks / Trade-offs

| 风险 | 缓解措施 |
|------|---------|
| Karmada 版本兼容性 | 声明支持 Karmada v1.18+，CI 中运行 Conformance 测试 |
| 跨集群网络延迟 | 异步 Operation 模型天然容忍延迟；心跳超时可配置 |
| GSLB DNS 缓存 | TTL 可配置，支持主动 DNS 更新 |
| 集群摘除时残留资源 | Karmada 的 PropagationPolicy 的 `removeOrphan: true` 处理