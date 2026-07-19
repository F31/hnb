## ADDED Requirements

### Requirement: [GOV-008] 异步消息基础设施部署档位
版本化 BOM SHALL 固定内部异步消息实现、镜像摘要、配置 Schema、持久化和兼容矩阵。Minimal MAY 使用明确标记为非 HA 的单节点持久化实例；Lite HA、Standard HA 和 Enterprise SHALL 使用满足多数派容错的奇数节点集群，并分别定义容量、备份恢复、升级回滚和单 Pod、单节点、Leader 故障验收。

**Traceability:** GOV-002, GOV-003, GOV-004, OBS-004

#### Scenario: 安装 Minimal 档位
- **GIVEN** 用户选择 Minimal 且接受控制面消息服务单点风险
- **WHEN** 安装器生成 Infrastructure BOM
- **THEN** 部署一个文件持久化消息实例并显示非 HA 限制
- **AND** 不引入 Kafka 或另一套生产消息路径

#### Scenario: Lite HA 消息节点故障
- **GIVEN** Lite HA 使用至少三个消息节点和多数派复制
- **WHEN** 一个 Pod 或节点停止
- **THEN** 已确认消息不丢失且处理在批准 RTO 内恢复
- **AND** Leader 变化、Consumer Lag 和恢复结果可观测

#### Scenario: 升级消息基础设施
- **GIVEN** 新版本通过消息契约和存储格式兼容测试
- **WHEN** 运维执行滚动升级
- **THEN** 生产者和消费者按兼容顺序升级且未确认消息可继续处理
- **AND** 失败时可回滚到 BOM 锁定版本并保留持久消息
