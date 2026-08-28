# kubernetes-runtime-provider Specification

## Purpose
TBD - created by archiving change kubernetes-runtime-provider. Update Purpose after archive.
## Requirements
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

### Requirement: [KRP-005] Kubernetes generation CAS 接管
Kubernetes Provider SHALL 在每次 Deployment 副作用前比较目标记录的 generation，并 SHALL 通过 resourceVersion CAS 执行更高 generation 的受控接管。更低 generation SHALL 被拒绝，相同 generation SHALL 仅允许完全相同身份与规格的幂等重放。

**Traceability:** KRP-003, OP-008, RDI-005

#### Scenario: 响应丢失后的接管
- **GIVEN** Deployment 已由较低 generation 创建但 Worker 未收到可提交结果
- **WHEN** 相同逻辑 Step 以更高 generation 重试
- **THEN** Provider CAS 更新 fencing 元数据并观察同一 Deployment
- **AND** 不创建重复资源

### Requirement: [KRP-006] 可 fencing 的逻辑删除
Provider SHALL 要求 delete 输入匹配目标 UID，SHALL 以更高 generation 和 resourceVersion CAS 将 Deployment 缩容为零并保留墓碑，且 SHALL NOT 物理删除作为 fence 的 Deployment。

**Traceability:** KRP-003, OP-008

#### Scenario: 延迟 deploy 在删除后恢复
- **GIVEN** 更高 generation 已提交逻辑删除墓碑
- **WHEN** 较低 generation 的 deploy 延迟到达
- **THEN** Provider 返回 `FENCED`
- **AND** Deployment 不会被重新扩容或重新创建

