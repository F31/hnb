# Proposal: gslb-traffic-resilience

## Change ID

- **Change**: `gslb-traffic-resilience`
- **Created**: 2026-08-19
- **Tier**: T2（标准可选，多地域生产形态）
- **影响平面**: HNB Core（Operation Engine / Read Model / Provider Registry）、Gateway 平面（流量层）、Web Console
- **受影响 specs**: `gslb`（新增 GSLB-005~011）、`observability-dr`（DR 编排引用）、`composition-operation`（RuntimeIntent/ExecutionPlan 扩展引用）
- **依赖 change**: `operation-engine-core`、`gateway-engine`、`observability-dr`、`add-multi-cluster`、`identity-tenancy`
- **迁移影响**: 新增数据库迁移 `081_gslb_traffic_resilience`（含 rollback）；新增契约 `schema/gslb/v1` 与 console OpenAPI 扩展；gslb-controller 行为变更
- **回滚策略**: 控制器回退到直写 DNSEndpoint 旧版本 + `081` rollback 脚本；GSLB 页面回退 stub

## Why

当前 GSLB 仅有 T2 方向性规格（GSLB-001~004）与一个骨架控制器 `cmd/gslb-controller`，存在三类欠账：

1. **执行旁路**：控制器 reconciler 直接写 ExternalDNS `DNSEndpoint` CR，绕过 Operation Engine，违反"Release → ExecutionPlan → Operation 是唯一写入运行目标的路径"铁律；流量切换不可审计、不可审批、不可回滚。
2. **零契约**：`contracts/` 中没有 GSLB 契约，违反 Schema First 边界，前端与控制器各自为政。
3. **产品面空缺**：资源菜单下的 GSLB 页面是占位 stub；无租户模型、无 Read Model 查询、无容灾联动、无演练能力。

## What Changes

- **受控流量变更**：所有 GSLB 流量变更（故障转移、回切、调权、启停）经 Typed RuntimeIntent → ExecutionPlan → Operation 执行；自动故障转移由平台判定后以 Operation 落地；高风险切换默认 `require_approval`
- **DNS 数据面 Provider 化**：`gslb-dns-provider` 契约 + ExternalDNS DNSEndpoint 参考实现，其余实现经 Conformance Harness 认证
- **Schema First 契约**：`schema/gslb/v1`（GSLBService/GSLBPool/HealthCheck/TrafficPolicy/FailoverPolicy）+ console OpenAPI（Read Model 查询 + RuntimeIntent 提交）并生成 SDK
- **数据模型**：迁移 081 —— `gslb_services`、`gslb_pools`、`gslb_pool_members`、`gslb_health_checks`、`gslb_read_model`，多租户隔离
- **CQRS 查询**：健康状态、当前流量目标、切换历史投影到 Read Model；查询路径不实时探测、不实时查 DNS
- **容灾联动**：DRProtectionGroup 可编排"数据层切换 → 流量层切换"顺序，回切显式确认
- **故障演练**：只读演练模式，不产生真实流量变更，产出演练报告
- **Web Console**：资源 → GSLB 列表/详情/动作页（替换 stub），动作全部转 Operation 并接入 Operation Center

## Capabilities

### New Capabilities

无（`gslb` 能力域已存在，本 change 扩展其行为要求）

### Modified Capabilities

- `gslb`: 新增 GSLB-005~011（受控变更、Provider 化、Read Model、多租户、容灾联动、演练、健康源契约）
- `composition-operation`: 支持 gslb 类型 RuntimeIntent 与 ExecutionPlan 步骤（DNS Apply/Verify/Revert）
- `observability-dr`: DRProtectionGroup 编排 GSLB 流量层切换步骤

## Impact

- **现有服务改造**：`cmd/gslb-controller`（直写 → Operation 执行器 + 投影器）；`platform-api`/`apiserver`（GSLB 查询与 intent 提交 API）
- **新组件**：无新独立服务；DNS Provider 以独立进程/容器接入（复用 Provider 模式）
- **新数据库表**：081 迁移 5 张表，均为平台 PostgreSQL（复用现有基础设施，不引入新中间件）
- **T2 分级**：GSLB 为 T2 标准可选能力，未部署时内核不加载相关执行逻辑（对齐 Gateway/AI Extension Plane 的可独立启停约定）
- **兼容性**：控制器新版本仍可管理既有 DNSEndpoint 记录集（首次接管时以 Operation 对账）；DNS 数据面契约向后兼容
- **安全风险**：流量切换是高风险操作——默认审批、二次确认、审计留痕、只读演练；DNS Provider 凭据仅使用 SecretReference
- **资源预算**：健康探测与投影追加量可控（每服务每探测周期一次请求）；控制面资源预算不突破 4 vCPU / 8 GiB
- **可观测要求**：健康状态变化、切换/回切/演练均产出 Operation + 领域事件（`hnb.event.gslb.*`）与告警
- **退出判据**：GSLB-005~011 全部通过验收场景；gslb-controller 不再存在直写 DNSEndpoint 的代码路径；契约门禁与 OpenSpec 门禁通过；PG16 迁移验证通过

## Non-Goals

- 不做 DNS 记录级编辑器（用户只操作 GSLBService 策略，不直接编辑 DNS 记录）
- 不替代 Ingress/Gateway 配置（那是 `gateway` 能力域，Gateway API Standard Channel）
- 不实现应用层负载均衡（K8s Service/LoadBalancer 职责）
- 不引入新中间件（复用 PostgreSQL + NATS + 现有 Provider 机制）
- 单地域（T1）形态不强制部署 GSLB；本 change 不改变 T1 基线
