## Context

HNB Cloud 已定义持久化 Operation 状态机、ExecutionPlan DAG、幂等恢复、事务 Outbox 和跨平面事件契约，但尚未冻结内部消息实现。部署、升级、备份恢复、制品门禁、Provider 调用、边缘离线对账和审批等待都可能持续较长时间，并由 Market、Platform、Worker、Projector、Audit 和 Notification 等独立组件协作完成。

设计受以下约束：PostgreSQL Operation Store 保存唯一业务事实；消息系统故障不能丢失已提交 Operation；Portal 不直接访问内部消息系统；Minimal 保持轻量；HA 档位必须可演练；消息不得成为 Secret 或大文件数据面。利益相关方包括平台开发、SRE、安全、数据库与中间件运维、Provider 开发者和 Portal 用户。

### Architecture

```text
Portal / CLI
     |
     v
Platform API
     |
     | one database transaction
     v
+-----------------------------------------+
| PostgreSQL Operation Store              |
| Operation / Step / Checkpoint / Lease   |
| Idempotency / Approval / Outbox         |
+-----------------------------------------+
                   |
              Outbox Relay
                   |
                   v
+-----------------------------------------+
| NATS JetStream                          |
| commands / domain events / notification |
+-----------------------------------------+
       |              |              |
       v              v              v
 Operation Worker  Projector   Audit / Notification
       |
       v
 Provider / RuntimeTarget
```

跨平面只共享版本化消息 Schema，不共享数据库。Market、AI 和 Edge 只能提交 Operation 或发布已提交领域事实，不能通过 NATS 建立目标写入旁路。制品和备份正文仍由 OCI/S3 数据面传输。

## Goals / Non-Goals

**Goals:**

- 用一个轻量消息骨干统一内部命令分发、领域事件扇出和短期重放。
- 保持 Operation Store 为唯一权威，并在消息重复、延迟、乱序和服务重启时获得一次业务效果。
- 通过 Transactional Outbox 消除数据库提交与事件发布之间的双写窗口。
- 为 Portal 提供低延迟进度投影，同时不让 Portal 依赖或暴露 NATS。
- 为 Minimal 到 Enterprise 提供一致协议和不同可用性档位。
- 满足 CONTRACT-005、OP-007 和 GOV-008 的故障、升级及安全验收。

**Non-Goals:**

- 不使用 NATS 实现新的业务状态机、审批引擎或永久审计库。
- 不追求传输层 exactly-once，不让 ACK 代替业务幂等。
- 不通过消息传输大文件、日志正文、模型、备份或 Secret。
- 不引入 Kafka、RabbitMQ、Temporal 或 Valkey Queue 作为并行生产路径。
- 不改变 Provider/RuntimeTarget 生命周期契约和 Operation 唯一写入口。

## Decisions

### Decision 1: 使用 NATS JetStream 作为 T1 默认内部消息骨干

可靠命令和领域事件使用 JetStream 的文件持久化、Durable Consumer、显式 ACK、重投和重放；Core NATS 只可用于允许丢失的临时通知，不承载 Operation 命令或权威领域事件。NATS 与 Go 控制面契合，部署和资源成本低于 Kafka，且比仅使用 PostgreSQL 队列更适合跨平面扇出和独立消费者。

版本不使用浮动 `latest`。实现 change 必须在 Infrastructure BOM 固定 NATS Server、Go Client、Chart、配置 Schema 和镜像 digest，并验证升级兼容。

### Decision 2: PostgreSQL 保存事实，JetStream 传递意图和事实

API 在同一事务内写入 Operation/业务数据和 Outbox。Outbox Relay 使用稳定 Message ID 发布，收到 JetStream 持久化确认后标记发送完成。Worker 收到命令后读取 Operation Store、校验状态和期望版本、获取带期限 Lease，再调用 Provider。Worker 在数据库事务内保存 Step 结果、Checkpoint 和后续 Outbox，提交后才 ACK。

NATS 不可用时，API 可继续接受符合容量策略的请求，Operation 保持 `Queued`；Outbox Age 超过 SLO 时告警。NATS 恢复后 Relay 续投。该设计不增加数据库共享：只有所属服务访问自身数据库，跨平面事实通过消息契约交换。

### Decision 3: 命令、领域事件和通知分流

Subject 使用 `hnb.<class>.<domain>.<type>.v<major>`：

```text
hnb.command.operation.step-requested.v1
hnb.event.operation.step-completed.v1
hnb.event.operation.state-changed.v1
hnb.notification.operation.progress.v1
```

- Command Stream 使用 WorkQueue 类消费语义，同一命令由一个 Worker 组竞争处理。
- Domain Event Stream 按时间与容量限制保留，Projector、Audit 和 Notification 分别使用 Durable Consumer。
- Notification 不是业务事实，可使用较短保留期；Portal 仅通过 Platform API 的 SSE/WebSocket 读取已授权进度。
- 失败消息在达到 MaxDeliver 后发布到版本化 failed Subject，并保留原消息标识、失败分类和关联 ID；不得复制 Secret。

全局顺序不作保证。需要顺序的操作以 Operation/Resource 为并发边界，由期望版本、数据库 Lease 和状态机拒绝陈旧消息；不会依赖无限 Subject 分区获得正确性。

### Decision 4: 消息 Envelope Schema First

```text
MessageEnvelope
- messageId: UUID
- messageType: versioned string
- schemaVersion: semantic version
- occurredAt: timestamp
- tenantId: identifier
- projectId: optional identifier
- operationId: optional identifier
- stepId: optional identifier
- resourceId: optional identifier
- correlationId: identifier
- causationId: optional message identifier
- idempotencyKey: string
- expectedVersion: optional integer
- payloadRef: optional immutable reference
- payload: bounded typed message
```

Envelope 与 payload 先定义 Protobuf/JSON Schema，再生成 Go SDK。消息大小上限由 BOM 配置和契约测试固定；超限内容写入 OCI/S3 后只发送 digest 与短期授权所需引用。Envelope 禁止 Secret、kubeconfig、访问 Token 和未脱敏日志。

### Decision 5: 消费者按至少一次和租约执行

Consumer 使用 Pull 模式控制并发与背压。每类命令配置 AckWait、MaxAckPending、MaxDeliver、退避和处理超时。Worker 延长数据库 Lease 与消息处理中状态；失去 Lease 后停止产生新副作用。ACK 只表示消息处理结果已持久化，不表示外部资源天然 exactly-once。

Provider 命令继续携带 IdempotencyKey 和期望版本。对无法原子确认的外部调用，Worker 先 Observe，再根据 Checkpoint 决定继续、补偿或人工处理。

### Decision 6: 按部署档位交付同一实现

| 档位 | JetStream 拓扑 | 持久化与验收 |
|---|---|---|
| Development | 单节点 | 本地文件，可重建，不承诺可用性 |
| Minimal | 单节点 | PVC，明确非 HA；验证重启恢复和容量上限 |
| Lite HA | 至少 3 节点 | 3 副本、反亲和；验证单 Pod/节点/Leader 故障 |
| Standard HA | 至少 3 节点 | 独立 PVC、容量预留、升级与恢复演练 |
| Enterprise | 奇数节点、多故障域 | 严格 SLO、证书轮换、容量隔离和灾备 Runbook |

Minimal 不安装 Kafka 或第二消息系统。HA 拓扑、存储类别和副本数进入 Infrastructure BOM，不由业务代码写死。

## Data Model

```text
Operation
- id, tenant_id, state, version, execution_plan_digest
- created_at, updated_at, terminal_at

OperationStep
- id, operation_id, state, attempt, idempotency_key
- checkpoint, timeout_at, last_error

WorkerLease
- step_id, owner_id, fencing_token, expires_at

OutboxEvent
- id, aggregate_type, aggregate_id, aggregate_version
- message_type, schema_version, payload_or_ref
- occurred_at, published_at, attempt, next_attempt_at

ConsumerCheckpoint
- consumer_name, stream, sequence, processed_at
```

Outbox 与业务事实同库同事务，但 Market 与 Platform 各自拥有自己的 Outbox，不相互读取。`fencing_token` 防止过期 Worker 在新 Worker 获取 Lease 后继续写结果。ConsumerCheckpoint 用于投影和审计重放证据，不代替 JetStream Consumer 状态。

## API and Event Contracts

- 公共写 API 返回 Operation ID；同步响应不等待长任务完成。
- Portal 通过租户鉴权后的 SSE/WebSocket 或轮询查询 Read Model，不连接 NATS。
- 内部消息 Subject、Envelope、payload、Header、权限和保留策略均版本化。
- 生产者必须设置 Message ID、Correlation ID、Causation ID 和 IdempotencyKey。
- 消费者忽略未知可选字段；破坏性字段变化创建新主版本 Subject 并经过双读/双写迁移窗口。
- NATS Request/Reply 只用于短时、可失败的内部查询；不得承载需要持久恢复的 Operation Step。

## State Machine

```text
API transaction
  -> Operation Queued + Outbox Pending
  -> Relay PublishPending
  -> JetStream Persisted
  -> Outbox Published
  -> Worker Received
  -> Lease Acquired
  -> Operation InProgress
  -> Step Checkpointed
  -> Result + Outbox committed
  -> Message ACK
  -> Projector updates Read Model

Broker unavailable: Outbox Pending -> retry -> publish after recovery
Worker failure: unacked -> redelivery -> lease/checkpoint recovery
Repeated failure: retry budget exhausted -> failed subject -> Operation Paused/Failed
Terminal Operation + stale command: no-op -> audit -> ACK
```

## Security and Isolation

- **租户隔离:** NATS 不作为租户直连服务；服务端根据业务授权写入 Tenant ID，消费者再次从权威资源验证租户。按服务账号和 Subject ACL 隔离生产、消费和管理权限。
- **Secret:** 使用 mTLS 和短期凭据；消息、Header、错误、failed Subject 和观测数据不得包含明文 Secret。Secret 只以 SecretReference 传递。
- **供应链:** NATS Server、客户端和 Chart 固定版本与 digest，生成 SBOM并通过漏洞、许可证、签名门禁。
- **权限:** NATS 管理、Stream 管理、业务发布和消费权限分离；默认拒绝通配发布和跨域消费。
- **审计:** 记录 Stream/Consumer 配置变更、凭据轮换、发布失败、重投、failed Subject、重放和删除操作。
- **数据边界:** 不共享 Market、Platform、AI 或 Edge 数据库；不经 NATS 代理 OCI/S3 数据面；只有 Operation Worker 能按计划调用目标写 API。

## Performance, Capacity, and Observability

- 性能预算分别测量 API 提交、Outbox Publish、JetStream ACK、Consumer Lag、端到端投影和 Operation 调度 P95/P99。
- 容量模型至少包含峰值消息率、平均/最大消息大小、事件保留期、Consumer 数、重投率和副本因子。
- 监控 NATS 可用性、Leader 变化、Stream Bytes/Messages、Publish/ACK 错误、Consumer Pending/Lag、Redelivery、Outbox Age、failed Subject 数量和 Queued Operation 时长。
- Correlation ID、Operation ID、Step ID 和 Message ID 贯穿日志、链路和指标；消息正文不进入常规日志。
- 背压时先限制低优先级通知，再限制新 Operation 提交；不得无限增加内存队列。

## Compatibility and Conformance

| 组合 | 支持要求 |
|---|---|
| Platform API ↔ Outbox Relay | 相同 Envelope 主版本；数据库 Schema 兼容 |
| Outbox Relay ↔ NATS Server | BOM 认证的 Server/Go Client 组合 |
| NATS Server ↔ Durable Consumer | Stream/Consumer 配置 Schema 与消息主版本兼容 |
| Worker ↔ Provider | 继续遵守 Provider 契约、幂等键和期望版本 |
| Projector ↔ Event Stream | 支持批准保留窗口内的事件重放和未知可选字段 |

Conformance Harness 覆盖重复投递、乱序、ACK 丢失、Worker 崩溃、Relay 崩溃、Broker 停机恢复、Leader 切换、failed Subject、Schema 兼容、超限消息和 Secret 检测。该 change 不改变 Gateway、RuntimeTarget 或 Edge Provider 接口；相关兼容矩阵为 N/A，但边缘离线消息仍通过统一 Operation 状态机验证。

## Failure Modes

- `[NATS 不可用]` -> Operation 与 Outbox 继续持久化到容量上限，保持 Queued并告警，恢复后续投。
- `[数据库不可用]` -> 不接受新的权威写入；Worker 不根据消息猜测状态或直接执行。
- `[Relay 发布后、标记前崩溃]` -> 使用稳定 Message ID 重发，消费者幂等处理。
- `[Worker 外部调用后崩溃]` -> 重新投递后通过 Lease、fencing token、Checkpoint 和 Observe 恢复。
- `[消息乱序]` -> 期望版本和状态机拒绝陈旧迁移，记录 stale delivery。
- `[Consumer 持续失败]` -> 有限重试后进入 failed Subject，Operation 暂停或失败并升级人工处理。
- `[磁盘接近容量]` -> 告警并执行保留/限流策略，不静默删除未确认命令或权威审计。
- `[集群失去多数派]` -> 停止需要持久确认的新消息，Outbox 保留；恢复多数派后继续。

## Risks / Trade-offs

- `[增加一个有状态基础设施]` -> Minimal 使用单节点轻量部署，HA 复用同一配置模型并提供自动化 Runbook。
- `[至少一次导致重复]` -> 所有命令使用业务幂等、Lease、fencing token 和期望版本，不依赖传输层 exactly-once。
- `[NATS 配置错误导致消息丢失或积压]` -> Stream/Consumer 配置进入版本化 BOM和契约测试，禁止运行时手工漂移。
- `[JetStream 被误当成业务数据库]` -> 代码与评审门禁要求 Worker始终读取 Operation Store，事件保留期不承担永久审计。
- `[单节点 Minimal 存在单点]` -> UI、安装报告和文档明确限制；生产 HA 需求必须选择 Lite HA 及以上。
- `[跨服务 Schema 演进复杂]` -> Schema First、生成 SDK、主版本 Subject 和兼容窗口。

## Alternatives Considered

- **仅 PostgreSQL 队列:** 阶段 0 原型简单，但跨平面扇出、独立消费进度和重放会增加数据库负载与自研逻辑；保留为迁移前基线，不作为最终生产双路径。
- **RabbitMQ:** 可靠队列成熟，但 HNB 同时需要轻量命令、领域事件扇出和短期重放，JetStream 的统一模型和 Go 集成更合适。
- **Kafka:** 高吞吐与长期事件流能力强，但资源和运维复杂度不符合 Minimal/MVP。
- **Temporal:** 可管理长工作流，但与已有 ExecutionPlan/Operation 状态机、审计和 Provider 语义重叠，引入成本过高。
- **Valkey/Redis Queue:** 部署轻，但可靠消费、事件重放和跨平面消息治理不如 JetStream 完整。

## Migration Plan

1. 固定 NATS、Go Client 和 Chart BOM，部署非生产 JetStream 与 mTLS/ACL。
2. 创建版本化 Stream、Subject、Durable Consumer、保留和容量策略。
3. 增加 Outbox Schema、Relay、Envelope SDK 及契约测试。
4. 以影子模式发布事件并重建测试 Read Model，校验顺序、重复和数据一致性。
5. 先迁移 Projector/Audit/Notification，再迁移不产生外部副作用的命令，最后迁移 Operation Step。
6. 执行 Broker、Relay、Worker、数据库和网络故障注入以及升级回滚演练。
7. 关闭旧数据库调度入口，保留观测窗口后删除旧代码，不删除 Operation/Outbox 事实。

回滚时暂停新消息生产，等待执行中 Step 到达安全点，记录未确认消息序列，恢复旧调度入口并从 Operation Store 对账。NATS Stream 只读保留到审计和对账完成；任何回滚不得直接删除 Operation、Checkpoint 或未发布 Outbox。

## Upgrade, Rollback, and Disaster Recovery

- NATS Server 和客户端升级先通过存储格式、滚动升级、降级兼容和消息重放测试。
- HA 档位按一次一个节点滚动升级并保持多数派；失败立即停止后续节点升级。
- JetStream 备份与恢复保护消息传输连续性，Operation Store 备份仍是业务恢复权威。
- 灾难恢复后先恢复数据库事实，再恢复 JetStream 配置与必要消息，并通过 Outbox 和 Operation 对账补发。
- RPO/RTO 必须绑定部署档位、版本、消息负载和演练证据，不以副本数代替恢复验证。

## Open Questions

- 首个认证 NATS Server、Go Client 和 Helm Chart 的精确版本及升级窗口是什么？
- Minimal 和 Lite HA 的默认消息大小、保留期、磁盘预算、AckWait 和 MaxDeliver 分别是多少？
- Platform 与 Market 各自运行 Outbox Relay，还是使用同一镜像的独立实例和权限？
- failed Subject 的人工处置入口、保留期限和重新驱动审批策略如何定义？
