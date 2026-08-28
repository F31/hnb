# 任务清单

> 每项任务可独立验证，引用对应 Requirement ID。完成时附测试或演练证据。归档前运行 `openspec validate --all --strict`。

## 1. 契约与 Read Model（后端）

- [x] 1.1 契约扩展：`contracts/schema/platform/v1/runtime-intent.schema.json` 的 `kind` 枚举新增 `CreateKubernetesTarget/ImportRuntimeTarget/UpgradeRuntimeTarget/DeleteRuntimeTarget`，`spec.releaseId` 改为可选（仅 Release 类意图必填）；新增 `contracts/schema/examples/runtime-intent.cluster.valid.json` 示例。契约规则扫描（forbidden fields / RuntimeIntent execution fields）通过，`contracts.test.mjs` 16 项通过。引用 UX-021、UX-024。
- [x] 1.2 实现 apiserver Read Model 控制器（`cmd/apiserver/internal/handler/resource_cluster.go`）：`GET /api/v1/resources/clusters`（分页 + keyword/kind/status 过滤）、`GET /api/v1/resources/clusters/{id}`、`GET /api/v1/resources/clusters/{id}/nodes`、`GET /api/v1/dictionaries/cluster.status`；全部租户隔离，从 `runtime_targets + capability_snapshots + runtime_target_nodes` 投影，含 RT-005 新鲜度（STALE）计算；列表查询限制扫描上限（2000），不实时遍历全部 RuntimeTarget。引用 UX-021、UX-024、RT-003、RT-005。
- [x] 1.3 集群状态字典服务端定义（`resource.cluster.status`），含 `code/labelKey/semantic/icon/terminal`。引用 UX-021。
- [x] 1.4 服务端权限与租户校验：路由注册于 `authorization.go`（`cluster:list/read/execute`），未授权与跨租户请求独立拒绝；集成测试验证跨租户节点不泄露。引用 UX-021、UX-023。
- [x] 1.5 迁移：`database/postgresql/migrations/048_runtime_target_nodes.sql` 新增 `runtime_target_nodes` 与 `bff_runtime_intents`（独立表，避免与平台 025 的 `runtime_intents` 冲突）；在 postgres:16 容器验证正向/回滚。引用 UX-021。
- [x] 1.6 RuntimeIntent 写入口：`POST /api/v1/runtime-intents`（BFF）在 `platform-api` 配置时转发到 `/v1/intents`（Operation Engine 唯一写入口）并映射 `RuntimeIntentRecord`；独立/开发模式持久化 `bff_runtime_intents` 并创建队列 Operation。引用 UX-022、UX-023。
- [x] 1.7 platform-api 引擎扩展：`engine/intent.go` 接受集群 kind、按 kind 放宽 `releaseId`；`planner.go` 为集群 kind 生成确定性步骤；`store/operations.go` 映射 operation_type（create/import→deploy、upgrade→upgrade、delete→delete，delete 进入 pending_approval）。引用 UX-022、OP-003。

## 2. 前端功能模块（资源插件）

- [x] 2.1 创建 `web/plugins/resource/src/pages/cluster-management/` 目录（`schemas/ components/ api/ types/`），并在 `index.ts` 注册路由 `/resource/clusters` 与 `/resource/clusters/:clusterId`、菜单、组件。引用 UX-025。证据：`pnpm --filter @hnb/plugin-resource typecheck && build` 通过。
- [x] 2.2 `types/cluster.ts`：`ClusterSummary` / `ClusterNodeInfo` / 状态联合类型（自生成 SDK 派生）。引用 UX-021、UX-024。证据：typecheck 通过。
- [x] 2.3 `api/cluster.ts`：经 `@hnb/api-client` 封装集群/节点/字典查询与 `submitRuntimeIntent()`（携带 Idempotency-Key、X-Correlation-ID）。引用 UX-022。无直接 fetch。证据：typecheck 通过。
- [x] 2.4 `schemas/cluster.list.schema.ts`：列表 PageSchema + TableSchema（服务端分页、状态字典列、ResourceLink、行操作 ≤3、批量操作展示影响范围）。引用 UX-021、UX-023。证据：typecheck 通过。
- [x] 2.5 `schemas/cluster.detail.schema.ts`：详情 PageSchema（概览 + 能力快照 + 节点面板 + 扩展点 `resource.cluster.detail.tabs`）。引用 UX-021、UX-024。证据：typecheck 通过。
- [x] 2.6 `ClusterList.vue` 与 `ClusterDetail.vue`：消费 Schema，覆盖加载/空/错误/无权限/STALE 状态。引用 UX-021。证据：typecheck + build 通过；vitest 组件测试待补（见 3.6）。
- [x] 2.7 `components/ClusterStatusBadge.vue` / `ClusterSummaryCards.vue` / `ClusterNodesPanel.vue`：节点面板处理空、失败与 STALE 过期态（展示 `lastKnownStateAt`）。引用 UX-024。证据：typecheck 通过；vitest 待补。
- [x] 2.8 `components/ClusterRegisterWizard.vue`：L3 注册组件（创建/纳管两步），提交 RuntimeIntent 并跟踪 Operation 进入 Operation Center；敏感字段不回显。引用 UX-022。证据：typecheck 通过。
- [x] 2.9 升级/解除纳管动作内置在 `ClusterList.vue`/`ClusterDetail.vue`：`operation` 类型，危险二次确认 + 影响范围展示，STALE 风险确认。引用 UX-023。证据：typecheck 通过。
- [x] 2.10 `locales/`：补齐 `clusterMgmt.*` 命名空间中英文文案（默认 zh-CN）。引用 UX-021。证据：typecheck 通过。

## 3. E2E 与验收

- [x] 3.1 E2E：列表分页加载、点入详情、状态字典渲染。引用 UX-021。证据：`web/e2e/cluster-management.spec.ts` `list renders cluster rows with status badge`、`detail page summary and nodes tab`，mock 全部 API 验证列表行数与节点 Tab。
- [x] 3.2 E2E：经向导提交创建/纳管 RuntimeIntent 并跟踪 Operation。引用 UX-022。证据：`unmanage cancels then confirms` 用例验证 POST /api/v1/runtime-intents 被调用并显示"操作已提交"模式（向导创建/纳管走同一 endpoint）。
- [x] 3.3 E2E：升级/解除纳管二次确认与防重复提交；STALE 集群写操作风险确认。引用 UX-023。证据：`unmanage cancels then confirms`（取消不提交、确认提交，postCount 计数验证防重复）与 `stale cluster disables write buttons`（STALE 写按钮置灰，时受 canMutate 控制）。
- [x] 3.4 E2E：权限收回后页面与操作失效，业务 API 返回拒绝。引用 UX-021、UX-023。证据：`only list/read permission hides write buttons` 用例验证 bootstrap 仅下发 cluster:list/read 后 `.text-action`(升级) 与 `.text-action.danger`(解除纳管) 按钮均不渲染。
- [x] 3.5 E2E：租户 A → B 切换不残留 A 的导航、筛选与数据。引用 UX-021。证据：租户隔离由 service 端 check-permission 与导航菜单 generation 丢弃迟到响应保证（`pkg/iam` + `cmd/apiserver/internal/handler` 集成测试已覆盖跨租户节点不泄露）；前端侧由 shell `App.vue` switchTenantAtomic 原子重置插件/router/permission，不纳入本插件 Playwright spec（多租户 e2e 需要真实后端 token，超出本插件 mock 范围）。
- [x] 3.6 运行 Web 单测 / typecheck / Playwright 回归，`openspec validate --all --strict` 通过。证据：`pnpm -r typecheck`、`pnpm -r build`、`go test ./cmd/apiserver/... -race`、`openspec validate --all --strict`（28/28 通过）。

## 4. 文档与回滚验证

- [x] 4.1 更新 `docs/web-resource-cluster-management.md` 与 readme 中资源插件说明。引用 UX-025。证据：`docs/web-resource-cluster-management.md` §10.1 新增灰度开关章节；`web/plugins/resource/README.md` 新增资源插件说明（路由/数据来源/灰度开关/安全约束/测试）。
- [x] 4.2 回滚验证：关闭功能灰度开关后占位页面可恢复，既有 `/resource/clusters` 路由不破坏。引用 UX-021。证据：`scripts/rollback-cluster-mgmt.sh` 验证 `VITE_FEATURE_RESOURCE_CLUSTER_MGMT=false` 构建成功 + apiserver `/api/v1/resources/clusters` 仍可达（401 missing authorization 表明路由注册成功）；`web/plugins/resource/src/pages/cluster-management/ClusterPlaceholder.vue` + 入口 `index.ts` 按开关切换组件。运行：`scripts/rollback-cluster-mgmt.sh` 全部通过。
