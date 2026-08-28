# 无障碍、移动端与功能门禁（10.x）证据

范围：`openspec/changes/cluster-management-full-closure/tasks.md` 任务 10.1–10.5。
对应 specs：`portal-experience`（UX-021/022/023/024/025、KERNEL-016/018/022、
RT-005/006/008/009/010、P1-WRITE-005）。

## 10.3 服务端分阶段 fail-closed 能力门禁（RT-010/KERNEL-016/UX-021/022/023）

- 新增 `cmd/apiserver/internal/capability` 包：
  - 定义六阶段能力：`cluster.contract` / `cluster.schema` / `cluster.provider` /
    `cluster.projector` / `cluster.read` / `cluster.write`（`StageOrder`）。
  - `Set.FromCSV`：逗号分隔启用阶段，空串=全量（保持现有部署默认）；未知名称一律 fail-closed
    （`isKnown` 校验，未知/空名永不启用）。
- 路由级门禁 `cmd/apiserver/internal/router/router.go`：
  - `gate(caps, stage)` 包装器在进入 handler 前对禁用阶段返回 503 `capability_disabled`。
  - 映射：read 路由（list/get/nodes/dictionary）→ `cluster.read`；
    write 路由（runtime-intents + Operation approve/reject/cancel）→ `cluster.write`（`providerGate` 叠加）；
    schema.page → `cluster.schema`（`contractGate` 叠加）；provider 门禁随写路由。
  - `NewWithCapabilities` 显式注入能力集合；`New` 默认全量（向后兼容）。
- 能力端点 `cmd/apiserver/internal/handler/capability.go`：
  - `GET /api/v1/capabilities` → 启用阶段列表；
  - `GET /api/v1/capabilities/{name}` → `{available}`（fail-closed）。
  - 在 authorization 元数据中注册（`navigation:read`），AuthzMW 放行。
- 导航门禁 `cmd/apiserver/internal/infrastructure/navigation/capability_wrapping.go`：
  - 包装 repository，把启用阶段并入 snapshot capabilities，禁用阶段自动移除菜单/路由。
- 迁移 `database/postgresql/migrations/066_cluster_management_capability_gates.sql`：
  - 给 `resource.clusters`/`resource.operations` 路由与 `nav.resource.clusters`/
    `nav.resource.operations` 导航项标记 `capability='cluster.read'`。
- 测试：
  - `capability/capability_test.go`：CSV 默认全量/子集/未知名 fail-closed/Snapshot。
  - `router/capability_gate_test.go`：禁用阶段 503、启用阶段放行、未知能力 503。
  - `handler/capability_test.go`：list/get 可用性、未知名 fail-closed。
  - `infrastructure/navigation/capability_wrapping_test.go`：阶段合并与保留。

## 10.4 构建期 flag 是 server gate 的部署覆盖（KERNEL-016/UX-021/UX-025）

- 服务端 `CLUSTER_CAPABILITIES` 配置（`config.go`）是权威 gate；构建期
  `VITE_FEATURE_CLUSTER_SCHEMA_RENDERER` 只决定编译哪个渲染器，无法开启服务端禁用能力：
  - 服务端禁用 `cluster.read` 时，导航不发布菜单/路由，且直接 API 路由 503；
  - 前端 `useClusterCapabilities` 查询 `/api/v1/capabilities/{name}` 仅用于隐藏入口/禁用动作，
    不是安全边界；每条 gated 路由服务端仍独立 fail-closed。
- 前端注入：plugin create(ctx) 通过 `setClusterCapabilityManager(ctx.capability)` 注入
  `CapabilityManager`；`ClusterDetailActions` 在 `cluster.write` 未启用时隐藏升级/解除纳管按钮。
- 测试：
  - `ClusterDetailRenderer.test.ts`："服务端禁用 write 能力时隐藏写动作"（server-off/build-on）；
  - `ClusterListRenderer.test.ts`："服务端禁用 read 能力时（503）不渲染任何集群数据"。
- 矩阵（design §8）：server-on/build-off → 页面可用；server-off/build-on → 导航隐藏 + API 503 + 前端不渲染/不提供写按钮。

## 10.1 无障碍（UX-022/023/024/025）

- 集群组件复用 ui-kit primitives（HNBPageState/HNBTable/HNBPagination/HNBDialog/
  HNBConfirmation/HNBAlert），其 focus trap/Escape/aria/live-region 已有 2.x 测试。
- 新组件审计：
  - `StaleChallengeDialog`：`initial-focus`、非预选确认复选框、`role=dialog`+`aria-modal`、
    busy/error 关联；
  - `ClusterNodesPanelRenderer`/`ClusterOverviewPanel`：`aria-busy`、HNBPageState 六态
    （loading=role status、error=role alert 带安全重试）；
  - 分页 `HNBPagination` 内含 live-region（statusText）。
- 修复：补齐缺失的 `resource.clusterMgmt.pagination.statusText` i18n 键（zh/en），
  消除 `[intlify] Not found` 告警并保证分页 live-region 有内容。
- 测试：`ClusterNodesPanelRenderer.test.ts` 新增 aria-busy/live-region/role=alert 契约断言。

## 10.2 移动端（UX-024/UX-025）

- ui-kit HNBTable 提供 `max-width:100%` + `overflow-x:auto` 滚动包装；
  HNBPagination 在 ≤480px 居中换行；ClusterSummaryCards ≤768px 折成 2 列；
  ClusterList/Detail/Wizard 已有 ≤768px media query 与窄屏表单布局。
- jsdom 无法验证真实布局，Playwright 视觉/移动断言按 tasks.md 作为验收证据保留在 11.x/11.7。

## 10.5 权限撤销与 tenant switch 清理（KERNEL-018/KERNEL-022/UX-021/022/024）

- DataSourceManager（8.4 已测）：`invalidateContext()` 递增 generation，在途迟到响应被丢弃；
  cacheKey 含 contextKey 与稳定序列化参数，跨租户/上下文隔离（`closure.test.ts`）。
- 集群渲染器：`watch(contextKey) → runtime.invalidateContext()`（ClusterListRenderer/
  ClusterDetailRenderer），`onBeforeUnmount` 也 invalidate；ClusterNodesPanelRenderer 用
  AbortController 取消在途请求。
- Operation 轮询：`operationPoller`（7.3 已测）在隐藏/离线暂停、卸载取消、终态停止。

## 验证汇总

| 命令 | 结果 |
| --- | --- |
| `go build ./cmd/apiserver/...` | 通过 |
| `go vet`（capability/router/handler/infrastructure-navigation） | 通过 |
| `go test ./cmd/apiserver/internal/capability/... ./cmd/apiserver/internal/handler/... ./cmd/apiserver/internal/router/... ./cmd/apiserver/internal/infrastructure/navigation/...` | 通过 |
| `pnpm --filter @hnb/plugin-resource typecheck` | 通过 |
| `pnpm --filter @hnb/plugin-resource build` | 通过（100 modules） |
| `npx vitest run plugins/resource/src/pages/cluster-management/__tests__/` | 35 tests 通过 |

> 备注：Playwright 键盘/axe 扫描、320/375/768px 视觉快照、权限撤销/租户竞态端到端、
> server-off/build-on 与 bundle smoke 的浏览器级证据归属 11.x/11.7 live 验收阶段。
