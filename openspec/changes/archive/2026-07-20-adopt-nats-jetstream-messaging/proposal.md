## Metadata

- Change ID: `adopt-nats-jetstream-messaging`
- Tier: T1
- Planes: market, runtime-governance, ai-extension
- Affected Specs: new `contracts-events/CONTRACT-005`, new `composition-operation/OP-007`, new `deployment-governance/GOV-008`
- Depends On: `bootstrap-openspec-governance`, `bootstrap-contracts-events`, `bootstrap-operation-engine`
- Target Milestone: MVP
- Risk: medium

## Why

HNB Cloud 的部署、升级、备份、恢复、供应链检查和边缘对账都是可跨进程、跨 Provider 且持续数分钟至数天的 Operation。仅靠同步调用或数据库临时轮询会耦合 Market、Platform、Worker、Projector 和通知组件，并削弱进度反馈、故障恢复和事件扇出能力；平台需要一个轻量、持久、可重放的内部异步消息骨干，同时保持 PostgreSQL Operation Store 为唯一权威状态。

## What Changes

- 将 NATS JetStream 选为 MVP 默认内部命令与领域事件骨干，Core NATS 不承载需要可靠投递的任务。
- 采用 Transactional Outbox，业务事实和待发布事件在同一数据库事务中提交。
- Operation、Step、Checkpoint、Idempotency、Lease 和 Approval 保存在权威 Operation Store；JetStream 仅负责唤醒、分发、广播和短期重放。
- Worker 按至少一次投递设计，在执行前读取权威状态并获取 Lease，在持久化结果后 ACK。
- 为命令、领域事件和非权威通知定义版本化 Subject、Schema、消费者和保留策略。
- Minimal 使用单节点文件持久化并明确非 HA；Lite HA 及以上使用至少三节点 JetStream 集群和故障恢复演练。
- NATS 故障时 API 可持久化 Operation 和 Outbox，Operation 保持 Queued；恢复后继续投递，不丢失已提交业务事实。
- 消息只携带资源标识、关联信息和 Payload Reference，不携带明文 Secret、kubeconfig 或大文件正文。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `contracts-events`: 增加内部异步消息的持久化投递、确认、重放、背压、失败隔离和消息数据边界。
- `composition-operation`: 增加 Operation Store 与消息骨干的权威边界，以及消息故障和重复投递下的执行语义。
- `deployment-governance`: 增加消息基础设施在 Minimal、Lite HA、Standard HA 和 Enterprise 档位中的 BOM、可用性与恢复要求。

## Non-Goals

- 不使用 NATS 代替 Operation 状态机、工作流引擎、业务数据库或审计事实源。
- 不承诺端到端 exactly-once；平台通过至少一次投递和业务幂等获得一次业务效果。
- 不通过 NATS 传输镜像、Chart、备份、日志包、SBOM、模型或其他大文件。
- 不让 Portal、CLI、租户应用或外部 Provider 直接访问内部 NATS 集群。
- 不在本 change 引入 Kafka、RabbitMQ、Temporal、Redis/Valkey Queue 或第二套生产消息路径。

## Impact

- **代码:** Platform API、Operation Worker、Outbox Relay、Projector、Audit/Notification Consumer 和内部消息契约。
- **API/事件:** 新增版本化内部 Subject 与消息 Envelope；公共 HTTP API 保持兼容。
- **数据:** 新增或扩展 Outbox、Consumer Checkpoint、Worker Lease 和幂等记录；不把业务权威状态迁入 NATS。
- **依赖:** 新增 NATS JetStream 及 Go 客户端；版本、镜像 digest、配置 Schema 和兼容矩阵进入 Infrastructure BOM。
- **资源:** Minimal 增加一个 NATS 实例和持久卷；HA 档位增加至少三个实例及独立持久卷，容量按消息速率、保留期和副本数核算。
- **运维:** 增加安装、升级、备份/恢复、扩缩容、证书轮换、流量控制、Consumer Lag 和存储容量 Runbook。

## Compatibility and Migration

- 先保留 PostgreSQL Operation Store 和现有状态机，再用 Outbox Relay 接入 JetStream；迁移不改变已有 Operation ID 和状态语义。
- 消息 Envelope 与 Subject 使用版本后缀；同一主版本保持兼容，消费者先升级后生产者启用新字段。
- 未发布的 Outbox 记录在切换期间保留；发布成功后按去重键安全重放。
- 不长期维护数据库轮询和 JetStream 两套生产执行路径；完成迁移和回滚演练后移除旧调度入口。

## Security and Isolation

- NATS 使用 mTLS、最小 Subject ACL、独立服务账号和凭据轮换；Portal 与租户网络不可达。
- 消息 Envelope 包含 Tenant ID、Operation ID、Step ID、Resource ID、Correlation ID、IdempotencyKey 和 Schema Version，但不得包含明文 Secret。
- 审计记录消息发布、消费、重试、死信和管理操作；NATS 管理权限与业务发布/消费权限分离。
- 大文件和敏感数据仅通过 OCI/S3/Secret Provider 的受控引用访问。

## Reliability and Operations

- Transactional Outbox 确保业务提交与待发布事件一致；Outbox Relay 可重试且发布幂等。
- Durable Consumer 使用显式 ACK、AckWait、MaxDeliver、背压和失败隔离策略；消费者必须幂等。
- JetStream 不可用时 API 仍可写入 Operation 和 Outbox，执行延迟可观测；恢复后自动续投。
- 监控 Publish/ACK 延迟、Consumer Lag、Redelivery、Outbox Age、存储使用率、Leader 变化和 Operation Queued 时长。
- Minimal 明确单点风险；HA 档位验证单 Pod、单节点和 Leader 故障，记录 RTO 与消息恢复结果。

## Rollout and Rollback

1. 在非生产环境部署 JetStream，创建版本化 Stream、Subject、Consumer 和 ACL。
2. 启用 Outbox Relay 和影子消费者，验证消息一致性、重复投递和重放，不驱动真实写操作。
3. 逐类切换 Projector、通知和 Operation 命令消费者，并观察 Lag 与错误率。
4. 完成故障注入后关闭旧数据库调度入口，JetStream 成为唯一消息传输实现。
5. 回滚时停止新消息生产和消费，等待执行中 Step 达到安全点，再恢复旧调度入口；Operation Store 和 Outbox 不回滚、不删除。

## Exit Criteria

- **GIVEN** API 已提交 Operation 但 JetStream 不可用，**WHEN** JetStream 恢复，**THEN** Outbox 最终发布且 Operation 从 Queued 继续执行，不丢失业务事实。
- **GIVEN** 同一 Step 命令被重复投递，**WHEN** Worker 处理全部副本，**THEN** 只产生一次业务效果且重复消息被记录和 ACK。
- **GIVEN** Worker 在外部调用后、ACK 前崩溃，**WHEN** 消息重新投递，**THEN** Worker 从权威状态和检查点恢复，不重复创建资源。
- **GIVEN** Lite HA JetStream 集群失去一个 Pod 或节点，**WHEN** 执行故障演练，**THEN** 已确认消息不丢失且消息处理在批准 RTO 内恢复。
- **GIVEN** 消息包含明文 Secret 或超过批准大小，**WHEN** 发布门禁执行，**THEN** 消息被拒绝并产生安全审计。
