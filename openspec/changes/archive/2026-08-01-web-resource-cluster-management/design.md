## Overview

在 Web Console 资源插件落地集群管理：列表/详情走 L2 Schema 渲染，注册/创建向导与节点面板走 L3 注册组件，写动作统一经 RuntimeIntent 提交并跟踪 Operation。所有查询走 Read Model，遵守白皮书 §3.2/§3.5。

## Architecture

- 前端模块化：`web/plugins/resource/src/pages/cluster-management/` 按 `schemas/`、`components/`、`api/`、`types/` 分类组织，`index.ts` 注册路由、菜单、组件。
- 查询路径：UI 页面 → `@hnb/api-client` → apiserver Read Model 控制器 → PostgreSQL Read Model 投影。请求路径不实时遍历 RuntimeTarget。
- 写路径：UI 向导/操作 → `@hnb/api-client.submitRuntimeIntent()` → `POST /api/v1/runtime-intents` → ExecutionPlan → Operation → Provider。前端不直连 Provider / NATS。
- 状态字典：服务端统一下发 `resource.cluster.status`，前端引用 `dictionaryId`，不自定义状态色。

## Data Sources

- 集群/节点 Read Model：`runtime-target` 领域投影（KubernetesTarget / EdgeRuntimeTarget），带 `observedAt/lastKnownStateAt`（RT-005）。
- 状态字典：UI Registry / Read Model 字典接口。
- 权限：`cluster:view/update/delete/create` 快照，服务端独立校验。

## API / Event Contracts

新增 Read Model 只读接口：

```http
GET /api/v1/resources/clusters?pageSize&page&keyword&type&status
GET /api/v1/resources/clusters/{clusterId}
GET /api/v1/resources/clusters/{clusterId}/nodes?pageSize&page
GET /api/v1/dictionaries/resource.cluster.status
```

写动作复用既有契约：

```http
POST /api/v1/runtime-intents      # CreateKubernetesTarget / ImportRuntimeTarget / UpgradeRuntimeTarget / DeleteRuntimeTarget
```

统一响应信封遵循 UI 规范 V2.5 §4.1（`apiVersion/ui.hnb.io/v1`、`kind`、`metadata`、`spec`）。

## State Machines

- 集群状态：`REGISTERING → PROVISIONING → RUNNING ⇄ DEGRADED/STALE/SUSPENDED → DELETING → TERMINATED`，前端仅展示，状态迁移由平台 Operation 驱动。
- STALE（RT-005）：超过新鲜度阈值时写操作排队/拒绝或要求显式风险确认，UI 显示 `lastKnownStateAt` 并提示。

## Failure Modes

- 列表接口失败：区块级 ErrorState，保留其余内容，独立重试（V2.5 §4.4）。
- 向导提交失败：展示 ProblemDetails（RFC 9457），不产生运行时副作用（Planning 失败无副作用）。
- 未识别组件类型：仅影响对应区块，显示安全占位符。
- Schema 版本不兼容：拒绝渲染并提示升级，不自动降级执行。
- 租户切换迟到响应：按 generation 丢弃，不渲染旧上下文数据。

## Alternatives Considered

- 同步 REST 写接口：违背白皮书 §3.2（Operation 唯一写入口），拒绝。
- 纯代码页面不做 Schema：违背 V2.5 数据驱动优先原则，标准 CRUD 仍走 L2 Schema；仅在复杂交互（向导、节点拓扑）用 L3 组件。

## Security

- 菜单/按钮隐藏不构成安全边界；每个集群 API 独立校验权限与租户（V2.5 §16.1），服务端重复执行业务校验。
- kubeconfig / CloudCore 凭据仅使用 SecretReference，前端只显示引用信息，不进日志与埋点。
- 默认拒绝：权限/上下文/能力缺失时不渲染、不执行。
- Schema 只引用受信 `componentType` / `endpointId` / `actionId`，禁止任意 URL / 脚本 / Secret。
- 跨租户操作（解除纳管）显示源/目标租户并二次确认。

## Compatibility

- `/resource/clusters` 路径与菜单项保持不变；PageSchema 低版本兼容旧组件；apiVersion 支持当前与前一稳定版本。
- 新增字段保持向后兼容，删除字段或改变语义须升主版本。
- 集群状态字典由服务端统一治理，避免跨页面表达不一致。

## Observability

- 指标：接口加载/失败/耗时、动作执行次数、Operation 状态变化、字典缓存命中。
- 日志：包含租户、subject 哈希、版本向量与结果计数；不含 token、Secret、kubeconfig 明文。

## Non-Goals

- 不做集群内工作负载/存储/网络管理（属 `container` 域）。
- 不做联邦 Placement / DR Placement。
- 不在前端实现写路径或直连 Provider / NATS。
- 不新增第四级导航。

## Conformance

- 遵循既有 `portal-experience` 验收场景与 UI 规范 V2.5 §21 测试治理；新增 E2E 覆盖列表/详情/向导/权限收回/租户切换。
- 本 change 不涉及 Provider/RuntimeTarget 执行面契约变更，无需 Provider Conformance 矩阵。
