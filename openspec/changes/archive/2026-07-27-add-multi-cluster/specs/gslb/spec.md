# gslb

## Purpose
定义全局流量负载均衡，基于 DNS 的健康感知流量分发。

## ADDED Requirements

### Requirement: [GSLB-001] 集群健康探测
GSLB Controller SHALL 定期探测成员集群的 API Server 和关键服务健康状态。

**Traceability:** T2

#### Scenario: 健康探测成功
- **GIVEN** 集群 A 运行正常
- **WHEN** GSLB 执行健康探测
- **THEN** 集群 A 标记为 healthy
- **AND** DNS 记录正常解析到集群 A

### Requirement: [GSLB-002] DNS 流量分发
GSLB SHALL 基于健康状态和权重，通过 DNS 将流量分发到多个集群。

**Traceability:** T2

#### Scenario: 故障转移
- **GIVEN** 集群 A 健康探测失败
- **WHEN** GSLB 检测到异常
- **THEN** 集群 A 的 DNS 记录被移除
- **AND** 流量自动转移到集群 B

### Requirement: [GSLB-003] 多集群流量权重
GSLB SHALL 支持按权重分配流量（如 70% 集群 A、30% 集群 B）。

**Traceability:** T2

#### Scenario: 灰度流量
- **GIVEN** 集群 A 权重 70、集群 B 权重 30
- **WHEN** DNS 查询发起
- **THEN** 约 70% 请求解析到集群 A、30% 到集群 B

### Requirement: [GSLB-004] 与 Karmada 集成
GSLB SHALL 读取 Karmada 集群状态作为健康探测数据源之一。

**Traceability:** T2

#### Scenario: Karmada 报告集群不可用
- **GIVEN** Karmada 将成员集群 A 标记为不可用
- **WHEN** GSLB 刷新集群健康状态
- **THEN** GSLB 将该状态纳入集群 A 的流量调度决策
