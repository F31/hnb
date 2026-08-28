## Why

Web Console 资源菜单下的「集群」目前仅占位（`ClusterList.vue` 只渲染标题），未消费 RuntimeTarget 领域模型。用户无法查看、创建或纳管 KubernetesTarget / EdgeRuntimeTarget，也无法跟踪集群状态与节点。需要按 UI 规范 V2.5 的四级扩展模型落地集群管理功能，同时严格遵循白皮书 Operation 唯一写入口与 Read Model 查询约束。

## What Changes

- Change ID: `web-resource-cluster-management`
- Tier: T1（默认交付）；集群写动作依赖 T0 RuntimeIntent 链路与 T1 Read Model。
- Impacted planes: Web Console Shell（resource 插件）、apiserver/Read Model、RuntimeTarget 领域、UI Registry。
- 在 `web/plugins/resource` 新增 `cluster-management` 功能模块：集群列表、集群详情、注册/创建向导（L3 注册组件）、升级/解除纳管操作、节点只读视图。
- 页面与组件按功能模块分类组织：`schemas/`（L2 PageSchema/TableSchema）、`components/`（L3 注册组件与展示单元）、`api/`（Read Model 查询 + RuntimeIntent 提交）、`types/`。
- 列表/详情/节点查询全部走 Read Model 接口（分页），写动作统一经 `POST /api/v1/runtime-intents` 提交 Typed RuntimeIntent，不新增同步写 REST 接口。
- 集群状态使用服务端统一字典（`resource.cluster.status`），前端不自定义状态语义色。
- Dependencies: `runtime-target-engine`、`complete-console-bff-navigation`、`wire-schema-runtime-e2e`。
- Migration impact: 既有 `/resource/clusters` 路径与菜单保持不变，占位页面替换为真实实现，不破坏既有路由。
- Rollback strategy: 占位页面保留在 git 历史；功能灰度开关控制展示；PageSchema 低版本兼容旧组件。

## Capabilities

### New Capabilities

- `portal-experience`: 集群列表、详情、注册/创建向导、升级/解除纳管操作、节点只读视图的 Schema 驱动页面与注册组件。

### Modified Capabilities

- None.

## Impact

- Affected code: `web/plugins/resource/src/`（新增 `cluster-management/` 模块、`index.ts` 路由/菜单/组件注册、`locales/`）、`cmd/apiserver` 或 Read Model 控制器（集群/节点/字典查询）、`contracts/openapi` 与 `contracts/schema`（新增集群 Read Model 与字典契约）。
- APIs: 新增 `GET /api/v1/resources/clusters`、`GET /api/v1/resources/clusters/{clusterId}`、`GET /api/v1/resources/clusters/{clusterId}/nodes`、`GET /api/v1/dictionaries/resource.cluster.status`；写复用 `POST /api/v1/runtime-intents`。
- Dependencies: RuntimeTarget Read Model 投影、IAM 权限快照（`cluster:*`）、UI Registry（PageSchema/字典）、统一 API Client。
- Security risks: kubeconfig / CloudCore 凭据泄露；STALE 目标写操作；跨租户状态复用。缓解：SecretReference 不落前端日志、RT-005 风险确认、租户隔离缓存键与 generation 丢弃。
- Resource budget: 前端仅新增资源插件代码与 Schema；后端仅新增只读 Read Model 控制器，无新数据库或中间件。
- Observability: 接口加载/失败/耗时埋点、动作执行、Operation 状态变化上报；日志不含 token 与 Secret。
- Exit criteria: 登录后可查看集群列表与详情；可经向导提交创建/纳管 RuntimeIntent 并进入 Operation Center；节点面板只读展示并正确处理 STALE 过期态；权限不足时页面与操作 fail-closed；`resource.cluster:*` 权限收回后接口拒绝；契约校验门禁通过。

## Affected Specs

- `portal-experience`（ADDED Requirements）
