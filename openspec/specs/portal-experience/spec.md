# portal-experience

## Purpose
定义 Portal 基于已安装能力、用户权限和目标能力动态呈现的简单、标准、专家三层体验，以及可恢复场景向导行为。
## Requirements
### Requirement: [UX-001] 能力驱动界面
Portal SHALL 根据已安装 CapabilityPack、用户权限和 RuntimeTarget 能力动态显示菜单、表单和动作；未安装能力 SHALL 不出现空菜单或不可执行入口。

**Traceability:** METH-03

#### Scenario: 未安装 Edge Pack
- **GIVEN** 用户登录平台
- **WHEN** Portal 构建导航
- **THEN** 不显示边缘菜单和向导
- **AND** 核心页面无 Edge 依赖错误

### Requirement: [UX-002] 三层操作模式
Portal SHALL 提供简单、标准和专家模式；简单模式面向业务对象，标准模式暴露策略和生命周期，专家模式才允许查看底层资源。

**Traceability:** METH-04

#### Scenario: 新用户交付应用
- **GIVEN** 用户使用简单模式
- **WHEN** 完成发布和部署
- **THEN** 无需直接编辑 Kubernetes CRD
- **AND** 专家模式仍受权限和策略约束

### Requirement: [UX-003] 场景化向导
平台 SHALL 为应用发布、数据库创建、服务暴露、备份恢复和 Gateway 迁移提供可恢复向导；安装对应 CapabilityPack 后，平台 SHALL 为边缘节点纳管和 AI 接入提供可恢复向导；所有产生运行变更的向导 SHALL 在提交前展示 ExecutionPlan 摘要。

**Traceability:** GW-14, GOV-04

#### Scenario: 向导中途退出
- **GIVEN** 用户已填写部分部署参数
- **WHEN** 稍后重新进入
- **THEN** 草稿被恢复且未产生目标资源
- **AND** 最终提交生成 Operation

### Requirement: [UX-004] Web 告警中心与实时通知
Portal SHALL 提供租户隔离的告警中心、通知铃铛和未读计数，并通过鉴权后的 Platform API SSE/WebSocket 或 Read Model 实时更新；用户 SHALL 可按权限筛选、查看、确认、认领、静默和解除静默告警，并跳转到关联资源、Operation、日志、指标、链路和 Runbook。Portal SHALL NOT 直接连接内部消息系统或外部通知 Provider。

**Traceability:** ALERT-001, ALERT-002, ALERT-004, ALERT-009, TENANT-002

#### Scenario: 租户收到实时告警
- **GIVEN** 租户 A 的授权用户已登录 Portal
- **WHEN** 租户 A 产生一个新 Firing 告警
- **THEN** 通知铃铛和未读计数实时更新且告警出现在告警中心
- **AND** 租户 B 用户无法读取该告警或通知正文

#### Scenario: 只读用户尝试静默告警
- **GIVEN** 一个用户只有告警只读权限
- **WHEN** 其尝试静默或确认告警
- **THEN** Portal 隐藏或禁用不可执行动作且 API 拒绝越权请求
- **AND** 告警状态保持不变并记录越权尝试

### Requirement: [UX-005] 租户、角色和审批策略管理界面
Portal SHALL 提供租户管理、项目/环境管理、Namespace 管理、角色管理、用户角色分配、审批策略配置和 SecretReference 管理界面；界面 SHALL 按登录用户角色显示可用操作，平台管理员可管理所有租户，租户管理员仅管理所属租户。

**Traceability:** TENANT-005, TENANT-006, TENANT-007, TENANT-008

#### Scenario: 租户管理员查看用户角色
- **GIVEN** 租户管理员登录 Portal
- **WHEN** 其导航到用户管理页面
- **THEN** 显示所属租户下的用户列表和角色分配
- **AND** 无法看到其他租户的用户或角色

#### Scenario: 平台管理员创建审批策略
- **GIVEN** 平台管理员登录 Portal
- **WHEN** 其创建审批策略并绑定到 `database-failover` 操作类型
- **THEN** 策略保存后对该操作类型生效
- **AND** 租户管理员可在租户范围内查看该策略

#### Scenario: 项目管理员创建 Namespace
- **GIVEN** 项目管理员登录 Portal
- **WHEN** 其进入 Project A 的 Production 环境，创建一个 Namespace 并指定 suffix 为 `api`
- **THEN** 系统自动生成 Namespace 名称为 `{tenant}-{project}-production-api`
- **AND** Namespace 显示在环境的 Namespace 列表中

#### Scenario: 多 Namespace 环境视图
- **GIVEN** Production 环境包含 api、worker、cache 三个 Namespace
- **WHEN** 项目管理员查看 Production 环境详情
- **THEN** 显示三个 Namespace 及其状态、标签信息
- **AND** 可分别管理每个 Namespace

### Requirement: [P1-CONSOLE-001] Authenticated Console Bootstrap
The Web Console SHALL obtain the verified subject, selected tenant,
memberships, deployment capabilities, scoped permissions, and version metadata
from an authenticated server bootstrap contract. It SHALL NOT infer authority
from plugin manifests, locally stored roles, identity headers, or route names.

**Traceability:** UX-001, TENANT-005, TENANT-007, P0-BASE-005

#### Scenario: Plugin declares a permission the user lacks
- **GIVEN** a plugin manifest declares a required runtime-write permission
- **WHEN** bootstrap does not grant that permission to the selected tenant
- **THEN** the Console does not expose the route or action and the server would independently deny a direct request

### Requirement: [P1-CONSOLE-002] Capability and Permission Intersection
Console plugin activation, navigation, routes, and actions SHALL be enabled
only by the intersection of installed capability availability and the
subject's scoped permission. Missing, stale, or failed bootstrap data SHALL
fail closed for protected features.

**Traceability:** UX-001, UX-002, TENANT-007, P0-BASE-005

#### Scenario: Backend capability becomes unavailable
- **GIVEN** a subject retains permission for a plugin action but the deployment capability is disabled
- **WHEN** the Console refreshes bootstrap
- **THEN** the plugin action is removed or disabled and cannot be invoked through the shared API client

### Requirement: [P1-CONSOLE-003] Session and Tenant Transition Safety
The Web Console SHALL clear tenant-scoped caches, pending privileged state, and
plugin-derived data on logout, token expiry, subject disable, tenant switch, or
permission version change. Every API call SHALL use the shared authenticated
client and correlation contract.

**Traceability:** TENANT-005, UX-005, P0-BASE-005

#### Scenario: User switches tenants
- **GIVEN** the Console has cached tenant-A operations and permissions
- **WHEN** the user switches to tenant B
- **THEN** tenant-A data and authority are cleared before tenant-B routes or requests are enabled

### Requirement: [UX-006] Apiserver-owned final navigation view
The platform SHALL expose the final browser-facing Console navigation view only through apiserver `GET /api/v1/navigation/menus`. The response SHALL be computed from authenticated subject, tenant/space context, scoped permissions, database-backed plugin/menu/route metadata, capability availability, feature/license state, locale, and stored ordering metadata. The Web Console SHALL NOT hardcode final menu order, generate menus from plugin manifests, or call platform-api for final user menus.

**Traceability:** UX-001, P1-CONSOLE-001, P1-CONSOLE-002, CONTRACT-001

#### Scenario: User lacks a route permission
- **GIVEN** an installed plugin registers a route requiring `cluster:update`
- **WHEN** a user without that scoped permission requests `/api/v1/navigation/menus`
- **THEN** apiserver omits that route and menu item from the returned `NavigationResponse`
- **AND** a direct request to the protected backend route remains independently denied

#### Scenario: Browser attempts to call platform-api menu endpoint
- **GIVEN** the Web Console is configured with the apiserver base URL
- **WHEN** navigation is loaded
- **THEN** the browser requests `/api/v1/navigation/menus` from apiserver
- **AND** platform-api does not expose a public final menu endpoint for browser use

#### Scenario: Navigation order comes from metadata
- **GIVEN** database-backed navigation metadata stores first-level menu items with `sort_order`
- **WHEN** an authenticated subject requests `/api/v1/navigation/menus`
- **THEN** apiserver returns menus ordered by `sort_order`
- **AND** Web renders the returned order without applying a hardcoded order list

#### Scenario: Home has no sidebar from data shape
- **GIVEN** the returned first-level home navigation item is a leaf route or has no visible children
- **WHEN** Web renders the Console layout on that route
- **THEN** the top navigation shows the home item
- **AND** the side navigation is not shown because the returned navigation item has no children

### Requirement: [UX-007] Navigation cache invalidation and tenant safety
The navigation service SHALL scope ETag, L1 cache and optional distributed cache entries to subject, tenant, space, locale and version vector. A change to permission version, plugin catalog version, navigation version, license/feature version or tenant SHALL invalidate stale navigation and fail closed for protected routes.

**Traceability:** UX-001, P1-CONSOLE-003, TENANT-005, CONTRACT-003

#### Scenario: Permission version changes
- **GIVEN** a user has a cached navigation response with permission version `p1`
- **WHEN** their role binding changes to permission version `p2`
- **THEN** the next navigation request is recomputed or returns a fresh ETag for `p2`
- **AND** routes no longer granted by `p2` are absent

#### Scenario: Tenant switch with persisted LKG
- **GIVEN** the browser has a last-known-good navigation snapshot for tenant A
- **WHEN** the user switches to tenant B
- **THEN** apiserver returns tenant B navigation only
- **AND** the tenant A snapshot cannot authorize or display tenant B protected routes

### Requirement: [UX-008] Executable Schema-Driven Console Pages
The Web Console SHALL render authenticated schema pages from apiserver and SHALL support declared data source queries and actions through trusted Shell runtime services. Schema pages SHALL NOT execute arbitrary JavaScript, arbitrary URLs, unfiltered HTML, Secret values, or target credentials supplied by schema content.

**Traceability:** UX-001, UX-006, CONTRACT-001, CONTRACT-008

#### Scenario: Schema page loads and queries data
- **GIVEN** an authenticated user has access to a schema route returned by Console navigation
- **WHEN** the user opens the route
- **THEN** the Shell fetches the page schema from apiserver
- **AND** trusted components query data only through registered dataSource endpoint IDs and the shared authenticated API client

#### Scenario: Schema action invokes a backend operation
- **GIVEN** a schema page contains a declared action with a trusted endpoint ID and required permission
- **WHEN** the user triggers the action and the permission check passes
- **THEN** the Shell invokes the endpoint through the shared authenticated API client
- **AND** the result is reported as a safe text notification or operation tracking state

### Requirement: [UX-009] Controlled Schema Visibility and Action Gating
The Web Console SHALL evaluate schema region conditions and action enabled conditions using only controlled Shell context: scoped permissions, capabilities, feature/license state, tenant/space context, and safe record state. Hidden or disabled schema elements SHALL fail closed when required context is missing or stale.

**Traceability:** UX-001, P1-CONSOLE-002, P1-CONSOLE-003

#### Scenario: Region requires missing permission
- **GIVEN** a schema region declares a permission condition
- **WHEN** the authenticated subject lacks that permission in the selected tenant
- **THEN** the Shell does not render the protected region
- **AND** direct backend requests remain independently denied by server authorization

#### Scenario: Action condition becomes false
- **GIVEN** a rendered schema action depends on a capability and scoped permission
- **WHEN** either capability or permission state is unavailable or false
- **THEN** the action is disabled or hidden and cannot call the backend endpoint

### Requirement: [UX-011] Shared UI component baseline
The Web Console SHALL provide reusable UI Kit components for standard pages, toolbars, buttons, tables, forms, select/date inputs, status, detail panels, empty/error/loading states, and action bars. Shell, Schema Renderer, and plugins SHALL prefer these components over local duplicate styling for standard CRUD, list, detail, and dashboard pages.

**Traceability:** UX-008, CONTRACT-009

#### Scenario: Schema page uses registered UI Kit components
- **GIVEN** a PageSchema references a standard registered component type such as `DataTable`, `MetricCard`, or `DescriptionList`
- **WHEN** the Schema Renderer resolves the component
- **THEN** it uses the `@hnb/ui-kit` registered component
- **AND** unknown component types are isolated to a component-level error placeholder

#### Scenario: Plugin page reuses UI Kit primitives
- **GIVEN** a plugin implements a standard list page
- **WHEN** it renders table, toolbar, button, empty, loading, and error states
- **THEN** it uses `@hnb/ui-kit` primitives and HNB design tokens instead of local hardcoded colors and duplicate table wrappers

### Requirement: [UX-010] Schema Incompatibility User Experience
When a schema page requires a Shell version newer than the running Shell, the Web Console SHALL show an explicit user-facing incompatibility message and SHALL NOT render partial or unsafe page content.

**Traceability:** UX-002, CONTRACT-002

#### Scenario: Schema requires newer Shell
- **GIVEN** apiserver returns a schema whose `minShellVersion` is higher than the running Shell version
- **WHEN** the Shell validates the schema
- **THEN** the user sees a clear upgrade-required message
- **AND** no schema regions or actions from that page are executed

### Requirement: [UX-021] Schema 驱动的集群列表与详情
Web Console SHALL 提供资源菜单下 Schema 驱动的集群列表与详情页面，列表 SHALL 通过服务端分页 Read Model 接口加载，状态字段 SHALL 使用服务端统一下发的集群状态字典 `resource.cluster.status`，详情 SHALL 展示概览、能力快照、节点只读面板与声明式扩展点。集群页面及其服务端查询 SHALL 仅包含 `KubernetesTarget` 与 `EdgeRuntimeTarget`，不得包含 `ContainerEngineTarget`、工作负载或联邦成员资源。列表与详情 SHALL 获取并实际执行 apiserver 下发的 L2 `PageSchema`，通过 Schema Renderer、注册的 `dataSource`/`endpointId` 和共享鉴权客户端加载数据，不得以本地硬编码表格或详情页面冒充 Schema 驱动实现。Web Console SHALL 从服务端获取并消费 `resource.cluster.status` 字典的值、标签与语义色，不得维护会覆盖服务端定义的本地状态映射。列表 SHALL 使用服务端过滤、排序、精确总数和游标或页码分页；列表请求 SHALL 要求 `cluster:list`，详情请求及 `resource.cluster.detail.tabs` 扩展内容 SHALL 要求 `cluster:read`，服务端 SHALL 对每次请求独立授权。

列表和详情 SHALL 使用请求所属的 tenant、请求序列或取消信号隔离异步响应；租户切换 SHALL 先清除集群 Schema、字典、列表、详情和权限缓存，旧租户的迟到响应 SHALL 被丢弃且不得覆盖新租户界面。页面 SHALL 分别呈现完整的 loading、error、empty、no-permission、offline 和 schema-incompatible 状态；offline 状态可展示明确标记且不授予动作的同租户最后已知数据，schema-incompatible 状态 SHALL 停止渲染页面区域和动作，不得回退到不受 Schema 约束的实现。

**Traceability:** UX-001, UX-008, UX-009, UX-010, UX-011, P1-CONSOLE-003, RT-003, RT-005, PORTAL-EXPERIENCE-004, CONTRACT-001, CONTRACT-002

#### Scenario: 分页加载集群列表
- **GIVEN** 已纳管多个 KubernetesTarget 与 EdgeRuntimeTarget
- **WHEN** 用户访问 `/resource/clusters`
- **THEN** 页面从 Read Model 接口分页加载集群列表
- **AND** 创建与纳管的集群在统一表格展示并以来源字段区分
- **AND** 状态列使用服务端 `resource.cluster.status` 字典渲染标签与统一语义色
- **AND** 翻页、过滤和排序由服务端执行并返回精确总数

#### Scenario: 实际消费 L2 Schema
- **GIVEN** apiserver 为集群列表返回兼容的 L2 `PageSchema` 及已注册的 dataSource endpoint ID
- **WHEN** 用户打开集群列表
- **THEN** Schema Renderer 使用返回的 Schema 构建页面并通过共享鉴权客户端查询 Read Model
- **AND** 页面不使用本地硬编码列表替代该 Schema

#### Scenario: 无权限访问列表
- **GIVEN** 当前用户无 `cluster:list` 权限
- **WHEN** 用户访问集群列表页
- **THEN** 页面显示 no-permission 状态且不展示受保护数据
- **AND** 后端接口独立拒绝未授权请求

#### Scenario: 列表没有集群
- **GIVEN** 当前租户具有 `cluster:list` 权限但没有 KubernetesTarget 或 EdgeRuntimeTarget
- **WHEN** Read Model 返回成功且总数为零
- **THEN** 页面显示 empty 状态而非 error 状态
- **AND** 仅在用户具有 `cluster:create` 权限时显示创建或纳管入口

#### Scenario: 页面离线或 Schema 不兼容
- **GIVEN** 浏览器离线、Read Model 不可达或 `PageSchema.minShellVersion` 高于当前 Shell 版本
- **WHEN** 用户打开集群列表或详情
- **THEN** 页面分别显示可重试的 offline、error 或 schema-incompatible 状态
- **AND** schema-incompatible 时不执行任何 Schema 区域或动作
- **AND** 离线时不得把缓存状态表示为实时状态或启用写动作

#### Scenario: 切换租户时旧请求迟到
- **GIVEN** 租户 A 的集群列表请求尚未完成
- **WHEN** 用户切换到租户 B 且租户 A 的响应随后到达
- **THEN** Web Console 丢弃租户 A 的响应并只呈现租户 B 的 Schema、字典和集群数据
- **AND** 租户 B 的路由与动作仅按租户 B 的权限启用

### Requirement: [UX-022] 集群注册/创建向导
Web Console SHALL 提供 L3 注册组件形式的集群注册/创建向导，SHALL 支持创建 KubernetesTarget 与纳管已有集群（kubeconfig SecretReference 或 CloudCore endpoint），提交 SHALL 通过 `POST /api/v1/runtime-intents` 携带 Idempotency-Key 与 Correlation-ID，并 SHALL 在提交后跟踪 Operation 状态进入 Operation Center。该向导 SHALL 是可实际提交的 create/import 流程而非占位表单：创建 KubernetesTarget SHALL 生成 `CreateRuntimeTarget`，通过 kubeconfig `SecretReference` 纳管已有 Kubernetes 集群以及通过 CloudCore endpoint 纳管 KubeEdge 集群 SHALL 生成 `ImportRuntimeTarget`，且 Edge 路径 SHALL 创建 `EdgeRuntimeTarget`。向导 SHALL 仅接受 `KubernetesTarget` 与 `EdgeRuntimeTarget`，不得提供工作负载、联邦成员或 `ContainerEngineTarget` 流程。服务端 SHALL 校验目标租户、`targetKind`、SecretReference 归属、Provider 能力和输入，并 SHALL 以受信端点解析 RuntimeIntent；浏览器不得上传或回显 Secret 值，也不得直接调用 Provider 或目标集群。

创建和纳管 SHALL 要求所选租户内的 `cluster:create` 权限；仅有 `cluster:list` 或 `cluster:read` 不得提交。向导 SHALL 在提交前展示服务端 validate/planning 结果和 ExecutionPlan 摘要，提交期间防止重复提交，并 SHALL 使用相同 Idempotency-Key 安全重试。提交成功后 SHALL 从 RuntimeIntent 解析实际 `operationId`，轮询浏览器可用的 Operation Read Model，采用退避、终态停止和卸载取消策略，并提供指向 Operation Center 对应 Operation 详情的深链接；提交回执不得被表示为运行成功。向导 SHALL 覆盖 loading、error、empty（无可用 Provider/选项）、no-permission、offline 和 incompatible 状态，并在租户切换时取消提交前请求、清除草稿中的租户资源引用且丢弃旧租户响应。

**Traceability:** UX-003, UX-009, UX-010, UX-011, P1-CONSOLE-003, RT-002, RT-006, OP-003, OP-004, GOV-004, CONTRACT-002

#### Scenario: 创建 KubernetesTarget
- **GIVEN** 用户在当前租户具有 `cluster:create` 权限且存在兼容的 Kubernetes lifecycle Provider
- **WHEN** 用户完成创建步骤、审阅 ExecutionPlan 摘要并提交向导
- **THEN** Web Console 提交 `CreateRuntimeTarget` RuntimeIntent 及 Idempotency-Key 与 Correlation-ID
- **AND** 服务端创建真实 RuntimeIntent、ExecutionPlan 和 Operation
- **AND** 页面轮询该 Operation 并提供 Operation Center 详情深链接

#### Scenario: 纳管 EdgeRuntimeTarget
- **GIVEN** 用户选择纳管一个 KubeEdge 集群并填写 CloudCore endpoint 与节点组映射
- **WHEN** 向导提交
- **THEN** 平台创建 `ImportRuntimeTarget` RuntimeIntent 并返回 `RuntimeIntentRecord`
- **AND** 该 Intent 的目标类型为 `EdgeRuntimeTarget`
- **AND** 前端轮询对应 Operation、显示注册进度并提供 Operation Center 深链接
- **AND** 前端不直接连接边缘节点

#### Scenario: 通过 SecretReference 纳管 KubernetesTarget
- **GIVEN** 用户选择纳管已有 Kubernetes 集群并选择当前租户拥有的 kubeconfig SecretReference
- **WHEN** 用户审阅计划并提交
- **THEN** 平台创建目标类型为 `KubernetesTarget` 的 `ImportRuntimeTarget`
- **AND** 浏览器请求和界面均不包含 kubeconfig Secret 值

#### Scenario: 提交被拒
- **GIVEN** 向导提交的 RuntimeIntent 校验失败
- **WHEN** 平台执行 validate/planning
- **THEN** 平台返回 RFC 9457 Problem 且不产生任何运行时副作用
- **AND** 前端展示字段级或全局 error 状态且不进入 Operation 跟踪

#### Scenario: 用户没有创建权限
- **GIVEN** 用户具有 `cluster:list` 和 `cluster:read` 但没有 `cluster:create`
- **WHEN** 用户打开或直接调用创建/纳管流程
- **THEN** Web Console 显示 no-permission 状态并不提交 RuntimeIntent
- **AND** 服务端独立拒绝伪造请求

#### Scenario: 向导依赖不可用
- **GIVEN** Provider 列表为空、浏览器离线或服务端报告 Provider 与目标版本不兼容
- **WHEN** 用户进入向导或尝试进入下一步
- **THEN** 向导分别显示 empty、offline 或 incompatible 状态及安全的恢复操作
- **AND** 在依赖恢复并重新校验计划之前提交保持禁用

### Requirement: [UX-023] 集群升级与解除纳管操作
集群详情页 SHALL 提供升级与解除纳管操作，类型 SHALL 为 `operation`，高风险动作 SHALL 二次确认并展示影响范围，写请求 SHALL 只引用受信 `endpointId`，执行结果 SHALL 进入统一 Operation Center 跟踪。升级 SHALL 要求当前租户的 `cluster:update` 权限，解除纳管 SHALL 要求 `cluster:delete` 权限；服务端 SHALL 根据 RuntimeIntent kind、targetId、targetKind 和租户归属逐动作授权，不得以列表、读取或通用 intent 权限代替。每次动作 SHALL 使用 Idempotency-Key 与 Correlation-ID，提交成功后 SHALL 解析并轮询实际 Operation，终态停止，并提供 Operation Center 详情深链接；页面 SHALL 明确区分请求已接受、等待审批、执行中、成功和失败。

当服务端新鲜度判断为 `STALE` 时，Web Console SHALL 展示 `lastKnownStateAt`、影响范围和非预选的显式风险确认；未确认的请求 SHALL NOT 提交。确认信息 SHALL 随请求发送，但 SHALL NOT 决定是否执行；服务端策略 SHALL 独立返回允许执行、排队/等待审批或拒绝，并记录 actor、目标、新鲜度和决策审计。Web Console SHALL 严格呈现该服务端决策，不得将显式确认解释为成功、不得绕过审批或拒绝，也不得以客户端缓存的新鲜状态覆盖服务端判断。动作区域 SHALL 覆盖 loading、error、no-permission、offline 和 incompatible 状态；没有可用动作时 SHALL 显示 empty 状态而非空白控件。

**Traceability:** UX-003, UX-009, UX-010, UX-011, RT-004, RT-005, OP-004, GOV-002, GOV-003, CONTRACT-002

#### Scenario: 升级处于 DEGRADED 的集群
- **GIVEN** 集群状态为 `DEGRADED` 且用户具有 `cluster:update`
- **WHEN** 用户点击升级并确认维护窗口
- **THEN** 操作按钮进入 loading 且防重复提交
- **AND** 提交 `UpgradeRuntimeTarget` 并轮询对应 Operation
- **AND** 页面提供 Operation Center 详情深链接且仅在 Operation 成功终态后显示成功

#### Scenario: 对 STALE 集群执行写操作
- **GIVEN** 服务端判定集群超过 RT-005 新鲜度阈值并返回 `STALE` 与 `lastKnownStateAt`
- **WHEN** 用户选择升级或解除纳管
- **THEN** Web Console 展示时间、影响范围及非预选的显式风险确认
- **AND** 未确认时不发送写请求
- **AND** 确认后由服务端决定允许执行、排队/等待审批或拒绝并记录审计
- **AND** 前端按服务端决策展示状态且不得显示为实时成功

#### Scenario: 动作权限彼此独立
- **GIVEN** 用户具有 `cluster:update` 但不具有 `cluster:delete`
- **WHEN** 用户查看同一集群的可用操作
- **THEN** 升级动作可用而解除纳管动作隐藏或禁用并说明无权限
- **AND** 服务端允许合法升级请求并拒绝伪造的解除纳管请求

#### Scenario: 动作不可执行
- **GIVEN** 浏览器离线、动作 Schema 不兼容、服务端加载失败或目标没有兼容 lifecycle Provider
- **WHEN** 用户打开详情动作区
- **THEN** 页面分别显示 offline、incompatible、error 或 empty 状态
- **AND** 不发送 RuntimeIntent 且不保留可误触的写按钮

### Requirement: [UX-024] 节点只读视图
集群详情 SHALL 提供节点只读面板，数据来源 SHALL 为 Agent 上报或 CloudCore 代理，Web Console SHALL NOT 直接连接边缘节点，且节点状态过期时 SHALL 展示 `lastKnownStateAt` 与过期提示。节点面板 SHALL 作为 L3 注册组件嵌入 L2 集群详情 Schema，仅对 `KubernetesTarget` 与 `EdgeRuntimeTarget` 可用，并 SHALL 通过 apiserver 节点 Read Model 的服务端分页、过滤和排序接口读取投影数据；不得一次性加载全部节点或在浏览器侧伪造分页。节点读取 SHALL 要求当前租户的 `cluster:read`，服务端 SHALL 校验 targetId 与租户归属。每条节点记录 SHALL 携带 `lastKnownStateAt`，页面 SHALL 按服务端新鲜度结果逐条显示最后已知时间和 STALE/offline 提示，不得把最后已知状态表示为实时状态。

节点面板 SHALL 覆盖 loading、error、empty、no-permission、offline 和 incompatible 状态；切换租户或 targetId SHALL 取消或隔离在途分页请求，丢弃旧租户或旧目标的迟到响应。分页、状态提示和重试控件 SHALL 在移动端可操作并可由键盘及辅助技术识别。

**Traceability:** UX-008, UX-009, UX-010, UX-011, P1-CONSOLE-003, RT-002, RT-005, RT-007, EDGE-02, CONTRACT-001

#### Scenario: 通过 CloudCore 查看边缘节点
- **GIVEN** 集群为 KubeEdge 纳管集群
- **WHEN** 用户查看集群详情的节点面板
- **THEN** 平台通过 CloudCore API 代理的投影返回节点列表与状态
- **AND** 前端不直接连接边缘节点

#### Scenario: 分页查看节点最后已知状态
- **GIVEN** 一个集群包含超过一页的节点且部分节点已过新鲜度阈值
- **WHEN** 用户翻页或按状态过滤节点
- **THEN** 服务端返回对应分页结果、分页元数据及每条记录的 `lastKnownStateAt`
- **AND** 页面对过期记录显示 STALE/offline 提示和最后已知时间
- **AND** 页面不把过期记录标记为实时在线

#### Scenario: 节点面板完整状态
- **GIVEN** 节点请求正在加载、失败、返回零条、被拒绝、离线或组件版本不兼容
- **WHEN** 节点面板处理对应结果
- **THEN** 面板分别显示 loading、error、empty、no-permission、offline 或 incompatible 状态
- **AND** error 与 offline 状态提供安全重试且 no-permission 状态不泄露节点数据

#### Scenario: 切换集群时节点请求迟到
- **GIVEN** 集群 A 的节点分页请求仍在进行
- **WHEN** 用户切换到集群 B 或另一个租户后集群 A 的响应到达
- **THEN** 节点面板丢弃集群 A 的响应
- **AND** 页面只显示当前租户当前 targetId 的节点

### Requirement: [UX-025] 集群功能模块目录组织
Web Console 资源插件 SHALL 将集群管理相关文件按功能模块分类组织在 `pages/cluster-management/` 下，并 SHALL 划分为 `schemas/`、`components/`、`api/`、`types/` 子目录；标准列表/详情 SHALL 使用 L2 Schema，复杂交互 SHALL 使用 L3 注册组件并遵循组件命名空间规范。集群列表与详情 SHALL 复用 `@hnb/ui-kit` 的页面、表格、分页、字典状态、表单、对话框、通知以及 loading/error/empty/no-permission/offline/incompatible 状态组件；向导、节点面板和 Operation 跟踪等 L3 组件 SHALL 复用同一套 primitives 和设计 token。若通用能力缺失，开发者 SHALL 先以向后兼容、可复用且有测试的方式增强通用 UI Kit 或 Schema Renderer，再由集群模块消费，不得先在集群模块创建重复组件、硬编码颜色或私有状态模式。

集群页面和组件 SHALL 满足键盘操作、可见焦点、语义化名称、状态变化通知、错误关联和适当对比度要求；表格、详情、对话框、向导、节点分页和 Operation 深链接 SHALL 在移动端窄视口下保持可读、可滚动且关键动作可操作。实现与测试 SHALL 仅覆盖 `KubernetesTarget` 和 `EdgeRuntimeTarget` 集群管理，不得在该模块增加工作负载、联邦调度或联邦成员管理范围。

**Traceability:** UX-001, UX-002, UX-008, UX-010, UX-011, CONTRACT-001, CONTRACT-009

#### Scenario: 新增集群子功能
- **GIVEN** 需要为集群管理新增子页面
- **WHEN** 开发者实现该功能
- **THEN** 文件放置在 `cluster-management/` 对应子目录
- **AND** 标准列表与详情实际消费 L2 Schema，复杂交互注册为命名空间 L3 组件
- **AND** 不修改 Shell 核心代码

#### Scenario: 通用组件能力缺失
- **GIVEN** 集群页面需要一种现有 `@hnb/ui-kit` 或 Schema Renderer 未提供的通用状态或交互
- **WHEN** 开发者实现该能力
- **THEN** 先增强通用 UI Kit 或 Schema Renderer 并补充通用测试
- **AND** 集群模块消费增强后的公共组件而不保留重复的本地实现

#### Scenario: 键盘和移动端操作集群页面
- **GIVEN** 用户使用键盘、屏幕阅读器或移动端窄视口
- **WHEN** 用户浏览列表、打开详情、完成向导并进入 Operation 深链接
- **THEN** 焦点顺序、语义名称、状态通知和错误关联可被辅助技术识别
- **AND** 表格和对话框不遮挡关键内容且分页与主要动作保持可操作

#### Scenario: 阻止扩展无关范围
- **GIVEN** 开发者在集群管理模块增加资源类型或页面入口
- **WHEN** 该实现不属于 KubernetesTarget 或 EdgeRuntimeTarget 的集群生命周期与只读节点体验
- **THEN** 该实现不得并入本模块
- **AND** 工作负载与联邦成员管理不得复用集群页面伪装实现

### Requirement: [UX-012] 存储供给与消费视图分离
Portal SHALL 在“资源 → 存储”提供平台管理员的存储总览、存储系统、存储服务、驱动与连接器和告警入口，并在“容器 → 存储”提供应用管理员的 StorageClass、PVC、PV 和 Snapshot 消费视图；两个视图 SHALL 通过 Offering/Binding 链接而不重复拥有同一事实。

**Traceability:** UX-001, UX-002, STO-001, STO-002

#### Scenario: 从存储服务跳转 StorageClass
- **GIVEN** 一个 Offering 在三个目标上有 Binding
- **WHEN** 平台管理员点击关联 StorageClass 数量
- **THEN** Portal 跳转到容器存储并携带 Offering 与目标过滤上下文
- **AND** 列表只展示用户有权访问的 Binding

### Requirement: [UX-013] 存储页面基于能力和新鲜度呈现
Portal SHALL 展示存储事实的来源、新鲜度和 Unknown/Elastic/NotReported 状态，并根据安装能力、Provider Conformance、用户权限和目标能力显示动作；隐藏动作 SHALL NOT 替代服务端授权。

**Traceability:** UX-001, P1-CONSOLE-002, STO-003, RT-005

#### Scenario: 观察数据已陈旧
- **GIVEN** 存储系统最后观测时间超过租户阈值
- **WHEN** 用户打开资源存储详情
- **THEN** Portal 显示 Stale 和最后观测时间
- **AND** 需要新鲜事实的危险动作被禁用并由服务端独立拒绝

### Requirement: [UX-014] 存储兼容路由迁移
Portal SHALL 在新资源存储能力稳定前保留旧容器存储路由；切换时 SHALL 使用数据库导航版本和兼容重定向保留目标、命名空间、Offering 与 StorageClass 查询上下文。

**Traceability:** UX-006, UX-007, STO-002

#### Scenario: 访问旧存储书签
- **GIVEN** 用户保存了旧容器存储 URL 与 StorageClass 过滤参数
- **WHEN** 导航版本已切换到新消费视图
- **THEN** Portal 重定向到兼容页面并保留过滤上下文
- **AND** 权限不足时服务端仍拒绝数据读取
