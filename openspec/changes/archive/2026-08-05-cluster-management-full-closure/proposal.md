## Why

现有资源菜单「集群管理」已具备列表、详情、向导和 RuntimeIntent 骨架，但核心验收仍未闭环：L2 Schema 未实际驱动页面、状态字典未消费、权限命名不一致、写路径缺少目标归属与 STALE 策略校验、Operation 仅展示提交回执、节点 Read Model 没有 Agent/CloudCore 投影来源，集群生命周期计划也缺少可执行 Provider 路由。需要以新的纠偏 change 收敛规格与实现，避免继续把编译通过误认为端到端可用。

## What Changes

- Change ID: `cluster-management-full-closure`。
- Tier: T1 Web/Read Model 能力；依赖 T0 RuntimeIntent、ExecutionPlan、Operation、IAM 与 Provider 执行链路。
- 影响平面：Web Console resource 插件与新增 Operation Center 页面、apiserver BFF、platform-api RuntimeTarget/Intent/Operation、Agent/Edge Provider 观测投影、RuntimeTarget lifecycle Provider。
- **BREAKING**：集群列表只包含 `KubernetesTarget` 与 `EdgeRuntimeTarget`；`ContainerEngineTarget` 不再套用 Kubernetes 集群/节点模型，后续进入独立运行时目标页面。
- **BREAKING**：统一标识与查询字段为 `targetId`/`targetKind`；统一权限为 `cluster:list/read/create/update/delete`，BFF 根据 RuntimeIntent kind 执行动作级授权，platform-api 使用受信服务身份并保留原始 actor 审计上下文，不再要求浏览器同时具备不相关的 `cluster:execute` 与 `intent:create`。
- 将生命周期、健康、连接性和新鲜度拆分建模；STALE 不再覆盖最后已知生命周期状态。STALE 写操作进入显式风险确认，由服务端策略决定允许、排队审批或拒绝，并写审计。
- 发布集群 Read Model、节点、状态字典、RuntimeIntent 和 Operation BFF 的 OpenAPI/Schema 契约，生成 TypeScript/Go 类型；统一 Problem Details 与幂等重放语义。
- Read Model 改为数据库级过滤、精确计数与分页；增加 Agent/CloudCore 观测事件和幂等投影器，持续维护 target、能力快照和节点投影。
- 为集群 RuntimeIntent 解析 Provider、固化逐步输入/SecretReference，并实现 Kubernetes/Edge RuntimeTarget lifecycle Provider 的可执行步骤。
- 新增浏览器可用的 Operation BFF 与 Operation Center 列表/详情；提交后轮询 Read Model、深链到 Operation，后续可选 SSE 仅作加速。
- 使资源插件的列表/详情真实消费 L2 PageSchema；复杂向导、节点面板和 Operation 跟踪继续作为 L3 组件，并实现 `resource.cluster.detail.tabs` 扩展点。
- 将灰度能力移到 apiserver capability/navigation 决策，默认 fail-closed；构建期 flag 仅作部署覆盖，并验证实际 resource 插件 bundle。
- 补齐契约、数据库、Provider、BFF、Vue 单测、Playwright、租户切换、移动端/无障碍、回滚和 live-stack 验收。
- 非目标：不在 resource 插件实现命名空间、工作负载、网络、存储、GPU CRUD；不把普通 RuntimeTarget 与 Karmada member registry 合并；不在本 change 实现联邦调度、DR Placement、节点 cordon/drain UI 或集群备份 UI。

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `portal-experience`: 将 UX-021~025 从页面骨架提升为真实 Schema 渲染、服务端字典、Operation Center 跟踪、动作级权限、STALE 策略交互和可验证多租户体验。
- `runtime-target`: 明确集群目标范围、分离状态维度、增加 Agent/CloudCore 观测投影与服务端 STALE 写策略，并规范 RuntimeTarget lifecycle Provider 执行。
- `platform-kernel`: 明确浏览器到 Operation/Intent 的 BFF 边界、动作级授权、受信服务身份、幂等重放、Problem Details 和逐步 Provider 解析/固化要求。

## Impact

- 受影响代码：`contracts/`、`database/postgresql/migrations/`、`cmd/apiserver`、`cmd/platform-api`、`cmd/operation-worker`、`cmd/cluster-agent`、`cmd/edge-provider`、新增 lifecycle Provider、`web/packages/schema-engine`、`web/plugins/resource`、Operation Center 插件/页面及测试。
- 依赖 change：已归档 `2026-08-01-web-resource-cluster-management`、`runtime-target-engine`、`operation-engine-core`、`complete-console-bff-navigation`、`wire-schema-runtime-e2e`、`runtime-driver-integration`、`edge-pack-integration`。
- 迁移影响：新增规范化观测/投影字段和索引；已有 `runtime_target_nodes` 数据原位迁移；不新增数据库或中间件。PostgreSQL 与 JetStream 沿用现有备份、恢复和升级流程。
- 兼容与回滚：旧原始 Read Model 响应在迁移窗口内由 BFF 适配；新前端只消费生成契约。灰度关闭时 apiserver 不下发菜单/路由；回滚保留数据库向后兼容字段，并可切回旧组件，不删除已记录的 Operation/观测历史。
- 安全风险：跨租户 targetRef、伪造 SecretReference、STALE 风险绕过、重复提交和 Provider 越权。缓解措施为双层租户/实例授权、服务端策略与审计、Header/Body 幂等一致性、SecretReference 归属校验、不可变计划和 fencing。
- 资源预算：复用 PostgreSQL、Transactional Outbox、JetStream 与 Provider HTTP v2；投影器按增量观测写入，列表不在请求路径遍历目标；Operation UI 首版轮询采用退避与终态停止。
- 可观测：记录 intent→plan→operation→provider correlation、投影 lag/丢弃乱序观测、STALE 策略决策、BFF 上游错误和前端 Operation 跟踪失败；日志不得包含 token、kubeconfig 或 Secret 值。
- 用户价值：列表与详情可信、写动作真正可执行且可追踪、节点状态有真实来源、权限和 STALE 风险行为一致，管理员可从提交一直追踪到终态。
- 退出判据：契约生成/校验通过；两类集群目标完成 create/import/upgrade/unmanage 至 Operation 终态；节点观测进入 Read Model；Schema 页面、字典、权限撤回、STALE 决策、租户切换、幂等重放、移动端与回滚 E2E 全部通过。
