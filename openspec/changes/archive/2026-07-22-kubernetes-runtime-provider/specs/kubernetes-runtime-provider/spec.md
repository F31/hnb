## ADDED Requirements

### Requirement: [KRP-001] 受限 Deployment 生命周期
Kubernetes Provider SHALL 仅接受声明支持的 Deployment `deploy` 和 `delete` 动作，SHALL 验证标量输入和副本上限，并 SHALL NOT 接受任意 Kubernetes Manifest 或集群级资源。

**Traceability:** PROV-003, RDI-002, RT-004

#### Scenario: 提交任意 YAML
- **GIVEN** Step 输入包含任意 Kubernetes YAML
- **WHEN** Provider 验证请求
- **THEN** Provider 拒绝请求且不写入 Kubernetes API

### Requirement: [KRP-002] 租户所有权与命名空间隔离
Provider SHALL 仅写入显式允许的 Namespace，并 SHALL 使用托管标记和权威 Tenant/Operation/Step 元数据标记资源；已有非托管资源或其他 Tenant 资源 SHALL NOT 被接管。

**Traceability:** RDI-004, PROV-002

#### Scenario: 同名非托管 Deployment
- **GIVEN** 目标 Namespace 已存在同名非 HNB Deployment
- **WHEN** Provider 执行 deploy
- **THEN** Provider 返回冲突且不修改该资源

### Requirement: [KRP-003] 幂等与 fencing
Provider MUST 在副作用边界校验幂等键和 fencing token；相同标识的重放 SHALL 返回同一资源结果，不同 token 的覆盖 SHALL 被拒绝，删除 SHALL 使用期望 fencing token 和资源 UID 前置条件。

**Traceability:** OP-004, RDI-003, RDI-004

#### Scenario: 相同请求重放
- **GIVEN** Deployment 已由相同幂等键和 fencing token 创建
- **WHEN** Provider 再次收到请求
- **THEN** Provider 不重复创建并返回已有资源

#### Scenario: 陈旧 token 覆盖
- **GIVEN** Deployment 标记了不同 fencing token
- **WHEN** Provider 尝试部署或删除
- **THEN** Provider 返回冲突且不产生副作用

### Requirement: [KRP-004] 可观测完成与故障隔离
Provider SHALL 在调用上下文内等待 Deployment Available 或删除已提交，SHALL 在取消和 Kubernetes 故障时返回失败，并 SHALL 提供健康检查且不持有 Operation 数据库或 NATS 凭据。

**Traceability:** PROV-002, PROV-004, RDI-003

#### Scenario: Deployment 可用
- **GIVEN** Kubernetes 接受 Deployment 且副本变为 Available
- **WHEN** Provider 观察到目标 generation 完成
- **THEN** Provider 返回 succeeded、资源 Outputs 和 Checkpoint
