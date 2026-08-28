# 集群 L2/L3 前端（9.x）证据

范围：`openspec/changes/cluster-management-full-closure/tasks.md` 任务 9.1–9.9。
对应 specs：`portal-experience`（UX-021/022/023/024/025、KERNEL-016/018/019/021/022、
RT-001/003/005/006/007/008/009/010、P1-WRITE-005）。

## 9.1 模块结构与生成 SDK 派生

- 目录按 `pages/cluster-management/{schemas,components,api,types,runtime,composables}` 组织，
  与 spec §9.1 的 `schemas/`、`components/`、`api/`、`types/` 划分一致。
- `types/cluster.ts` 从 `@hnb/contracts/console` 生成 SDK 派生 `ClusterIntentKind` 与
  `SecretReference`（无手写重复 DTO）；状态/字典来自服务端 `resource.cluster.status`。
- 验证：
  - `pnpm --filter @hnb/plugin-resource typecheck`（vue-tsc --noEmit）通过；
  - `pnpm --filter @hnb/plugin-resource build`（vite）通过，114 modules，dist/index.js 171.80 kB；
  - `@hnb/schema-engine`、`@hnb/api-client` typecheck 通过。

## 9.2 集群列表 L2 PageSchema（RT-001/RT-005/UX-021）

- `schemas/cluster.list.ts` 定义 `resource.cluster.list` PageSchema：
  注册 `resource.clusters.list/detail/nodes` 与 `runtime-intents.submit` endpoint，
  声明 `resource.cluster.list` paginatedQuery dataSource（queryBindings keyword/kind/status，
  responseMapping items/total）。
- region 绑定：`summary → resource.ClusterSummaryCards`（dataSource 注入）、
  `table → ClusterTable`（dataSource 注入），全部经 PageRenderer + registered DataSource 渲染，
  无本地硬编码列表。
- 组件测试 `__tests__/ClusterListRenderer.test.ts`：
  - 断言 PageRenderer 实际经 `apiClient.get('/api/v1/resources/clusters', { params: { page, pageSize } })`
    加载服务端列表（非本地数组），并把响应行渲染进表格；
  - 断言分页参数 page=1 / pageSize=20 传到服务端。

## 9.3 集群详情 L2 PageSchema（RT-003/RT-005/UX-021/UX-023）

- `schemas/cluster.detail.ts` 定义 `resource.cluster.detail` PageSchema：
  overview/config 使用 `resource.ClusterOverviewPanel`（展示最后已知状态、能力快照），
  actions 使用 `resource.ClusterDetailActions`（升级/解除纳管，走可信 action endpoint），
  nodes 使用 `ClusterNodesPanel`（L3 嵌入），并保留 `resource.cluster.detail.tabs` 扩展点。
- `ClusterDetailRenderer.vue` 从 URL 解析 clusterId，经 `useDataSource`（注入运行时同源
  DataSourceManager）读取详情，STALE 时展示 HNBAlert 过期提示（不覆盖、不伪造实时状态）。
- 组件测试 `__tests__/ClusterDetailRenderer.test.ts`：详情 DataSource 注册并加载、
  STALE 展示"状态已过期"而非实时状态、RUNNING 不展示过期提示、动作区渲染升级/解除纳管按钮。

## 9.4 向导 Kubernetes create/import + Edge import（RT-006/RT-009/UX-022/UX-025）

- `components/ClusterRegisterWizard.vue`：create 走 `CreateKubernetesTarget`，
  import 走 `ImportRuntimeTarget`（Kubernetes kubeconfig SecretReference / Edge CloudCore endpoint），
  两步来源配置→确认提交；凭据仅经 SecretReference，前端不接收/不回显明文。
- 组件测试 `__tests__/ClusterRegisterWizard.test.ts`：create 提交断言 kind=CreateKubernetesTarget、
  Edge import 断言 kind=ImportRuntimeTarget 且 secretReferences 不伪造。

## 9.5 向导提交防重/安全重试/202 语义（P1-WRITE-005/KERNEL-022/UX-022）

- 向导 submit 用 `submitting` 状态禁用按钮（防重双击），失败展示错误并可重试，
  提交结果经 `submitted` 事件携带 intent/operation 记录，202 不表示为成功。
- 组件测试：submitting 期间按钮 disabled、失败后恢复重试、提交结果含 operationId。

## 9.6 upgrade/unmanage L3 动作（KERNEL-018/RT-009/RT-010/UX-023）

- `components/ClusterDetailActions.vue`（L3）经 `ClusterActionsBridge.triggerRowAction`
  分发 `resource.cluster.upgrade` / `resource.cluster.delete`，分别走
  `UpgradeRuntimeTarget` / `DeleteRuntimeTarget` RuntimeIntent（update/delete 权限语义），
  不提供不支持 target/action 组合；未知 action fail-closed。
- 测试 `__tests__/clusterActionsBridge.test.ts`：delete/upgrade 提交 kind 断言、
  未知动作抛错。

## 9.7 STALE challenge 非预选确认（KERNEL-019/RT-005/UX-023）

- `components/StaleChallengeDialog.vue`：展示 lastKnownStateAt、四维状态（lifecycle/health/
  connectivity）与影响范围，严格呈现服务端 policyOutcome（allow/require_approval/
  queued_offline/deny），确认必须非预选勾选；确认后回传
  `riskConfirmation{acknowledged:true, confirmation:<opaque token>}`。
- `api/clusterApi.ts`：`submitRuntimeIntent` 支持 riskConfirmation；`staleChallengeFromError`
  结构化解析 `STALE_CONFIRMATION_REQUIRED` ProblemDetails 扩展字段（不依赖 instanceof，
  避免插件/Shell 双副本问题）。`@hnb/api-client` 的 `ApiError` 新增白名单 `problem` 扩展字段。
- `composables/useClusterActionsBridge.ts`：初次提交遇 challenge 时挂起重试回调并通知
  challenge；确认后携带 token 重试；`STALE_POLICY_DENIED` 直接抛错不提交。
- 测试：
  - `__tests__/StaleChallengeDialog.test.ts`：四策略严格文案、非预选勾选、确认回传 token、
    未勾选不触发；
  - `__tests__/clusterActionsBridge.test.ts`：challenge 触发→确认重试携带 riskConfirmation、
    deny 决策零提交。

## 9.8 ClusterNodesPanel L3（RT-005/RT-007/RT-008/UX-024）

- `components/ClusterNodesPanelRenderer.vue`：经 `resource.cluster.nodes` paginatedQuery
  服务端分页读取，逐节点展示 lastHeartbeatAt 与 STALE/offline 提示，empty/error 用
  HNBPageState 六态，clusterId 由 props 或详情页注入。
- 测试 `__tests__/ClusterNodesPanelRenderer.test.ts`：多页（翻页触发第二次服务端请求）、
  empty、error、STALE 过期提示、target 切换重载。

## 9.9 清理（KERNEL-016/UX-021/UX-024/UX-025）

- 新渲染路径无直接 `fetch`/axios/直连内部服务 URL；region 数据统一经 registered
  DataSource + 共享鉴权客户端。
- 状态色仅经 ui-kit 语义 token（StatusBadge semantic、`--hnb-color-status-*`）与
  `schemas/cluster.status.ts` 字典映射，无本地硬编码状态颜色。
- DataSourceManager 增强：路径占位符 `{param}` 插值（detail/nodes endpoint 用 clusterId），
  queryBindings + contextBindings + 路径占位符白名单（V2.5 §10.4 之外参数丢弃）；
  相应 `packages/schema-engine/src/__tests__/` 通过。

## 验证汇总

| 命令 | 结果 |
| --- | --- |
| `pnpm --filter @hnb/plugin-resource typecheck` | 通过 |
| `pnpm --filter @hnb/plugin-resource build` | 通过（114 modules） |
| `pnpm --filter @hnb/schema-engine typecheck` | 通过 |
| `pnpm --filter @hnb/api-client typecheck` | 通过 |
| `npx vitest run plugins/resource/src/pages/cluster-management/__tests__/` | 31 tests 通过 |
| `npx vitest run packages/schema-engine/src/__tests__/` | 40 tests 通过 |
| 全量 `npx vitest run` | 14 项预存在失败不变（SchemaPage 2 / api-client 2 / NavigationManager 1 / PluginManager 8 / PluginRegistry 1），无新增回归 |
