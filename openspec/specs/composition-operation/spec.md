# composition-operation

## Purpose
定义 CompositionRelease、ExecutionPlan、Operation 状态机、DAG 和补偿语义。
## Requirements
### Requirement: [OP-001] 不可变 ExecutionPlan
平台 SHALL 将 ReleaseManifest 或 CompositionRelease 解析为不可变 ExecutionPlan，固定目标、Artifact digest、参数、SecretReference、Provider 版本、策略结果和步骤 DAG。

**Traceability:** CMPOS-03, INT-03

#### Scenario: 计划生成后发布被修改
- **GIVEN** 一个 ExecutionPlan 已经批准
- **WHEN** 市场产生新的 Release
- **THEN** 已批准计划仍引用原版本和 digest
- **AND** 重新部署新版本必须生成新计划

### Requirement: [OP-002] DAG 与输出绑定
CompositionRelease SHALL 支持依赖顺序、并行、条件、参数映射和跨节点输出绑定；Helm MAY 作为节点执行器，但 SHALL NOT 成为平台组合编排的唯一机制。

**Traceability:** CMPOS-01, CMPOS-02, CMPOS-08

#### Scenario: 部署应用数据库缓存组合
- **GIVEN** 一个组合包含 PostgreSQL、Valkey 和业务应用
- **WHEN** 平台执行组合
- **THEN** 数据库和缓存就绪后其连接输出被绑定到应用
- **AND** 可并行步骤并行执行

### Requirement: [OP-003] 持久化状态机
Operation SHALL 至少支持 Pending、PendingApproval、Queued、QueuedOffline、InProgress、Paused、Compensating、Succeeded、Failed、Cancelled；终态 SHALL 不再自动迁移。

**Traceability:** EDGE-18, METH-02

#### Scenario: 离线目标上的部署
- **GIVEN** EdgeRuntimeTarget 当前离线
- **WHEN** 用户提交允许排队的部署
- **THEN** Operation 进入 QueuedOffline
- **AND** 超过 maxOfflineDuration 后按策略失败并告警

### Requirement: [OP-004] 幂等恢复与断点续作
Operation Step SHALL 具备幂等键、重试策略、超时、检查点和恢复规则；控制器重启 SHALL 从持久化状态恢复而非重复创建资源。Worker 调用 Runtime Provider 时 SHALL 传播权威幂等键、已提交 Checkpoint 和当前 fencing token，且仅在严格验证 Provider 成功响应后提交成功结果。

**Traceability:** CMPOS-04, RDI-002, RDI-003

#### Scenario: Worker 在步骤中重启
- **GIVEN** 一个部署已完成前两步
- **WHEN** platform-worker 重启
- **THEN** Operation 从最近检查点继续
- **AND** 已成功步骤不会重复产生资源

#### Scenario: Provider 调用后发生重投
- **GIVEN** Provider 已收到包含幂等键和 fencing token 的执行请求
- **WHEN** Worker 在提交结果前重启并重新执行 Step
- **THEN** Provider 使用幂等键避免重复创建资源
- **AND** Worker 只接受当前 Lease 下可 fenced commit 的结果

### Requirement: [OP-005] 补偿与有状态安全
失败补偿 SHALL 按资源类型和策略执行；有状态组件失败时默认 SHALL NOT 自动删除数据卷、备份或持久化实例。

**Traceability:** CMPOS-05

#### Scenario: 组合部署中数据库后续步骤失败
- **GIVEN** 数据库已经初始化数据
- **WHEN** 业务应用部署失败并触发补偿
- **THEN** 系统保留数据库数据并将其标记为需人工处理
- **AND** 无状态资源可按策略自动回滚

### Requirement: [OP-006] 审计与证据链
每个 Operation SHALL 记录发起人、审批人、来源 Release、ExecutionPlan digest、策略结果、Provider、步骤日志、最终结果和回滚证据。

**Traceability:** INT-03, AI-OPS-03

#### Scenario: 审计一次生产回滚
- **GIVEN** 生产实例发生回滚
- **WHEN** 审计人员查看 Operation
- **THEN** 可以还原从用户请求到目标资源变化的完整链路
- **AND** 敏感字段经过脱敏

### Requirement: [OP-007] Operation 状态权威与异步调度分离
Operation、Step、Checkpoint、Idempotency、Lease 和 Approval 的持久化存储 SHALL 是执行状态的唯一权威；异步消息系统 SHALL 仅传递执行意图和已提交事实。Worker SHALL 在执行前读取权威状态并获取有效 Lease，在持久化结果和待发布事件后确认消息；消息系统不可用 SHALL NOT 丢失已提交 Operation 或导致绕过 Operation 的直接执行。

**Traceability:** CMPOS-03, CMPOS-04, INT-05, OP-003, OP-004

#### Scenario: 消息系统在 Operation 提交后不可用
- **GIVEN** API 已在同一事务中提交 Operation 和待发布事件
- **WHEN** 异步消息系统暂时不可用
- **THEN** Operation 保持 Queued 且待发布事件可重试
- **AND** 消息系统恢复后 Operation 自动继续而无需重新提交

#### Scenario: Worker 在外部调用后确认前重启
- **GIVEN** Worker 已调用 Provider 且消息尚未确认
- **WHEN** Worker 重启并重新收到同一 Step 命令
- **THEN** Worker 从权威状态、Lease 和 Checkpoint 判断恢复动作
- **AND** 已成功的外部资源不会被重复创建

#### Scenario: 陈旧消息尝试执行已结束 Operation
- **GIVEN** Operation 已处于 Succeeded、Failed 或 Cancelled 终态
- **WHEN** Worker 收到该 Operation 的陈旧 Step 命令
- **THEN** Worker 不执行任何目标写操作并安全确认消息
- **AND** 陈旧投递进入可观测计数和审计上下文

### Requirement: [OP-008] 单调外部执行 fencing
平台 SHALL 为每次成功的 Step Lease 获取分配全局唯一且单调递增的 fencing generation，并 SHALL 在 Lease 释放后保留该 Step 已授予的最大 generation。Worker 的续租、重试和结果提交 MUST 同时匹配当前 attempt identity、generation、owner 和有效 Lease。

**Traceability:** OP-004, OP-007, RDI-005

#### Scenario: Provider 成功后 Worker 崩溃
- **GIVEN** Worker A 以 generation 41 创建了外部资源但尚未提交 Step 结果
- **WHEN** Lease 过期且 Worker B 获得更高 generation
- **THEN** Worker B 安全接管同一资源并提交一次成功结果
- **AND** Worker A 的延迟外部写入和数据库提交均被拒绝

#### Scenario: Lease 释放后重新获取
- **GIVEN** 一个 Step 的 Lease 已释放或删除
- **WHEN** 该 Step 再次获取 Lease
- **THEN** 新 generation 大于此前已授予的 generation
- **AND** generation 不因事务回滚、冲突或进程重启而复用

### Requirement: [OP-009] ExecutionPlan 节点组亲和性
ExecutionPlan SHALL 支持 `node_group_affinity` 字段，指定目标节点组列表。Worker 在路由 Step 时 SHALL 将该字段传递给 Provider，Provider SHALL 将其映射为目标运行环境（Kubernetes 节点标签或 KubeEdge 节点组）的亲和性约束。

**Traceability:** EDGE-04, ENG-001, ERP-005

#### Scenario: 指定节点组的 ExecutionPlan
- **GIVEN** 边缘环境包含 node-group-a 和 node-group-b
- **WHEN** 用户创建 ExecutionPlan 指定 `node_group_affinity: ["node-group-a"]`
- **THEN** 所有 Step 均路由到该节点组
- **AND** 非该节点组的目标不会执行这些 Step
