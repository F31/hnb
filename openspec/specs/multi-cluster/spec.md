# multi-cluster

## Purpose
定义多集群资源注册、心跳、状态聚合和跨集群调度策略，使 HNB 能够通过 Karmada 管理成员集群生命周期和多集群资源分发。

## Requirements

### Requirement: [MC-001] 集群注册表
HNB SHALL 提供集群注册表 API，支持成员集群的注册、更新、摘除和列表查询。

**Traceability:** T2

#### Scenario: 注册新集群
- **GIVEN** 一个合法的 Karmada 成员集群 kubeconfig
- **WHEN** 管理员通过 Platform API 注册集群
- **THEN** 集群信息持久化到 clusters 表，状态为 pending
- **AND** 系统触发连通性验证

### Requirement: [MC-002] 集群心跳与状态聚合
成员集群 SHALL 定期上报心跳（含版本、节点数、容量）；HNB SHALL 聚合心跳生成集群健康状态。

**Traceability:** T2

#### Scenario: 心跳超时
- **GIVEN** 集群已注册且状态为 active
- **WHEN** 超过 maxSilenceSeconds 未收到心跳
- **THEN** 集群状态转为 inactive
- **AND** 触发告警通知

### Requirement: [MC-003] Karmada 集成
HNB SHALL 通过 Karmada API 将资源下发到成员集群，SHALL 支持 PropagationPolicy 策略。

**Traceability:** T2

#### Scenario: 多集群 Deployment
- **GIVEN** 一个 K8s Deployment 需要部署到多个集群
- **WHEN** Operation Worker 执行 deploy 步骤
- **THEN** Karmada Provider 生成 PropagationPolicy 并下发
- **AND** 各成员集群根据调度策略部署资源

### Requirement: [MC-004] 调度策略
HNB SHALL 支持集群选择器（标签/区域/租户）和部署策略（Duplicated/Divide）。

**Traceability:** T2

#### Scenario: 按区域调度
- **GIVEN** 集群带有 region=cn-east 标签
- **WHEN** 用户选择 cn-east 区域部署
- **THEN** 资源仅下发到该区域集群

### Requirement: [MC-005] 跨集群 Operation 追踪
HNB SHALL 在 Operation 中记录目标集群信息，支持按集群过滤 Operation 列表。

**Traceability:** T2

#### Scenario: 查看跨集群 Operation
- **GIVEN** 一个 Operation 涉及 3 个集群
- **WHEN** 用户查看 Operation 详情
- **THEN** 显示每个集群的执行状态和结果
