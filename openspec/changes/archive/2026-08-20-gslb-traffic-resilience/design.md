# Design: gslb-traffic-resilience

## Context

平台已有流量层容灾方向（白皮书 §8.1"入口/统一域名故障 → Gateway API + GSLB"）与多地域部署形态（§6"多 Region + GSLB"），但 GSLB 落地存在执行旁路、零契约、产品面空缺三类欠账（见 proposal）。本设计将 GSLB 定位为**流量层容灾与多地域入口编排的受控能力**：四层容灾模型的流量层执行器、多地域形态的入口组件，服从平台铁律（Operation 唯一写入口、Provider 可插拔、Schema First、CQRS、多租户）。

## Goals / Non-Goals

**Goals:**
- 消除 GSLB 执行旁路：所有流量变更经 Operation Engine，可审计、可审批、可回滚
- DNS 数据面 Provider 化：内置 ExternalDNS 参考实现，外部实现经 Conformance 认证
- Schema First：gslb 契约入 `contracts/`，生成 SDK
- CQRS：健康/状态/切换历史投影 Read Model，查询不碰数据面
- 多租户隔离：GSLBService 归属租户，策略与视图隔离
- 与 DRProtectionGroup 级联：数据层 → 流量层按序编排，回切显式确认
- 故障演练：只读演练产出报告，不产生真实流量变更
- Web Console 资源 → GSLB 页面替换 stub（列表/详情/动作）

**Non-Goals:**
- DNS 记录级编辑、Ingress/Gateway 配置替代、应用层负载均衡（见 proposal Non-Goals）
- 自动切换免审批（保持默认 `require_approval`，可配置降级）
- 会话保持 / 地域偏好 / Anycast 原生能力（后续 change）
- 单地域 T1 形态的强制部署

## Architecture

```text
用户请求
   ↓ 全局入口（统一域名 / Anycast / 边缘接入）
┌───────────────────────────────┐
│  GSLB 数据面（DNS Provider）    │  ← gslb-dns-provider 契约（ExternalDNS 参考实现 / 未来厂商）
└───────────────┬───────────────┘
                │ 写入由 Operation 执行（不再由控制器直写）
┌───────────────▼───────────────┐
│  gslb-controller（执行器）      │
│  ─ 消费 Operation 命令（DNS    │
│    Apply / Verify / Revert）   │
│  ─ HealthSource 聚合健康状态    │
│  ─ 投影 Read Model             │
└───────┬───────────────┬───────┘
        │ 领域事件        │ 投影
┌───────▼───────┐  ┌─────▼──────────┐
│ Operation     │  │ Read Model     │
│ Engine /      │  │（PostgreSQL）   │
│ ExecutionPlan │  └─────┬──────────┘
└───────┬───────┘        │ 只读查询
        │ intent 提交     ▼
┌───────▼───────────────────────────┐
│ 平台 API / Web Console（资源→GSLB） │
│  DRProtectionGroup（流量层步骤）     │
└───────────────────────────────────┘
```

写入链路：`用户/DR Group → Typed RuntimeIntent → ExecutionPlan（钉死池、权重、批准结果）→ Operation → gslb-controller 执行 DNS 变更 → 验证 → 投影`。
查询链路：`Web Console → Read Model 查询 API`，请求路径不实时探测、不实时查 DNS。

## Decisions

### Decision 1: 写路径接入 Operation Engine（P0，消除旁路）
`gslb-controller` 从"reconciler 直写 DNSEndpoint"改造为 Operation 执行器：
- 控制器监听平台命令（经 NATS 可靠投递），执行 ExecutionPlan 中 DNS 相关步骤（Apply/Verify/Revert），完成后上报结果；
- **自动故障转移也是受控的**：健康聚合判定 → 平台生成 `gslb.failover` Operation（默认 `require_approval`，可配置维护窗口自动批准）→ 控制器执行；
- 切换/回切/调权全部落 Operation 与审计（哈希链），杜绝静默改 DNS。
理由：白皮书 §3.2 明确"任何门户、Copilot、Provider 或 Controller MUST NOT 绕过该状态机直接改变 RuntimeTarget"；DNS 目标是多地域的入口 RuntimeTarget，同属约束范围。

### Decision 2: DNS 数据面 Provider 化
- 定义 `gslb-dns-provider` SPI：`ApplyRecords(records, weights, ttl) / Verify(targets) / DeleteRecords(set)`，返回 OperationRef；
- 内置参考实现：ExternalDNS `DNSEndpoint` CR（复用现有 `internal/dns/manager.go` 逻辑，改由 Operation 驱动）；
- 其余实现（Cloudflare/NS1/自研权威/Anycast）经 **gslb Conformance Harness** 认证后接入，内核零依赖（对齐 §4.2 第二 Provider 机制）；
- Provider 不得建立独立执行入口。
理由：与平台"网络/存储/网关均可替换"的既有承诺一致，避免 GSLB 绑定单一 DNS 厂商。

### Decision 3: CQRS + Read Model
- 控制器将健康状态、当前 DNS 目标、权重、最近切换投影到 `gslb_read_model`；
- 平台查询 API 只读 Read Model；跨地域场景由跨站点 Relay 同步投影快照；
- 查询时延不随成员规模线性增长（对齐 §3.5 硬约束）。
理由：DNS 探测有网络时延且不应在请求路径执行；投影天然支持多地域同步。

### Decision 4: 多租户模型
- `GSLBService` 归属 tenant（`tenant_id` 必填）；池/成员/健康检查/策略随服务租户隔离；
- 平台管理员可管理平台级全局入口；租户仅见本租户服务；
- 缓存键、Read Model 查询、Operation 授权均按 tenant 范围收敛（对齐 UI 规范 §15.4 / §16）。
理由：DNS 记录按租户视图生成（域内前缀或租户专属域），避免跨租户串流量。

### Decision 5: 与 DR Group 级联
- `DRProtectionGroup` 可包含流量层步骤：`gslb.failover` Operation 作为切换链的一段，顺序在数据层切换之后；
- 回切（switchback）显式人工确认，默认 `require_approval`；
- 故障演练（drill）不触发真实切换。
理由：白皮书 §8.1"级联而非二选一"——数据库 HA 解决实例故障，DR Group 解决机房/地域级故障，GSLB 是其流量层出口。

### Decision 6: 状态机与 Operation 对齐
GSLBService 生命周期状态全部由 Operation 驱动迁移（复用 Operation 10 态状态机的预检-执行-验证-回滚四段式），见 Data Model 节。

## Data Model

### 迁移 081: gslb_traffic_resilience

```sql
CREATE TABLE gslb_services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    domain TEXT NOT NULL,                    -- 全局入口域名
    routing_mode TEXT NOT NULL DEFAULT 'dns',-- dns | anycast(预留)
    active_pool_id UUID,
    lifecycle_state TEXT NOT NULL DEFAULT 'Inactive'
        CHECK (lifecycle_state IN ('Inactive','Provisioning','Active','Degraded',
                                   'FailingOver','Paused','Disabled','Terminated')),
    require_approval BOOLEAN NOT NULL DEFAULT true, -- 高风险切换默认审批
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);

CREATE TABLE gslb_pools (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES gslb_services(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,     -- 主/备池
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE gslb_pool_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pool_id UUID NOT NULL REFERENCES gslb_pools(id) ON DELETE CASCADE,
    cluster_id TEXT NOT NULL,
    weight INTEGER NOT NULL DEFAULT 100,
    enabled BOOLEAN NOT NULL DEFAULT true,
    healthy BOOLEAN NOT NULL DEFAULT false,
    last_health_at TIMESTAMPTZ,
    UNIQUE (pool_id, cluster_id)
);

CREATE TABLE gslb_health_checks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES gslb_services(id) ON DELETE CASCADE,
    probe_type TEXT NOT NULL CHECK (probe_type IN ('apiserver','http','karmada','manual')),
    interval_seconds INTEGER NOT NULL DEFAULT 30,
    timeout_seconds INTEGER NOT NULL DEFAULT 5,
    failure_threshold INTEGER NOT NULL DEFAULT 3,
    cool_down_seconds INTEGER NOT NULL DEFAULT 60,
    config JSONB NOT NULL DEFAULT '{}'
);

CREATE TABLE gslb_read_model (
    service_id UUID PRIMARY KEY REFERENCES gslb_services(id) ON DELETE CASCADE,
    tenant_id TEXT NOT NULL,
    domain TEXT NOT NULL,
    active_pool_id UUID,
    lifecycle_state TEXT NOT NULL,
    healthy_pools TEXT[] NOT NULL DEFAULT '{}',
    current_dns_targets TEXT[] NOT NULL DEFAULT '{}',
    last_switch_operation_id UUID,
    last_switch_at TIMESTAMPTZ,
    observed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### Outbox 事件（与写操作同事务）

- `hnb.event.gslb.health-changed.v1`（成员健康变化）
- `hnb.event.gslb.switched.v1`（切换/回切完成）
- `hnb.event.gslb.drill-completed.v1`（演练完成）
- `hnb.event.gslb.weight-updated.v1`（调权完成）

## API / Event Contracts

### RuntimeIntent（写，均携带 IdempotencyKey + CorrelationID）

| Intent | 语义 | 默认审批 |
|---|---|---|
| `gslb.failover` | 切换 active_pool（含自动故障转移生成） | true |
| `gslb.switchback` | 回切主池，需显式确认 | true |
| `gslb.weight-update` | 调整成员权重（灰度） | false |
| `gslb.drill` | 只读演练，产出报告，不改 DNS | false |

### 查询（Read Model，只读）

- `GET /api/v1/gslb/services`（租户列表）
- `GET /api/v1/gslb/services/{id}`（详情 + 健康 + 流量分布）
- `GET /api/v1/gslb/services/{id}/pools`
- `GET /api/v1/gslb/services/{id}/operations`（切换/演练历史）

### 契约落位

- `contracts/schema/gslb/v1/`：GSLBService、GSLBPool、GSLBPoolMember、HealthCheck、TrafficPolicy、FailoverPolicy、GSLBReadModel
- `contracts/openapi/console/v1/openapi.yaml`：GSLB 查询 + intent 提交端点（Schema First，生成 Go/TS SDK）
- `contracts/proto`：`hnb.event.gslb.*` 事件 envelope

## State Machine

```text
Inactive → Provisioning → Active ⇄ Reconfiguring / WeightUpdating
Active → Degraded（成员部分失健康，仍服务，触发告警）
Active/Degraded → FailingOver → Active（切换完成）
Active → Paused（人工冻结，流量不动）
Active → Disabled → Terminated
```
所有迁移由 Operation 驱动；`FailingOver` 期间新请求进入 Operation Center 跟踪，失败自动触发补偿（Revert 步骤）。

## Failure Modes

| 失败模式 | 处理 |
|---|---|
| 健康探测误报（假阴性） | 多源聚合 + failure_threshold 防抖 + cool_down；人工标记覆盖 |
| 探测假阳性（成员实际故障但未标记） | Karmada/HTTP 多源交叉验证，操作前 ExecutionPlan 预检 |
| DNS 传播延迟 | Verify 步骤：TTL 感知 + 权威查询确认目标生效后才置 Operation 完成 |
| 跨地域控制器分区 | 双 Region 控制器经 Lease/Fencing 防双写；控制面中断不阻塞已下发 DNS 目标（§8.1 控制面故障不影响数据面） |
| Provider 故障 | Provider 失败隔离 + 有限重试 + 补偿 Revert 到上一已知目标 |
| 审批超时 | Operation 保持 Queued/等待状态，超时升级告警（复用 Operation SLO） |

## Alternatives

1. **维持现状（控制器直写 DNSEndpoint）**：实现最快，但不可审计、不可审批、不可回滚，违反唯一写入口铁律。否决。
2. **引入外部 GSLB 产品（NS1/Cloudflare 全家桶）**：能力强但脱离平台 Operation/租户/审计模型，形成旁路。可作为 DNS Provider 实现接入（Decision 2），不作为平台内嵌方案。
3. **控制器内自建状态机**：重复 Operation Engine 的 10 态/幂等/补偿，违反微内核原则。否决。

## Assessment

| 维度 | 结论 |
|---|---|
| 租户隔离 | GSLBService 归属 tenant；查询/策略/DNS 视图按租户隔离（Decision 4）；跨租户访问拒绝 |
| Secret | DNS Provider 凭据仅使用 SecretReference，不落库明文、不进日志/事件 |
| 供应链 | Provider/参考实现经 Conformance + 镜像 digest 锁定；无新第三方二进制 |
| 权限 | `gslb:read/list/update/execute`；切换/回切 `require_approval` + 二次确认 |
| 审计 | 全部流量变更走 Operation（哈希链防篡改），记录操作人、审批单号、参数 diff |
| 性能预算 | 查询读 Read Model（P95 < 200ms）；探测间隔 ≥ 10s 且有防抖；控制面资源不突破 4 vCPU / 8 GiB |
| 容量 | 成员/池规模上限按 Conformance 定义；Read Model 行数 = 服务数（有界） |
| 升级 | 控制器新版本兼容旧 DNSEndpoint 记录集；首次接管以 Operation 对账 |
| 回滚 | 081 rollback 脚本 + 控制器旧版本回退；DNS 目标为可逆状态 |
| 灾备 | 四层容灾的流量层执行器；DR Group 级联编排；控制面中断不影响已下发目标 |
| 可观测 | 健康变化/切换/演练产出 Operation + 领域事件 + 告警；Read Model 提供平台视角状态 |

## Evidence Plan

- gslb-controller 单测（执行器步骤、健康聚合、投影）
- PG16 集成测试（迁移 081、Operation 驱动切换、租户隔离）
- gslb-dns-provider Conformance harness 测试（ExternalDNS 参考实现）
- E2E：资源 → GSLB 页面列表/详情/切换/演练流程
- `openspec validate --all --strict` + 契约门禁 + `validate-specs.sh`
