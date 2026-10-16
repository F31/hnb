## ADDED Requirements

### Requirement: [OBS-009] in-cluster 数据面的故障演练与备份责任
当数据面背靠为 `in-cluster-ha` 时，平台 SHALL 自担 PostgreSQL 与 JetStream 的故障演练（单 Pod、单节点、数据库主实例、消息节点）与版本化备份/可执行恢复，并 SHALL 出具 RPO/RTO 实测；该档位 SHALL NOT 在缺少这些证据时被标记为已认证。

**Traceability:** OBS-003, OBS-004, ICHB-005; in-cluster-ha-backings

#### Scenario: 数据库主实例故障演练
- **GIVEN** 一个 `in-cluster-ha` 部署
- **WHEN** 触发 PostgreSQL 主实例故障
- **THEN** 复制拓扑自动完成故障转移
- **AND** 各平面数据无丢失且告警在档位目标内

#### Scenario: 消息节点故障演练
- **GIVEN** 一个 `in-cluster-ha` 部署的 3 节点 JetStream
- **WHEN** 触发其中一个消息节点故障
- **THEN** 流仍保持可读写且 quorum 满足
- **AND** 恢复包含 RPO/RTO 实测

#### Scenario: 缺证据不得标记认证
- **GIVEN** 一个 `in-cluster-ha` 档位缺少上述演练或备份恢复证据
- **WHEN** 评估其能力证据
- **THEN** 该档位保持未认证
