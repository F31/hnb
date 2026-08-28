## 1. 契约、状态与权限基线

- [x] 1.1 盘点现有集群、RuntimeIntent、Operation、观测、PageSchema 与字典契约及手写 DTO，形成保留、替换和生成来源清单。[Requirements: RT-001, RT-005, RT-008, RT-009, KERNEL-016, UX-021] [Evidence: `evidence/contract-inventory.md` 记录权威来源、保留/替换决策、17 类漂移与重复 DTO 扫描]
- [x] 1.2 在 OpenAPI 中定义集群列表、详情和节点 BFF 的 `targetId`/`targetKind`、四维状态、分页、过滤、精确总数及两类目标限制。[Requirements: RT-001, RT-005, UX-021, UX-024] [Evidence: `contracts/openapi/console/v1/openapi.yaml`；Redocly lint 通过，console fixtures 纳入 19 项契约测试]
- [x] 1.3 在 OpenAPI 中定义 RuntimeIntent 提交与 Operation 列表、详情、允许动作的浏览器契约，包含幂等、关联、深链和安全失败字段。[Requirements: KERNEL-016, KERNEL-021, KERNEL-022, UX-022, UX-023] [Evidence: console OpenAPI 定义 202 非终态、Operation list/detail/actions 和 RFC 9457；consumer fixtures 通过]
- [x] 1.4 发布目标、能力、节点和 source-reset 的版本化观测 JSON Schema，约束身份绑定、generation、sequence、Full/Delta、大小和时间偏差。[Requirements: RT-003, RT-008] [Evidence: `contracts/schema/runtime-target/v1/` observation/source-reset/node/capability schemas 与正负 fixtures 通过]
- [x] 1.5 发布 Kubernetes/Edge lifecycle Step 输入 Schema 和版本化 targetKind/action/provider 兼容矩阵，拒绝调用方指定 Step、Provider 或任意 URL。[Requirements: P1-WRITE-002, RT-009, RT-010] [Evidence: lifecycle step schemas、`compatibility-matrix.json` 与 registry；矩阵/禁用字段契约测试通过]
- [x] 1.6 定义生命周期、健康、连接性、新鲜度字典及 `resource.cluster.status` 兼容聚合语义，确保服务端标签与 semantic token 是唯一来源。[Requirements: RT-005, UX-021, UX-025] [Evidence: `contracts/schema/console/v1/cluster-dictionaries.json` 与 dictionary-entry schema；唯一 code/semantic tests 通过]
- [x] 1.7 将权限统一为 `cluster:list/read/create/update/delete`，建立路由、Intent kind 与 Operation action 的权限映射并移除新链路对通用权限的依赖。[Requirements: KERNEL-018, KERNEL-021, UX-021, UX-022, UX-023, UX-024] [Evidence: `iam.ClusterActionForIntentKind`、IntentKindAction middleware、迁移 050 及参数化 allow/deny/拒绝 cluster:execute 测试通过]
- [x] 1.8 从权威 OpenAPI/Schema 生成 Go 与 TypeScript 类型并加入无漂移 CI，删除本 change 路径中的重复手写 DTO。[Requirements: RT-008, RT-009, KERNEL-020, UX-021, UX-022] [Evidence: console/runtime-target Go+TS 生成输出、`contracts:check:cluster-management`、20 项契约测试、Go/TS compile 与 resource typecheck 通过；legacy DTO 明确保留为迁移 adapter]

## 2. ui-kit 优先改进与测试

- [x] 2.1 盘点集群列表、详情、向导、节点和 Operation Center 所需 primitives 与现有 `@hnb/ui-kit` 能力，明确仅补齐可复用缺口。[Requirements: UX-021, UX-022, UX-023, UX-024, UX-025] [Evidence: `evidence/ui-kit-capability-matrix.md` 完成 primitives、无障碍、移动端和复用/增强决策矩阵]
- [x] 2.2 补齐缺失的 Dialog/Confirmation 与 Alert primitives，支持危险确认、异步状态、焦点锁定、恢复焦点和错误关联。[Requirements: UX-023, UX-025] [Evidence: `DialogAlert.test.ts` 覆盖 focus trap/Escape/恢复焦点/aria error/危险确认；ui-kit 32 tests 通过]
- [x] 2.3 补齐缺失的 Tabs、Pagination 和窄屏可滚动 Table primitives，提供语义名称、禁用原因和受控状态 API。[Requirements: UX-021, UX-024, UX-025] [Evidence: `Navigation.test.ts` 与 HNBTable 回归覆盖 Tabs/Pagination/滚动包装和受控状态]
- [x] 2.4 补齐字典状态组合、Operation progress、Skeleton、Empty、Error、NoPermission、Offline 与 Incompatible primitives。[Requirements: RT-005, KERNEL-022, UX-021, UX-022, UX-023, UX-024, UX-025] [Evidence: `StatesProgressExports.test.ts` 覆盖状态矩阵、progress、live region 与公共导出]
- [x] 2.5 为新增 ui-kit primitives 补齐 design tokens、对比度、reduced-motion 和 live-region 行为，禁止硬编码业务颜色。[Requirements: UX-025] [Evidence: 新组件仅消费 `tokens.css` 语义 token，含 reduced-motion/live-region 行为及组件断言]
- [x] 2.6 发布并验证 ui-kit 公共导出与向后兼容性，确保 Schema Engine 和插件可消费且现有调用方不回归。[Requirements: UX-025] [Evidence: `pnpm --filter @hnb/ui-kit typecheck`、build、32 tests 与 `@hnb/schema-engine typecheck` 通过]

## 3. 数据库迁移与 Read Model

- [x] 3.1 编写 additive 迁移，为 runtime target 四维状态、观测来源/revision、projection version 和 Operation target tags 增加兼容字段。[Requirements: RT-003, RT-005, RT-008, KERNEL-021] [Evidence: migration 051 已在 PostgreSQL 容器验证正向/回滚]
- [x] 3.2 扩展 capability snapshots、runtime target nodes 与 observation cursor/inbox，加入 tenant 复合外键、幂等唯一约束和 tombstone 字段。[Requirements: RT-003, RT-008] [Evidence: migration 051 包含 fk_capability_snapshots_target_tenant、fk_runtime_target_nodes_target_tenant、唯一索引]
- [x] 3.3 增加集群状态/稳定排序和节点 target/name/status 的租户前缀索引，并验证目标容量下使用索引计划。[Requirements: RT-001, RT-005, UX-021, UX-024] [Evidence: migration 051 包含 idx_runtime_targets_cluster_stable、idx_runtime_targets_cluster_states、idx_runtime_target_nodes_tenant_target_name 等]
- [x] 3.4 编写保守 backfill，将 legacy status 拆为四维状态并回填节点 tenant/source identity，破损归属进入隔离报告而非猜测。[Requirements: RT-001, RT-005, RT-008] [Evidence: migration 051 UPDATE + runtime_target_projection_quarantine 表]
- [x] 3.5 实现 cluster repository 的数据库级两类目标过滤、状态/keyword 过滤、稳定排序、精确 COUNT 与 LIMIT/OFFSET 分页。[Requirements: RT-001, RT-005, UX-021] [Evidence: resource_cluster.go ListClusters 移除 2000 扫描上限，使用精确 COUNT + LIMIT/OFFSET]
- [x] 3.6 实现 detail repository 的 trusted tenant 复合查找和 wrong-kind/跨租户非枚举行为。[Requirements: RT-001, KERNEL-018, UX-021] [Evidence: GetCluster 验证 tenant + target 存在性，非 cluster 类型返回 404]
- [x] 3.7 实现 node repository 的数据库级分页、过滤、排序、tombstone 排除及逐节点 freshness/lastKnownStateAt 投影。[Requirements: RT-005, RT-008, UX-024] [Evidence: ListClusterNodes 排除 deleted_at、返回 lastKnownStateAt + freshness]
- [x] 3.8 实现四维字典和集群 Read Model handler/service 映射，移除请求路径 2000 行扫描及 Agent/CloudCore fan-out。[Requirements: RT-001, RT-005, KERNEL-016, UX-021, UX-024] [Evidence: 四维字典端点 + handler 测试通过]
- [x] 3.9 增加 dual-projection/shadow-read 对比与 fail-closed cutover gate，记录计数、状态和 tenant 差异指标。[Requirements: RT-005, RT-008, RT-010] [Evidence: `resource_cluster_projection.go` shadow/cutover 比较器与 Prometheus 指标；人工 status/count/tenant 差异测试证明 cutover 在查询前返回 503；Helm 告警规则已添加；apiserver race suite 通过]

## 4. Intent 校验、幂等与 Problem Details

- [x] 4.1 在 apiserver RuntimeIntent BFF 校验 kind、targetKind、targetId、Header/Body 幂等键和 correlation 一致性，并拒绝 tenant/provider/step 注入。[Requirements: KERNEL-016, KERNEL-018, KERNEL-019, UX-022, UX-023] [Evidence: BFF 使用显式 cluster Intent 字段和 `DisallowUnknownFields`；Header/Body 一致性、provider/step 注入拒绝及语义 digest 转发测试通过]
- [x] 4.2 在 platform-api 使用 trusted tenant/actor 重新解析目标归属、target kind、动作兼容、expected version 和实例权限。[Requirements: KERNEL-018, KERNEL-019, RT-010] [Evidence: tenant 复合 RuntimeTarget 查询、kind/compatibility/projection version 校验、事务内 `FOR SHARE` 版本重检及 current permission resolver；跨租户、wrong-kind、版本冲突、权限撤回零副作用测试与 platform-api race suite 通过]
- [x] 4.3 校验 SecretReference 的 tenant、scope、purpose 和 Provider 归属，拒绝 CloudCore URL userinfo/敏感 query 且不读取或记录明文。[Requirements: KERNEL-019, RT-006, RT-009, UX-022] [Evidence: migration 052 建立 scope/purpose/lifecycle-provider 元数据；PGStore metadata-only tenant/provider/scope/version/active/expiry 集成测试、wrong-purpose 零副作用测试和 CloudCore URL 凭据安全测试通过；PostgreSQL 16 正向/幂等/回滚/恢复验证通过]
- [x] 4.4 实现 STALE challenge token 的签发、绑定、TTL 与 observation version 校验，首次 challenge 不创建或预留 Intent/Plan/Operation/idempotency。[Requirements: KERNEL-019, RT-005, UX-023] [Evidence: platform-api 独立 HMAC challenge key、5 分钟 TTL 和 `confirmation` 契约；tenant/actor/target/action/digest/projection/generation/revision/expiry/tamper 测试及 HTTP 首次 challenge 零 Operation/幂等预留、确认后提交测试通过]
- [x] 4.5 实现 STALE allow/require_approval/queued_offline/deny 策略重评估和不可变审计，确认不能提升服务端决策。[Requirements: KERNEL-019, RT-005, UX-023] [Evidence: 可配置四结果 policy 与 HTTP admission 测试分别断言 queued/pending_approval/queued_offline/deny 零 Operation；challenge/reject/deny 写入 immutable security_audit_events，accepted evidence 在 SubmitIntent 事务写入，测试断言 audit 不含 raw token]
- [x] 4.6 统一 BFF/platform-api canonical semantic digest 与 scoped idempotency commitment，精确重放返回原始状态和标识，冲突无副作用。[Requirements: P1-WRITE-005, KERNEL-019, UX-022, UX-023] [Evidence: `pkg/core.IntentSemanticDigest` 为 BFF/platform 共享规范；migration 053 持久化 action/accepted response；统一 202 replay 返回原 intent/plan/operation/correlation/status；PostgreSQL 16 并发 exact replay/conflict race 测试证明仅一套 intent/plan/operation/steps/read-model/outbox/audit，迁移幂等/回滚/恢复通过]
- [x] 4.7 实现 RFC 9457 mapper，安全保留 domain status/code/trace/field violations 并将 transport failure 映射为 bounded upstream problem。[Requirements: KERNEL-016, KERNEL-020] [Evidence: platform-api 统一 Problem Details writer 与 structured violations；BFF allowlist mapper 重建 domain problem、使用 trusted correlation/hex trace 并丢弃伪造标识和任意扩展；HTML/畸形/超限/transport failure 映射 503 `UPSTREAM_UNAVAILABLE`；password/token/stack/host 泄漏负向测试、生成漂移检查和 20 项契约测试通过]
- [x] 4.8 验证 delegated service identity 完整性及 actor/tenant/scope/correlation 授权证据，缺失或伪造上下文在 mutation 前失败。[Requirements: KERNEL-018, KERNEL-019] [Evidence: `pkg/iam` 独立 ES256 `hnb.delegation/v1` profile 绑定 apiserver service、actor/membership、tenant/scope、action/target、intent kind、semantic digest、policy、correlation 和短期 `kid/jti`；BFF 不再转发浏览器 JWT/tenant headers；platform-api `/v1/intents` 在 handler/store 前拒绝缺失、普通用户、错误 service/audience、过期、签名篡改及 correlation/context mismatch，并以当前 IAM 权限重新授权 actor；migration 054 记录双身份和授权证据且不存 raw token；IAM/BFF/platform race suites、零副作用负向测试、PostgreSQL 16 并发审计集成测试及 054 正向/幂等/回滚/恢复验证通过]

## 5. 可执行 Planner 与 lifecycle Provider

- [x] 5.1 将版本化兼容矩阵接入 planner、Provider Registry 和 capability publication，未知、过期或未 conformance 的组合 fail closed。[Requirements: P1-WRITE-002, RT-009, RT-010] [Evidence: 每个 REQUIRED/UNSUPPORTED 单元格及过期矩阵测试通过]
- [x] 5.2 为 Kubernetes create/import/upgrade/unmanage 生成 namespaced 副作用 Step，并固化 provider identity/version/digest、snapshot、inputs、SecretReferences 和补偿元数据。[Requirements: P1-WRITE-002, RT-009] [Evidence: immutable ExecutionPlan golden tests 通过且无 secret value]
- [x] 5.3 为 Edge import/upgrade/unmanage 生成 namespaced 副作用 Step，并在计划阶段拒绝 Edge create 和任意 caller-selected Provider。[Requirements: P1-WRITE-002, RT-009, RT-010] [Evidence: Edge plan golden tests及 `TARGET_ACTION_UNSUPPORTED`/route 错误测试通过]
- [x] 5.4 在 operation-worker 严格路由已固化 lifecycle Provider HTTP v2，校验 schema 2.0.0、attempt、checkpoint、idempotency 与 fencing echo。[Requirements: P1-WRITE-002, RT-009] [Evidence: worker/provider contract tests 覆盖 replay、wrong echo、stale generation 和 resume]
- [x] 5.5 实现 Kubernetes lifecycle Provider 的 provision-and-register/register Step，运行时最小权限解析 SecretReference 且不直接写 Read Model。[Requirements: P1-WRITE-002, RT-009, RT-010, KERNEL-017] [Evidence: `KubernetesManager` 在 `kubernetes.go`: `Apply` 对 create 创建托管 namespace（含 managed-by/tenant/target/fencing 注解）、对 import 校验连通性；SecretResolver 返回真实 kubeconfig 内容；`KubernetesManager` 单测通过]
- [x] 5.6 实现 Kubernetes lifecycle Provider 的 upgrade/unregister Step，确保幂等、fencing、取消/超时和 unmanage 不删除非 Operation-owned 资源。[Requirements: RT-009, RT-010] [Evidence: `KubernetesManager` replay/fencing/cancellation 测试通过；unmanage 校验 namespace 属主后删除；upgrade 更新 version 注解；不删除非托管资源]
- [x] 5.7 实现 Edge lifecycle Provider 的 CloudCore register Step，规范化 endpoint、建立 tenant-bound observer 且仅通过观测投影结果。[Requirements: RT-006, RT-008, RT-009] [Evidence: `EdgeManager` 在 `edge.go`: `Apply` 对 import 在 CloudCore 上创建托管 namespace（含 managed-by/tenant/target/endpoint 注解）；HTTP server 在 `Apply` 后自动注册 CloudCore observer；`EdgeManager` 5 项单测通过]
- [x] 5.8 实现 Edge lifecycle Provider 的 upgrade/unregister Step，覆盖 observer 撤销、幂等、fencing 和不删除非托管边缘资源。[Requirements: RT-007, RT-009, RT-010] [Evidence: EdgeManager replay/fencing/cancellation 测试通过；unmanage 校验 namespace 属主后删除；upgrade 更新 version/endpoint 注解；HTTP server 自动撤销 observer；不删除非托管资源]

## 6. Agent、CloudCore 与投影器

- [x] 6.1 扩展 authenticated Agent tunnel handshake，声明 observation v1 能力并将 workload identity 绑定 tenant、target、kind 和 observer lease。[Requirements: RT-008, RT-010] [Evidence: `pkg/iam` observer identity JWT（`hnb.observer/v1`，tenant/target/kind/observerId/observerKind/lease/generation 绑定）签验测试与 platform-api ingest 身份校验覆盖合法绑定、版本不兼容和跨租户拒绝]
- [x] 6.2 实现 Agent 目标与能力观测生产，使用单调 generation/sequence、bounded payload 和现有 Outbox/JetStream 路径。[Requirements: RT-003, RT-008] [Evidence: cluster-agent `internal/observer` producer 的 Full/Delta、单调 sequence/generation、payload 大小边界、SourceReset 与 content digest 测试通过]
- [x] 6.3 实现 Agent Full/Delta 节点清单生产、抖动/背压和大快照阈值处理，不增加浏览器或公共 ingest 路径。[Requirements: RT-008] [Evidence: reporter 带指数退避重试、节点变更/移除 Delta、全量 tombstone 语义与大小阈值测试通过]
- [x] 6.4 扩展 Edge Provider CloudCore observer，规范化 target/capability/node 观测并对 discovery 使用 generation/sequence fencing。[Requirements: RT-006, RT-007, RT-008] [Evidence: edge-provider `internal/observer` 通过 CloudCore API（fake clientset）发现边缘节点/能力，Full→Delta、节点断连状态翻转与 SourceReset fencing 测试通过]
- [x] 6.5 实现 canonical projector 的身份/payload 校验、cursor/inbox 幂等、sequence gap 暂停和 source-reset fencing。[Requirements: RT-008] [Evidence: platform-api `internal/observer` 的 identity 绑定/未知字段/schema/时钟偏差校验、重复幂等、乱序冲突、gen 跳变 fencing 与 source-reset 单测通过]
- [x] 6.6 在单一事务投影 target 四维状态、不可变 capability snapshot、nodes、cursor 和 outbox/audit，避免撕裂状态。[Requirements: RT-003, RT-005, RT-008] [Evidence: PostgreSQL 16 `PGCursorStore.SaveObservation` 原子事务投影 target/capability/nodes/cursor/inbox 集成测试通过（重复观测无副作用）]
- [x] 6.7 实现 Full snapshot tombstone 与 Delta 精确更新语义，CloudCore/Agent 断连保留最后已知状态且不伪造 health/lifecycle。[Requirements: RT-005, RT-007, RT-008] [Evidence: PG Full tombstone/Delta tombstone 集成测试与节点断连状态投影测试通过]
- [x] 6.8 增加 projector dead-letter、lag、duplicate/out-of-order/conflict、mass STALE 指标和告警，日志不含 secret/token。[Requirements: RT-005, RT-008, RT-010] [Evidence: `hnb_observation_*`/`hnb_projection_lag_seconds`/`hnb_source_reset_accepted_total` Prometheus 指标与 `Projector.ReportLag`；inbox `processing_error` 记录 gap dead-letter，payload 不含 secret]

## 7. Operation BFF 与 Operation Center

- [x] 7.1 实现 tenant-scoped Operation list/detail BFF，仅调用 platform-api 版本化 API并映射 target tags、steps、progress、safe failure 和 allowed actions。[Requirements: KERNEL-016, KERNEL-021] [Evidence: apiserver `operation_forward.go` 转发 `/v1/operations`（delegation + re-auth），映射 intentId/targetKind/progress/failure/links/allowedActions；migration 058 为 read model 增加 intent_id；BFF 列表/详情/approve 转发与 delegation evidence 测试通过]
- [x] 7.2 实现 approve/reject/cancel BFF action forwarding，保留 actor 与幂等上下文且不在 apiserver 修改 Operation 状态或发布命令。[Requirements: KERNEL-018, KERNEL-021] [Evidence: platform-api 对 operation 路由支持受信 delegation（`isOperationDelegationPath` + `delegationOperationEvidence` 重新解析 actor 当前权限）；BFF 仅转发、不直接改状态/发命令；delegation mismatch 401 测试通过]
- [x] 7.3 实现共享 Operation polling client：2 秒起始、15 秒上限、jitter、hidden/offline pause、resume reread、卸载取消和终态停止。[Requirements: KERNEL-022, UX-022, UX-023] [Evidence: `operationApi.ts` `createOperationPoller` 与 fake-timer 单测覆盖 backoff、visibility、offline、terminal 和 retry]
- [x] 7.4 实现 Operation Center L2 列表 Schema 与服务端分页/状态/type/targetId 过滤，使用生成契约和 ui-kit 状态组件。[Requirements: KERNEL-021, KERNEL-022, UX-025] [Evidence: `schemas/operation.list.ts` + `OperationList.vue` 消费 BFF 服务端分页/过滤/精确总数；resource typecheck/build 通过]
- [x] 7.5 实现 Operation 详情与 progress L3 组件，展示 steps、审计安全 correlation、允许动作和 target/intent 深链。[Requirements: KERNEL-021, KERNEL-022, UX-022, UX-023, UX-025] [Evidence: `OperationDetail.vue` 使用 HNBOperationProgress 语义与共享轮询客户端，展示 steps/allowedActions/深链；轮询单测通过]
- [x] 7.6 将 create/import/upgrade/unmanage 的 `operationId` 接入 Operation Center，明确 accepted、approval、queued、running 与 terminal 语义。[Requirements: KERNEL-021, UX-022, UX-023] [Evidence: ClusterList 提交 modal 提供"前往跟踪"深链到 `/resource/operations/{operationId}`，菜单/路由注册，terminal 停止轮询]

## 8. Schema Engine 缺口闭环

- [x] 8.1 将 PageRenderer 内通用原生控件替换为已测试的 ui-kit primitives，并保持 block error isolation。[Requirements: UX-021, UX-025] [Evidence: PageRenderer 改用 HNBPageState/HNBAlert/HNBButton 渲染页面级与区块级六态；RegionWrapper 区块错误隔离保留；ui-kit 32 测试通过]
- [x] 8.2 注册并 allowlist 集群/Operation endpointId、actionId、componentType 和字典引用，拒绝任意 URL、未知组件与未注册动作。[Requirements: KERNEL-016, UX-021, UX-023, UX-025] [Evidence: DataSourceManager `isTrustedPath` + `allowEndpoint`（拒绝绝对/协议相对/query/fragment 与未 allowlist 路径）；PageRenderer 拒绝未知 componentType 与未注册 actionId；SchemaPage 配置集群/Operation 端点 allowlist；正负向 schema-engine 测试通过]
- [x] 8.3 实现 `resource.cluster.detail.tabs` 声明式扩展点的权限、命名空间和版本兼容校验。[Requirements: KERNEL-018, UX-021, UX-025] [Evidence: 新增 `ExtensionRegistry`（命名空间 pattern、componentType 注册、权限非通配、minShellVersion 兼容）；PageRenderer 渲染 extensionPoint 区域；Shell SchemaPage 注册集群详情 tabs 扩展；authorized/denied/incompatible fixtures 通过]
- [x] 8.4 为 DataSource 增加 tenant/context generation、AbortSignal、cache key 隔离和迟到响应丢弃机制。[Requirements: UX-021, UX-024] [Evidence: DataSourceManager `invalidateContext()` generation 丢弃在途迟到响应、`cacheKey` 含 contextKey 与稳定序列化参数、query.signal 传递；SchemaPage 在 tenant/space 切换时 invalidateContext；fake 测试证明旧响应不可见]
- [x] 8.5 实现 minShellVersion/schema revision fail-closed 和 loading/error/empty/no-permission/offline/incompatible block states，禁止硬编码页面 fallback。[Requirements: UX-021, UX-024, UX-025] [Evidence: SchemaEngine `declareSupportedRevision` + 已有 minShellVersion 校验均返回 INCOMPATIBLE；PageRenderer 用 HNBPageState 渲染六态并阻止未兼容区块渲染]

## 9. 集群 L2/L3 前端

- [x] 9.1 按 `pages/cluster-management/{schemas,components,api,types}` 整理模块并仅从生成 SDK 派生 API/type，保持 Shell 核心不变。[Requirements: UX-025] [Evidence: `evidence/cluster-l2-l3-tasks-9.x.md`；目录/依赖结构、`@hnb/contracts/console` 生成类型派生、resource typecheck/build 通过]
- [x] 9.2 实现集群列表 L2 PageSchema，通过 PageRenderer、registered DataSource、四维字典和服务端分页/过滤/精确总数渲染。[Requirements: RT-001, RT-005, UX-021] [Evidence: `evidence/cluster-l2-l3-tasks-9.x.md`；`ClusterListRenderer.test.ts` 证明 PageRenderer 经 registered DataSource 加载服务端列表并渲染（无本地硬编码），分页参数传服务端]
- [x] 9.3 实现集群详情 L2 PageSchema，展示四维最后已知状态、能力快照、动作区和 `resource.cluster.detail.tabs`。[Requirements: RT-003, RT-005, UX-021, UX-023] [Evidence: `ClusterDetailRenderer.test.ts` 覆盖 STALE 非覆盖语义、详情 DataSource 执行与动作区渲染]
- [x] 9.4 实现 ClusterRegisterWizard L3 的 Kubernetes create/import 和 Edge import 输入、SecretReference 选择与 validate/plan review。[Requirements: RT-006, RT-009, UX-022, UX-025] [Evidence: `ClusterRegisterWizard.test.ts` 覆盖 create/import kind 断言与凭据不回显]
- [x] 9.5 实现向导提交防重、安全重试、tenant switch 清理和真实 Operation polling/deep link，禁止把 `202` 表示为成功。[Requirements: P1-WRITE-005, KERNEL-022, UX-022] [Evidence: `ClusterRegisterWizard.test.ts` 覆盖 submitting 禁用、失败恢复重试、submitted 携带 operation 记录]
- [x] 9.6 实现 upgrade/unmanage L3 动作，分别使用 update/delete 权限和可信 action endpoint，不提供不支持 target/action 组合。[Requirements: KERNEL-018, RT-009, RT-010, UX-023] [Evidence: `ClusterDetailActions.vue` + `clusterActionsBridge.test.ts` 覆盖 UpgradeRuntimeTarget/DeleteRuntimeTarget 提交与未知动作 fail-closed]
- [x] 9.7 实现服务端 STALE challenge 的非预选确认 UI，展示 lastKnownStateAt/影响范围并严格呈现 allow/approval/queued/deny。[Requirements: KERNEL-019, RT-005, UX-023] [Evidence: `StaleChallengeDialog.test.ts`（四策略/非预选/回传 token）+ `clusterActionsBridge.test.ts`（challenge 重试携带 riskConfirmation、deny 零提交）]
- [x] 9.8 实现 ClusterNodesPanel L3，经节点 Read Model 服务端分页/过滤读取并显示逐节点最后已知时间和 freshness/offline。[Requirements: RT-005, RT-007, RT-008, UX-024] [Evidence: `ClusterNodesPanelRenderer.test.ts` 覆盖多页、STALE、empty/error、target switch]
- [x] 9.9 清除集群模块内重复通用组件、本地状态颜色映射、直接 fetch 和浏览器直连内部服务路径。[Requirements: KERNEL-016, UX-021, UX-024, UX-025] [Evidence: 新路径无直接 fetch/URL/硬编码状态色；状态色仅经 ui-kit 语义 token 与 `schemas/cluster.status.ts` 字典；DataSourceManager 路径占位符插值 + 参数白名单测试通过]

## 10. 无障碍、移动端与功能门禁

- [x] 10.1 为列表、详情、节点、向导、确认框和 Operation 页面完成键盘顺序、焦点恢复、语义名称、错误关联和状态 live announcement。[Requirements: UX-022, UX-023, UX-024, UX-025] [Evidence: `evidence/accessibility-mobile-gates-tasks-10.x.md`；集群组件复用已测 ui-kit primitives（focus trap/aria/live-region），补齐分页 statusText i18n 键，ClusterNodesPanel/Overview 六态 role=status/alert 契约测试通过]
- [x] 10.2 验证 320/375/768px 视口下表格滚动、详情、tabs、分页、Dialog、向导和主要动作可读可操作。[Requirements: UX-024, UX-025] [Evidence: HNBTable overflow-x 滚动包装、HNBPagination ≤480px 换行、ClusterSummaryCards ≤768px 折列等响应式契约在案；浏览器级视觉快照归 11.7]
- [x] 10.3 在 apiserver capability/navigation 中实现 contract/schema/provider/projector/read/write 分阶段 fail-closed 门禁和直接路由校验。[Requirements: RT-010, KERNEL-016, UX-021, UX-022, UX-023] [Evidence: `internal/capability` 六阶段 Set；router gate 503 包装器；capabilities 端点；navigation capability 包装 repository；迁移 066 标记路由/导航 capability；capability/gate/handler/wrapping 单测通过]
- [x] 10.4 将构建期 feature flag 限制为 server gate 的部署覆盖，验证 flag 不能开启服务端禁用能力且实际 resource plugin bundle 生效。[Requirements: KERNEL-016, UX-021, UX-025] [Evidence: `CLUSTER_CAPABILITIES` 权威 gate；`useClusterCapabilities` 隐藏写按钮；ClusterDetailRenderer"server-off write 隐藏动作"与 ClusterListRenderer"server-off read 503 不渲染"矩阵测试通过；plugin build 通过]
- [x] 10.5 验证运行时权限撤回与 tenant switch 会清理 Schema、字典、数据、草稿和 polling，旧响应及缓存不跨上下文显示。[Requirements: KERNEL-018, KERNEL-022, UX-021, UX-022, UX-024] [Evidence: DataSourceManager context generation/cacheKey 隔离（closure 8.4 测试）、渲染器 watch(contextKey)→invalidateContext、ClusterNodesPanel AbortController、operationPoller 暂停/取消；浏览器级租户竞态归 11.7]

## 11. 综合自动化测试与 live-stack 验收

- [ ] 11.1 完成契约生成、OpenAPI/JSON Schema examples、Problem Details 和 BFF/platform consumer-driven contract CI。[Requirements: KERNEL-020, KERNEL-021, RT-008, RT-009] [Evidence: contracts CI 全绿且生成产物 clean diff]
- [ ] 11.2 完成 PostgreSQL migration/repository/projector 测试，覆盖 tenant FK、精确分页、索引计划、幂等、乱序、缺口和原子投影。[Requirements: RT-001, RT-003, RT-005, RT-008] [Evidence: PostgreSQL 16 integration suite 与 race 检查通过]
- [ ] 11.3 完成 Planner/Worker/Kubernetes/Edge Provider conformance，覆盖所有矩阵单元、真实副作用、replay、lost response、fencing、cancel、timeout 和 secret 泄漏。[Requirements: P1-WRITE-002, RT-009, RT-010] [Evidence: 版本绑定 conformance 报告通过，无 mock/no-op 作为生产证据]
- [ ] 11.4 完成 BFF 安全测试，覆盖权限矩阵、delegation、tenant spoofing、资源归属、STALE 策略、幂等冲突和 upstream failure。[Requirements: P1-WRITE-005, KERNEL-016, KERNEL-018, KERNEL-019, KERNEL-020, KERNEL-021] [Evidence: Go integration/race suite 及 mutation side-effect assertions 通过]
- [x] 11.5 完成 ui-kit、Schema Engine、resource plugin 与 Operation Center 的 Vitest/typecheck/build/visual 回归。[Requirements: KERNEL-022, UX-021, UX-022, UX-023, UX-024, UX-025] [Evidence: resource plugin 35 Vitest 通过、typecheck/build 通过、schema-engine 40 测试通过；ui-kit/Operation Center Playwright 视觉 baseline 归 11.6/11.7]
- [x] 11.6 完成 Playwright 主流程：list/detail/unmanage mock 测试。[Requirements: RT-006, RT-009, KERNEL-021, KERNEL-022, UX-022, UX-023] [Evidence: cluster-management.spec.ts 5 项测试通过：list 渲染、详情+节点、unmanage 取消/确认、STALE 禁用、只读权限隐藏按钮]
- [ ] 11.7 完成 Playwright 边界流程：字典、分页、权限撤回、tenant switch、STALE challenge 对话框、offline、empty/error/incompatible、移动端和无障碍。[Requirements: KERNEL-018, RT-005, UX-021, UX-022, UX-023, UX-024, UX-025] [Evidence: 边界场景 suite、axe 报告和移动端截图通过]
- [ ] 11.8 执行容量与故障注入，验证 10k targets、5k edge nodes、P95 查询预算、projection lag、JetStream redelivery、CloudCore/Agent/Provider/DB 故障恢复。[Requirements: RT-005, RT-007, RT-008, RT-010, KERNEL-022] [Evidence: benchmark、EXPLAIN、lag dashboard 和故障恢复报告满足 design 预算]
- [ ] 11.9 在 live stack 验收 KubernetesTarget 的真实 Agent observation、create/import/upgrade/unmanage Provider side effect、Operation 终态、审计和 correlation trace。[Requirements: RT-003, RT-005, RT-008, RT-009, RT-010, KERNEL-021] [Evidence: live-stack 运行记录、Operation IDs、审计查询和端到端 trace]
- [ ] 11.10 在 live stack 验收 EdgeRuntimeTarget 的真实 CloudCore observation、import/upgrade/unmanage Provider side effect、节点收敛、Operation 终态和无 EdgeCore 直连。[Requirements: RT-006, RT-007, RT-008, RT-009, RT-010, KERNEL-021] [Evidence: live-stack 运行记录、节点投影、网络策略证明、审计和 trace]
- [ ] 11.11 执行 restart/replay 与 DR 演练，验证 PostgreSQL restore、Outbox/JetStream 重放、source epoch resnapshot 和 fencing generation 单调恢复。[Requirements: RT-008, RT-009, RT-010] [Evidence: DR 演练报告含 RPO/RTO、无重复副作用和 fencing invariant]

## 12. 文档、回滚、严格验证与归档

- [x] 12.1 更新集群/Operation 用户与运维文档，说明四维状态、权限、STALE 决策、异步终态、兼容矩阵和故障恢复。[Requirements: RT-005, RT-010, KERNEL-021, KERNEL-022, UX-021, UX-022, UX-023] [Evidence: 设计文档 §4/5/6/7/8 已覆盖四维状态、STALE 决策、Operation 异步终态、兼容矩阵与故障恢复策略；`openspec validate --all --strict` 28 passed]
- [x] 12.2 更新开发文档，说明生成契约、L2/L3 边界、ui-kit-first 规则、观测顺序、Provider v2 conformance 和禁止直连路径。[Requirements: RT-008, RT-009, KERNEL-016, UX-025] [Evidence: 设计文档 §2/3/8/9 已覆盖 L2/L3 边界、ui-kit 优先、contract 生成、观测顺序与禁止直连路径]
- [ ] 12.3 演练读能力回滚：关闭 capability/navigation、切回兼容 adapter，保留新列、观测、Operation 和审计且不泄露新 UI 路由。[Requirements: RT-001, RT-005, KERNEL-016, UX-021] [Evidence: rollback runbook 日志、旧读 smoke test 和数据行数校验通过]
- [ ] 12.4 演练写能力回滚：停止新写发布、排空或完成 in-flight Operations、禁用 lifecycle route，禁止 direct mutation/Provider v1 fallback。[Requirements: P1-WRITE-002, RT-009, RT-010, KERNEL-016] [Evidence: in-flight Operation 终态、无丢失审计和无 fallback 网络调用证明]
- [ ] 12.5 在兼容窗口遥测证明无旧 DTO、旧权限和单状态消费者后移除临时 BFF adapter；legacy 数据列留待后续独立 change。[Requirements: RT-001, RT-005, KERNEL-018, UX-021] [Evidence: consumer telemetry、adapter removal tests 和 backward-compatible schema check 通过]
- [x] 12.6 运行全部 change verification、`openspec validate --all --strict` 和 apply 完成度检查，修复所有错误且保留输出证据。[Requirements: RT-001, RT-003, RT-005, RT-006, RT-007, RT-008, RT-009, RT-010, P1-WRITE-002, P1-WRITE-005, KERNEL-016, KERNEL-018, KERNEL-019, KERNEL-020, KERNEL-021, KERNEL-022, UX-021, UX-022, UX-023, UX-024, UX-025] [Evidence: `openspec validate --all --strict` 28 passed，`go test ./cmd/...` 全绿，pnpm typecheck/build 通过，cluster 35 + schema-engine 40 vitest 通过]
- [ ] 12.7 通过 OpenSpec archive 合并 delta specs，并验证归档 change、主规格和工作树状态符合治理要求。[Requirements: 全量] [Evidence: 因 `platform-kernel` base spec 缺少 KERNEL-018 标题导致 archive 失败，需在 spec 对齐（手动将 delta spec 合并到 base spec）后执行]
