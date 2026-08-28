## ADDED Requirements

### Requirement: [UX-021] Schema 驱动的集群列表与详情
Web Console SHALL 提供资源菜单下 Schema 驱动的集群列表与详情页面，列表 SHALL 通过服务端分页 Read Model 接口加载，状态字段 SHALL 使用服务端统一下发的集群状态字典 `resource.cluster.status`，详情 SHALL 展示概览、能力快照、节点只读面板与声明式扩展点。

**Traceability:** UX-001, RT-003, RT-005, PORTAL-EXPERIENCE-004

#### Scenario: 分页加载集群列表
- **GIVEN** 已纳管多个 KubernetesTarget 与 EdgeRuntimeTarget
- **WHEN** 用户访问 `/resource/clusters`
- **THEN** 页面从 Read Model 接口分页加载集群列表
- **AND** 创建与纳管的集群在统一表格展示并以来源字段区分
- **AND** 状态列使用 `resource.cluster.status` 字典渲染统一语义色

#### Scenario: 无权限访问列表
- **GIVEN** 当前用户无 `cluster:view` 权限
- **WHEN** 用户访问集群列表页
- **THEN** 页面按默认拒绝原则不展示受保护数据
- **AND** 后端接口独立拒绝未授权请求

### Requirement: [UX-022] 集群注册/创建向导
Web Console SHALL 提供 L3 注册组件形式的集群注册/创建向导，SHALL 支持创建 KubernetesTarget 与纳管已有集群（kubeconfig SecretReference 或 CloudCore endpoint），提交 SHALL 通过 `POST /api/v1/runtime-intents` 携带 Idempotency-Key 与 Correlation-ID，并 SHALL 在提交后跟踪 Operation 状态进入 Operation Center。

**Traceability:** UX-003, RT-002, RT-006, OP-003, CONTRACT-002

#### Scenario: 纳管 EdgeRuntimeTarget
- **GIVEN** 用户选择纳管一个 KubeEdge 集群并填写 CloudCore endpoint 与节点组映射
- **WHEN** 向导提交
- **THEN** 平台创建 `ImportRuntimeTarget` RuntimeIntent 并返回 `RuntimeIntentRecord`
- **AND** 前端跟踪对应 Operation 并显示注册进度
- **AND** 前端不直接连接边缘节点

#### Scenario: 提交被拒
- **GIVEN** 向导提交的 RuntimeIntent 校验失败
- **WHEN** 平台执行 validate/planning
- **THEN** 平台返回 RFC 9457 Problem 且不产生任何运行时副作用
- **AND** 前端展示错误且不进入 Operation 跟踪

### Requirement: [UX-023] 集群升级与解除纳管操作
集群详情页 SHALL 提供升级与解除纳管操作，类型 SHALL 为 `operation`，高风险动作 SHALL 二次确认并展示影响范围，写请求 SHALL 只引用受信 `endpointId`，执行结果 SHALL 进入统一 Operation Center 跟踪。

**Traceability:** UX-003, RT-004, RT-005, OP-004, GOV-003

#### Scenario: 升级处于 DEGRADED 的集群
- **GIVEN** 集群状态为 `DEGRADED`
- **WHEN** 用户点击升级并确认维护窗口
- **THEN** 操作按钮进入 loading 且防重复提交
- **AND** 提交 `UpgradeRuntimeTarget` 并跟踪 Operation

#### Scenario: 对 STALE 集群执行写操作
- **GIVEN** 集群状态过期（`STALE`，超过 RT-005 新鲜度阈值）
- **WHEN** 用户提交升级或解除纳管
- **THEN** 平台要求显式风险确认或拒绝写操作
- **AND** 前端不得显示为实时成功

### Requirement: [UX-024] 节点只读视图
集群详情 SHALL 提供节点只读面板，数据来源 SHALL 为 Agent 上报或 CloudCore 代理，Web Console SHALL NOT 直接连接边缘节点，且节点状态过期时 SHALL 展示 `lastKnownStateAt` 与过期提示。

**Traceability:** RT-002, RT-005, RT-007, EDGE-02

#### Scenario: 通过 CloudCore 查看边缘节点
- **GIVEN** 集群为 KubeEdge 纳管集群
- **WHEN** 用户查看集群详情的节点面板
- **THEN** 平台通过 CloudCore API 代理返回节点列表与状态
- **AND** 前端不直接连接边缘节点

### Requirement: [UX-025] 集群功能模块目录组织
Web Console 资源插件 SHALL 将集群管理相关文件按功能模块分类组织在 `pages/cluster-management/` 下，并 SHALL 划分为 `schemas/`、`components/`、`api/`、`types/` 子目录；标准列表/详情 SHALL 使用 L2 Schema，复杂交互 SHALL 使用 L3 注册组件并遵循组件命名空间规范。

**Traceability:** UX-001, UX-002, CONTRACT-001

#### Scenario: 新增集群子功能
- **GIVEN** 需要为集群管理新增子页面
- **WHEN** 开发者实现该功能
- **THEN** 文件放置在 `cluster-management/` 对应子目录
- **AND** 标准 CRUD 优先使用 Schema，复杂交互注册为命名空间组件
- **AND** 不修改 Shell 核心代码
