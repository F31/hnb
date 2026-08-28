## ADDED Requirements

### Requirement: [K3S-001] 发行版识别
平台 SHALL 在 KubernetesTarget 注册时自动检测目标集群的发行版类型（standard/k3s/kubeedge/other），并记录在 `distribution` 字段中。

**Traceability:** RT-001, RT-003

#### Scenario: 注册 K3s 集群
- **GIVEN** 目标集群运行 K3s
- **WHEN** Agent 探测集群版本和节点信息
- **THEN** 平台将 distribution 标记为 k3s
- **AND** 记录 K3s 版本和内置组件列表

### Requirement: [K3S-002] 兼容性标注
平台 SHALL 为 K3s 集群标注已知兼容性和限制，包括但不限于：无 etcd（SQLite/embedded 替代）、内置 Traefik Ingress Controller、local-path-provisioner 存储类。

**Traceability:** RT-004

#### Scenario: 查看 K3s 兼容性
- **GIVEN** KubernetesTarget 的 distribution 为 k3s
- **WHEN** 用户查看目标详情
- **THEN** 平台显示 K3s 特定的兼容性标注和已知限制

### Requirement: [K3S-003] 无缝执行
K3s 集群上的 Operation 执行 SHALL 与标准 KubernetesTarget 使用相同路径，无需额外 Provider 或适配层。

**Traceability:** KRP-001, KRP-004

#### Scenario: 对 K3s 执行 deploy
- **GIVEN** KubernetesTarget 的 distribution 为 k3s
- **WHEN** Worker 通过 Kubernetes Provider 执行 deploy
- **THEN** Provider 正常创建 Deployment 并等待 Available
- **AND** 不需要特化逻辑