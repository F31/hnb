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
Operation Step SHALL 具备幂等键、重试策略、超时、检查点和恢复规则；控制器重启 SHALL 从持久化状态恢复而非重复创建资源。

**Traceability:** CMPOS-04

#### Scenario: Worker 在步骤中重启
- **GIVEN** 一个部署已完成前两步
- **WHEN** platform-worker 重启
- **THEN** Operation 从最近检查点继续
- **AND** 已成功步骤不会重复产生资源

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
