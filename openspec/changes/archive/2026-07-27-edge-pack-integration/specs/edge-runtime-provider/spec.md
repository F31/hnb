## ADDED Requirements

### Requirement: [ERP-001] Runtime Driver v2 契约
Edge Runtime Provider SHALL 实现 Runtime Driver v2 契约，支持 `2.0.0` 协议版本、十进制 generation、UUID attempt 回显验证，并返回标准错误码（`FENCED`、`RESOURCE_CONFLICT`、`TARGET_UNAVAILABLE`）。

**Traceability:** RDI-001, RDI-002, RDI-005

#### Scenario: Provider 健康检查
- **GIVEN** Edge Runtime Provider 已启动
- **WHEN** Worker 发送健康检查请求
- **THEN** Provider 返回 CloudCore 连接状态和 KubeEdge 版本

#### Scenario: Provider 版本不兼容
- **GIVEN** Provider 仅支持 `2.0.0`
- **WHEN** Worker 以 `1.0.0` 协议版本调用
- **THEN** Provider 拒绝请求并返回协议版本错误

### Requirement: [ERP-002] 受限 EdgeApplication 生命周期
Edge Runtime Provider SHALL 将 HNB 的 `deploy` 动作映射为 KubeEdge EdgeApplication CRD，`delete` 动作映射为删除 EdgeApplication。SHALL 验证输入包含副本数、镜像和端口映射，SHALL NOT 接受任意 Kubernetes Manifest。

**Traceability:** KRP-001, EDGE-02

#### Scenario: 部署边缘应用
- **GIVEN** Step 输入包含合法镜像、副本和端口
- **WHEN** Provider 执行 deploy
- **THEN** Provider 创建 EdgeApplication CRD 并指定目标节点组
- **AND** Provider 返回 succeeded 和资源引用

#### Scenario: 提交任意 YAML
- **GIVEN** Step 输入包含任意 Kubernetes YAML
- **WHEN** Provider 验证请求
- **THEN** Provider 拒绝请求且不写入 Kubernetes API

### Requirement: [ERP-003] 租户隔离与命名空间
Provider SHALL 将 Step 的租户范围映射为 KubeEdge 的命名空间隔离，并 SHALL 使用托管标记标记所创建的资源。非托管资源 SHALL NOT 被接管。

**Traceability:** KRP-002, RDI-004

#### Scenario: 同名非托管 EdgeApplication
- **GIVEN** 目标命名空间已存在同名非 HNB EdgeApplication
- **WHEN** Provider 执行 deploy
- **THEN** Provider 返回冲突且不修改该资源

### Requirement: [ERP-004] 幂等与 fencing
Provider MUST 在 EdgeApplication 副作用边界校验幂等键和 fencing token；相同标识的重放 SHALL 返回同一资源结果，不同 token 的覆盖 SHALL 被拒绝。

**Traceability:** KRP-003, RDI-003, RDI-004

#### Scenario: 相同请求重放
- **GIVEN** EdgeApplication 已由相同幂等键和 fencing token 创建
- **WHEN** Provider 再次收到请求
- **THEN** Provider 不重复创建并返回已有资源

#### Scenario: 陈旧 token 覆盖
- **GIVEN** EdgeApplication 标记了不同 fencing token
- **WHEN** Provider 尝试部署或删除
- **THEN** Provider 返回 FENCED

### Requirement: [ERP-005] 边缘节点组路由
Provider SHALL 将 Step 中的 `node_group_affinity` 字段映射为 EdgeApplication 的 `spec.nodeSelector` 或 KubeEdge 节点组标签。

**Traceability:** EDGE-04, OP-006

#### Scenario: 指定节点组部署
- **GIVEN** Step 指定了 `node_group_affinity: ["group-a"]`
- **WHEN** Provider 创建 EdgeApplication
- **THEN** EdgeApplication 的 nodeSelector 匹配 group-a 节点
- **AND** 应用仅调度到该节点组

### Requirement: [ERP-006] 可 fencing 的逻辑删除
Provider SHALL 要求 delete 输入匹配目标 UID，SHALL 以更高 generation 将 EdgeApplication 缩容为零并保留墓碑，SHALL NOT 物理删除作为 fence 的 EdgeApplication。

**Traceability:** KRP-006, OP-008

#### Scenario: 延迟 deploy 在删除后恢复
- **GIVEN** 更高 generation 已提交逻辑删除墓碑
- **WHEN** 较低 generation 的 deploy 延迟到达
- **THEN** Provider 返回 FENCED
- **AND** EdgeApplication 不会被重新创建

### Requirement: [ERP-007] 断连自治与重连对账
Provider SHALL 在 CloudCore 可用时正常执行操作；CloudCore 不可用时 SHALL 返回 TARGET_UNAVAILABLE。重连后 SHALL 不自动重试已失败的 Operation。

**Traceability:** EDGE-03, RDI-003

#### Scenario: CloudCore 断连
- **GIVEN** CloudCore 不可达
- **WHEN** Worker 调用 Provider
- **THEN** Provider 返回 TARGET_UNAVAILABLE
- **AND** Worker 按既有重试策略处理