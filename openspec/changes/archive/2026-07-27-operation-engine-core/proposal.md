## Why

Operation 引擎是 HNB Cloud 的核心执行路径——所有部署、升级、回滚、扩缩容都必须通过 Operation 状态机执行。目前 OP-007（状态权威与异步调度分离）已在消息层实现，但 ExecutionPlan 生成、DAG 编排、10 态状态机、断点续作、补偿和审计链尚未实现。Platform Kernel 定义的唯一写路径、查询/控制解耦、数据面隔离等架构约束也依赖 Operation 引擎的完整实现。

本 change 实现完整 Operation 引擎（ExecutionPlan + DAG + 状态机 + 补偿 + 审计）与 Platform Kernel 的 T0 内核边界。

## What Changes

- 定义 ExecutionPlan 数据模型：不可变，固定 Artifact digest、参数、SecretReference、Provider 版本、步骤 DAG
- 实现 DAG 编排引擎：依赖顺序、并行执行、条件分支、跨节点输出绑定
- 实现 10 态 Operation 状态机：Pending → PendingApproval → Queued → QueuedOffline → InProgress → Paused → Compensating → Succeeded/Failed/Cancelled
- 实现 Step 执行器：幂等键、重试策略、超时、检查点、恢复规则
- 实现补偿引擎：按资源类型执行补偿策略，有状态组件保留数据
- 实现 Operation 审计链：发起人、审批人、Release、ExecutionPlan digest、策略结果、步骤日志
- 实现 Read Model 投影：Operation 列表/详情查询从 Read Model 读取
- 定义 T0 内核边界：明确内核包含组件，排除具体实现

## Capabilities

### New Capabilities
- `operation-engine`: 完整 Operation 执行引擎，包含 ExecutionPlan 生成、DAG 编排、10 态状态机、Step 执行器、断点续作、补偿、审计链。

### Modified Capabilities
- `composition-operation`: OP-001~OP-006 新增；OP-007（已有）追加 ExecutionPlan 持久化和 Step 执行器依赖。
- `platform-kernel`: KERNEL-001~KERNEL-004 新增；定义 T0 内核组件清单、唯一写路径、查询/控制解耦、数据面隔离。

## Impact

- **代码**: Operation Engine（状态机、DAG 执行器、Step 执行器、补偿引擎）、ExecutionPlan Engine（计划生成、验证）、Read Model Projector、数据库迁移。
- **API/事件**: 新增 Operation lifecycle 事件（状态变更、Step 完成、补偿触发）；扩展现有 EventEnvelope oneof 使用 StepRequested/StepCompleted/OperationStateChanged。
- **数据**: 新增 Operation、Step、Checkpoint、Compensation、ReadModel 等表；迁移依赖 001（outbox 表）。
- **依赖**: 复用 PostgreSQL + NATS JetStream（OP-007 已有）；不新增强制中间件。
- **资源**: T0 内核必装；预估 1 CPU / 512MB 基线。
- **运维**: 新增 Operation 管理 Runbook、故障恢复流程、SLO 监控。
