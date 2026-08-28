# HNB Web Console 资源菜单 · 集群管理功能设计

> 版本：Draft V0.1　|　日期：2026-08-01　|　密级：内部
>
> 依据：`docs/HNB_技术白皮书.md`、`docs/UI设计规范_V2.5.md`、`openspec/architecture.md`、`openspec/specs/runtime-target/spec.md`。

## 1. 背景与目标

### 1.1 现状

`web/plugins/resource` 插件下「集群管理」目前只有占位页面（`ClusterList.vue` 仅渲染标题）。平台既有 RuntimeTarget 模型（`kubernetes-runtime-provider` / `runtime-target` 规格）定义了 KubernetesTarget、EdgeRuntimeTarget 等运行目标，但 Web Console 尚未消费。

### 1.2 目标

在资源菜单「集群」下提供统一集群列表、集群详情、注册/创建向导、升级/解除纳管动作、节点只读视图，遵循：

- **UI 规范 V2.5 四级扩展模型**：优先 L2 Schema 页面，复杂交互（注册向导、拓扑）使用 L3 注册组件；
- **白皮书 §3.2 Operation 唯一写入口**：所有写动作经 `RuntimeIntent` 提交，前端不直连 Provider / RuntimeTarget；
- **CQRS + Read Model 硬约束**：列表/详情/节点只读查询走 Read Model 接口，请求路径不实时遍历目标；
- **Schema First**：`contracts/` 新增 OpenAPI 与 JSON Schema，前端类型由生成 SDK 派生。

### 1.3 非目标

- 不做集群内工作负载、存储、网络管理（属于 `container` 插件域）；
- 不做联邦 Placement 与 DR Placement（`multi-cluster` / `observability-dr` 域）；
- 不在前端实现任何写路径（禁绕过 Operation）；
- 不新增第四级导航。

## 2. 页面范围与导航

### 2.1 导航

资源菜单「集群」为叶子路由，指向集群列表页（二级导航，符合 V2.5 §6.3 最多三级约束）。

| 菜单 | 路径 | 页面 | 模板 |
|---|---|---|---|
| 集群 | `/resource/clusters` | 集群列表 | `list` |
| 集群详情 | `/resource/clusters/:clusterId` | 集群详情 | `detail` |

### 2.2 页面目录组织（按功能模块分类）

遵循 `service` / `container` 插件按业务子域划分子目录的既有惯例，`resource` 插件新增 `cluster-management` 功能模块目录：

```text
web/plugins/resource/src/
├── index.ts                                  # 插件入口：注册路由/菜单/组件
├── locales/
│   └── index.ts                              # 资源插件语言包（含 cluster.* 命名空间）
└── pages/
    ├── ResourceLayout.vue
    ├── NodeList.vue                          # 既有：节点列表（占位）
    ├── GPUResources.vue / Network.vue / Storage.vue / GSLB.vue
    └── cluster-management/                   # 集群管理功能模块（新增）
        ├── ClusterList.vue                   # 列表页
        ├── ClusterDetail.vue                 # 详情页
        ├── ClusterRegisterWizard.vue         # 注册/创建向导（L3 注册组件承载）
        ├── components/
        │   ├── ClusterStatusBadge.vue        # 状态字典徽章
        │   ├── ClusterSummaryCards.vue       # 概览指标卡组
        │   └── ClusterNodesPanel.vue         # 节点只读面板（含空/错误/过期态）
        ├── schemas/
        │   ├── cluster.list.schema.ts        # PageSchema + TableSchema（L2）
        │   └── cluster.detail.schema.ts      # 详情 PageSchema
        ├── api/
        │   └── cluster.ts                    # Read Model 查询 + RuntimeIntent 提交封装
        └── types/
            └── cluster.ts                    # 集群领域类型（自生成 SDK 派生）
```

设计要点：

- 标准 CRUD/列表/详情走 **L2 Schema**（`schemas/`），PageRenderer 消费；
- 注册向导与节点面板因含专用交互与拓扑，走 **L3 注册组件**（在 `index.ts` 的 `components` 注册，命名空间 `resource.ClusterRegisterWizard`）；
- 所有接口调用经统一 `@hnb/api-client`，无直接 `fetch`；
- 目录内细分子域：`components`（展示单元）、`schemas`（Schema 定义）、`api`（数据访问）、`types`（类型）。

## 3. 领域模型与 Read Model

### 3.1 集群视图（Read Model 投影）

`ClusterSummary`：列表与详情页展示的统一投影，只读，来自 Read Model。

```typescript
interface ClusterSummary {
  clusterId: string
  displayName: string
  type: 'kubernetes' | 'edge' | 'container-engine'   // 统一表格展示，字段区分创建/纳管
  source: 'created' | 'imported'                      // 创建 or 纳管
  status: ClusterStatus                              // 状态字典，统一语义色
  runtimeVersion: string                              // k8s / 容器运行时版本
  nodeCount: number                                   // 汇总，详情页展开
  cpuTotal: string
  memoryTotal: string
  capabilitySnapshot: {
    snapshotVersion: number
    observedAt: string
    freshness: 'fresh' | 'stale'                     // 对应 RT-005 新鲜度
  }
  tenantId: string
  environmentId?: string
  createdAt: string
  updatedAt: string
}

type ClusterStatus =
  | 'REGISTERING'   // 注册中
  | 'PROVISIONING'  // 创建中
  | 'RUNNING'       // 运行中
  | 'DEGRADED'      // 降级
  | 'STALE'         // 状态过期（RT-005）
  | 'SUSPENDED'
  | 'DELETING'
  | 'TERMINATED'
```

### 3.2 状态字典

状态语义与字典由服务端统一下发（V2.5 §11.3），前端不得自定义状态色。前端引用 `dictionaryId: resource.cluster.status`：

```json
{
  "code": "RUNNING",
  "labelKey": "resource.cluster.status.running",
  "semantic": "success",
  "terminal": false
}
```

状态 → 语义映射：

| 状态 | semantic | 说明 |
|---|---|---|
| REGISTERING / PROVISIONING | info | 进行中，操作按钮 loading |
| RUNNING | success | 正常 |
| DEGRADED | warning | 部分异常 |
| STALE | warning | 状态过期，写操作需风险确认（RT-005） |
| SUSPENDED / TERMINATED | default | 已停用/已终止 |

### 3.3 节点视图（只读）

`ClusterNodeInfo`：集群详情内节点只读面板数据，来源为 Agent 上报 / CloudCore 代理（RT-006/RT-007），前端不直接连接边缘节点。

```typescript
interface ClusterNodeInfo {
  nodeId: string
  name: string
  role: 'control-plane' | 'worker' | 'edge'
  status: 'Ready' | 'NotReady' | 'Unknown'
  ipAddress?: string
  os: string
  arch: string
  cpuAllocatable: string
  memoryAllocatable: string
  kubeletVersion: string
  lastHeartbeatAt: string
}
```

## 4. API 契约（Schema First）

### 4.1 查询（Read Model，只读）

| Method | Path | 用途 | 分页 |
|---|---|---|---|
| GET | `/api/v1/resources/clusters` | 集群列表（Read Model） | `pageSize/page/keyword/type/status` |
| GET | `/api/v1/resources/clusters/{clusterId}` | 集群详情（Read Model） | - |
| GET | `/api/v1/resources/clusters/{clusterId}/nodes` | 节点只读列表 | `pageSize/page` |
| GET | `/api/v1/dictionaries/cluster.status` | 集群状态字典 | - |

统一响应信封（V2.5 §4.1）：

```json
{
  "apiVersion": "ui.hnb.io/v1",
  "kind": "ClusterSummaryList",
  "metadata": { "id": "resource.cluster.list", "revision": 1 },
  "spec": {
    "items": [],
    "total": 0
  }
}
```

### 4.2 写（RuntimeIntent，唯一写入口）

所有写动作经 `POST /api/v1/runtime-intents` 提交 Typed RuntimeIntent，不新增同步写 REST 接口：

| 动作 | RuntimeIntent kind | 关键字段 |
|---|---|---|
| 创建集群 | `CreateKubernetesTarget` | targetProfile、节点/容量参数、租户上下文 |
| 纳管集群 | `ImportRuntimeTarget` | targetType、kubeconfig SecretReference 或 CloudCore endpoint、nodeGroup |
| 升级集群 | `UpgradeRuntimeTarget` | targetVersion |
| 解除纳管 | `DeleteRuntimeTarget` | targetUid（服务端 fencing token 前置条件） |

前端统一走 `@hnb/api-client` 的 `submitRuntimeIntent()`，携带 `Idempotency-Key`、`X-Correlation-ID`，响应 `RuntimeIntentRecord`（`status: received/validated/planned/operationCommitted/rejected`）。进度通过 `operationStatus` DataSource 轮询 / SSE 跟踪，进入 Operation Center。

**后端实现说明（已完成）**：

- apiserver BFF `POST /api/v1/runtime-intents`：当 `platform-api` 已配置时**转发**到平台 `/v1/intents`（Operation Engine 唯一写入口）并映射 `RuntimeIntentRecord`；独立/开发模式（无 platform-api）持久化到 `bff_runtime_intents` 表并创建队列 Operation（`delete` 类进入 `pending_approval`）。此表与平台 025 迁移的权威 `runtime_intents` 表分离，不构成执行旁路。
- 权限：写入口路由鉴权为 `cluster:execute`；列表/详情/节点/字典为 `cluster:list` / `cluster:read`。前端按钮级 `cluster:create/update/delete` 仅作 UX 提示，服务端始终独立鉴权。
- 状态映射：内部 `online/degraded/decommissioned/offline/unknown` → 外部 `RUNNING/DEGRADED/TERMINATED/STALE`；超过 `stale_threshold_seconds` 一律投影为 `STALE`（RT-005）。
- 契约演进：`runtime-intent.schema.json` kind 枚举新增 4 个集群 kind，`spec.releaseId` 改为可选（仅 Release 类必填）；platform-api 引擎/planner/store 同步支持，`delete` 类 operation 进入高风险的 `pending_approval`。

## 5. UI Schema

### 5.1 列表页 PageSchema（L2）

`schemas/cluster.list.schema.ts` 输出 `PageSchema`，`template: list`：

```json
{
  "apiVersion": "ui.hnb.io/v1",
  "kind": "PageSchema",
  "metadata": { "id": "resource.cluster.list", "revision": 1, "pluginId": "resource" },
  "spec": {
    "template": "list",
    "titleKey": "resource.cluster.list.title",
    "contextRequirements": ["tenantId"],
    "layout": { "type": "grid", "columns": 12, "gap": "md" },
    "endpoints": [
      { "id": "resource.clusters.list", "path": "/api/v1/resources/clusters" },
      { "id": "resource.cluster.dictionary", "path": "/api/v1/dictionaries/cluster.status" }
    ],
    "dataSources": [
      { "id": "resource.cluster.list", "type": "paginatedQuery",
        "endpointId": "resource.clusters.list", "queryBindings": ["page", "pageSize", "keyword", "type", "status"],
        "responseMapping": { "items": "spec.items", "total": "spec.total" } },
      { "id": "resource.cluster.status.dictionary", "type": "dictionary", "endpointId": "resource.cluster.dictionary" }
    ],
    "actions": [
      {
        "id": "resource.cluster.openRegister",
        "type": "openDrawer", "labelKey": "resource.cluster.action.register",
        "permission": "cluster:create",
        "result": { "mode": "silent" }
      }
    ],
    "regions": [
      { "id": "summary", "componentType": "resource.ClusterSummaryCards", "span": 12, "props": { "dataSource": "resource.cluster.list" } },
      { "id": "table", "componentType": "DataTable", "span": 12, "props": { "schemaId": "resource.cluster.table" } }
    ]
  }
}
```

`TableSchema`（V2.5 §10）关键列：名称（ResourceLink → 详情）、类型、来源、状态（StatusBadge + 字典）、版本、节点数、更新时间。行操作 ≤3：查看（navigate）、升级（operation）、解除纳管（operation，危险色 + 二次确认）。批量操作：批量解除纳管（展示影响范围）。

### 5.2 详情页 PageSchema（L2）

`template: detail`，结构（V2.5 §11.2）：

```text
标题 + 状态 + 主操作（升级/解除纳管）
概览字段（DescriptionList）
Tabs
├── 运行状态（节点面板 resource.ClusterNodesPanel）
├── 配置（能力快照、RuntimeVersion、freshness）
├── 关联资源
├── 事件
└── 扩展点 resource.cluster.detail.tabs
```

声明扩展点：

```json
{
  "extensionPoint": "resource.cluster.detail.tabs",
  "accepts": ["tab", "panel"],
  "maxContributions": 10
}
```

### 5.3 注册/创建向导（L3 注册组件）

`ClusterRegisterWizard.vue` 注册为 `resource.ClusterRegisterWizard`（命名空间符合 V2.5 §22.2），`propsSchema` 通过 JSON Schema 校验。分两步：

1. **来源选择**：创建（KubernetesTarget：releaseId + targetProfile + 资源套餐）或纳管（kubeconfig SecretReference / CloudCore endpoint + 节点组映射，Edge 走 RT-006）；
2. **确认提交**：展示 RuntimeIntent 摘要与前置检查，提交到 `/api/v1/runtime-intents`。

向导为异步写，提交后 `result.mode = trackOperation`，进入 Operation Center，可离开页面继续执行（V2.5 §18.2）。

### 5.4 Action 定义

| Action ID | 类型 | 权限 | enabledWhen | confirm | result |
|---|---|---|---|---|---|
| `resource.cluster.openRegister` | openDrawer | `cluster:create` | - | - | silent |
| `resource.cluster.view` | navigate | `cluster:view` | - | - | silent |
| `resource.cluster.upgrade` | operation | `cluster:update` | resourceState `[RUNNING, DEGRADED]` | 高风险确认 + 维护窗口 | trackOperation |
| `resource.cluster.delete` | operation | `cluster:delete` | resourceState `[RUNNING, DEGRADED, SUSPENDED]` | 危险二次确认 | trackOperation |

`upgrade` / `delete` 的 `request` 只引用受信 `endpointId: runtime-intents.submit`，禁止下发任意 URL（V2.5 §12.3）。STALE 状态下写操作须服务端风险确认（RT-005）。

## 6. 状态机与交互状态

- 页面必备状态（V2.5 §18.1）：初始化、加载（骨架屏）、正常、空数据、部分失败、完全失败、无权限、不兼容、离线。
- 集群列表空态：提供「注册/创建集群」主操作与引导文案。
- STALE 集群：状态徽章 warning + 节点面板显示 `lastKnownStateAt` 与过期提示，写操作按策略排队或要求风险确认。
- 异步写：按钮 loading 防重复提交，提交后导航到 Operation Center 跟踪。

## 7. 安全、多租户与可观测

- **安全边界**：菜单/按钮隐藏不构成安全边界，每个 API 独立校验权限与租户（V2.5 §16.1）；服务端重复执行全部业务校验（V2.5 §9.3）。
- **默认拒绝**：缺权限/上下文/能力时不渲染，不默认放行（V2.5 §16.2）。
- **租户隔离**：Schema 快照、状态、缓存键绑定 `tenantId/spaceId/environmentId`，切换上下文按 generation 丢弃迟到响应（V2.5 §16.4）；跨租户操作二次确认。
- **敏感数据**：kubeconfig / CloudCore 凭据仅 SecretReference，前端只显示引用信息，不回显明文、不进日志（V2.5 §9.5）。
- **数据源约束**：列表/详情/节点全部走 Read Model 查询，禁止请求路径实时遍历 RuntimeTarget；实时性经 SSE/WS 或 operationStatus 跟踪，前端不直连 NATS。
- **可观测**：接口调用埋点（加载/失败/耗时）、动作执行、Operation 状态变化上报；日志不含 token 与 Secret。

## 8. 性能预算

| 指标 | 目标 |
|---|---|
| 集群列表 P95 | 缓存命中 ≤100ms，回源 ≤500ms |
| 列表页数据 | 服务端分页，默认 pageSize 20，>500 行启用虚拟滚动 |
| Schema 大小 | 单 Schema ≤200KB |
| 首屏 | 集群列表页 ≤2.5s（内网标准环境） |

## 9. 与既有实现的映射

| 本设计条目 | 对应代码/规格 |
|---|---|
| 集群状态字典 | 新增 `contracts/schema` + Read Model，字典接口 |
| RuntimeIntent 写入口 | 复用 `POST /api/v1/runtime-intents`（`platform/v1/openapi.yaml`） |
| 查询接口 | 新增 Read Model 控制器（Go），投影自 runtime-target 领域 |
| 页面/组件注册 | `web/plugins/resource/src/index.ts` 注册 routes + menuItems + components |
| 统一 API Client | `@hnb/api-client` |

## 10. 依赖 change 与兼容

- 依赖：`runtime-target-engine`（RuntimeTarget 模型）、`complete-console-bff-navigation`（导航 BFF）、`wire-schema-runtime-e2e`（Schema 渲染链路）。
- 兼容：既有 `/resource/clusters` 路径与菜单保持不变；本次为占位页面替换为真实实现，不破坏既有路由。
- 回滚：占位页面保留在 git 历史，功能灰度开关（`VITE_FEATURE_RESOURCE_CLUSTER_MGMT`，默认 `true`）控制展示；开关为 `false` 时插件入口降级为占位页面，路由 `/resource/clusters` 仍可访问但仅渲染空态文案，所有写动作按钮隐藏。Schema 低版本兼容旧组件。

### 10.1 灰度开关与回滚验证

| 维度 | 开关 `true`（默认） | 开关 `false` |
|---|---|---|
| 集群列表 | 真实列表（带状态字典/分页/写动作） | 占位页面（"功能未开启" 文案） |
| 集群详情 | 详情概览 + 节点面板 + Tabs | 路由仍可达，但渲染占位 |
| 注册向导 | 可用 | 不渲染 |
| 后端 BFF `/api/v1/resources/clusters` 等 | 正常工作（服务端权限独立校验） | 不被前端调用 |

- **关闭方式**：构建时设置 `VITE_FEATURE_RESOURCE_CLUSTER_MGMT=false`，或运行时通过 shell 环境变量注入。
- **回滚脚本**：`scripts/rollback-cluster-mgmt.sh` 验证开关关闭后页面占位、路由不破坏、既有 `/resource/clusters` 返回 200。
- **服务端不受影响**：关闭前端灰度不影响平台-api / apiserver BFF 的可用性，只关闭前端 UI 入口；服务端权限校验与租户隔离始终独立运行。

## 11. 打开问题

1. 创建 KubernetesTarget 所需 `releaseId/targetProfile` 的 Read Model 数据源是否在 MVP 就绪，或先仅开放「纳管」；
2. 集群升级的维护窗口字段是否由 ApprovalPolicy 校验；
3. 节点面板是否需要拓扑可视化（L3 组件 `resource.ClusterTopology`），或先表格化。
