## ADDED Requirements

### Requirement: [ERT-001] EdgeRuntimeTarget 注册
平台 SHALL 支持将 KubeEdge 集群注册为 EdgeRuntimeTarget，包含 CloudCore endpoint、租户命名空间映射和节点组列表。

**Traceability:** EDGE-02, RT-001, RT-002

#### Scenario: 注册 KubeEdge 集群
- **GIVEN** 管理员拥有 EdgeRuntimeTarget 创建权限
- **WHEN** 提交 CloudCore endpoint 和节点组定义
- **THEN** 平台创建 EdgeRuntimeTarget 并返回目标 ID
- **AND** 平台开始探测 KubeEdge 版本和节点状态

### Requirement: [ERT-002] 能力发现与版本检测
EdgeRuntimeTarget SHALL 上报 KubeEdge 版本（CloudCore + EdgeCore）、边缘节点数量、架构、资源和离线节点列表。

**Traceability:** RT-003, EDGE-02

#### Scenario: 边缘节点离线
- **GIVEN** EdgeRuntimeTarget 已注册
- **WHEN** 某边缘节点断连超过心跳间隔
- **THEN** 平台将节点状态标记为 Offline 并记录 lastKnownStateAt
- **AND** 该节点进入 QueuedOffline 状态，写操作排队

### Requirement: [ERT-003] 状态新鲜度
EdgeRuntimeTarget 的节点状态 SHALL 包含 observedAt 和 lastKnownStateAt；超过新鲜度阈值时，针对该节点的写操作 SHALL 排队或要求显式风险确认。

**Traceability:** RT-005, EDGE-14

#### Scenario: 陈旧节点执行升级
- **GIVEN** 节点已离线超过策略阈值
- **WHEN** 用户提交针对该节点的升级
- **THEN** 平台显示状态陈旧并按策略进入 QueuedOffline 或拒绝