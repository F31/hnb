## ADDED Requirements

### Requirement: [RT-006] EdgeRuntimeTarget 具体注册
EdgeRuntimeTarget SHALL 支持注册 KubeEdge 集群，包含 CloudCore endpoint、节点组映射和 KubeEdge 版本。平台 SHALL 通过 CloudCore 代理发现边缘节点状态，不直接连接 EdgeCore。

**Traceability:** EDGE-02, ERT-001, ERT-002

#### Scenario: 注册 KubeEdge 集群
- **GIVEN** KubeEdge 集群已部署 CloudCore
- **WHEN** 用户提供 CloudCore endpoint 注册 EdgeRuntimeTarget
- **THEN** 平台记录 CloudCore endpoint 并开始探测
- **AND** 平台发现边缘节点列表和状态

### Requirement: [RT-007] KubeEdge 隧道集成
KubeEdge 节点 SHALL 使用 CloudHub–EdgeHub 隧道与中心通信，平台 SHALL NOT 在 KubeEdge 节点上部署 HNB Agent。平台通过 CloudCore API 获取节点状态和事件。

**Traceability:** EDGE-02, RT-002

#### Scenario: 通过 CloudCore 查询边缘节点
- **GIVEN** 边缘节点通过 CloudHub–EdgeHub 隧道连接
- **WHEN** 平台查询节点状态
- **THEN** 平台通过 CloudCore API 获取节点信息
- **AND** 不直接连接边缘节点