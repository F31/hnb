## 1. BOM and Contracts

- [ ] 1.1 `[GOV-008]` 评估并固定 NATS Server、Go Client、Helm Chart、镜像 digest、配置 Schema 和支持窗口；证据：Infrastructure BOM 与兼容矩阵评审记录。
- [ ] 1.2 `[CONTRACT-005]` 定义版本化 Subject、MessageEnvelope、命令/事件 payload、消息大小和禁止字段 Schema，并生成 Go SDK；证据：Schema 兼容检查和生成物可重复性测试。
- [ ] 1.3 `[CONTRACT-005]` 定义 Stream、Durable Consumer、ACK、AckWait、MaxDeliver、退避、保留、failed Subject 和重放配置；证据：配置快照和自动化契约测试。
- [ ] 1.4 `[CONTRACT-005]` 记录公共 HTTP API、Provider API 和 RuntimeTarget API 无破坏性变化，数据库迁移仅涉及内部 Operation/Outbox Schema；证据：API diff 和 N/A 评审记录。

## 2. Persistent State and Outbox

- [ ] 2.1 `[OP-007]` 添加或扩展 OutboxEvent、WorkerLease、fencing token、ConsumerCheckpoint 和幂等记录的版本化数据库迁移；证据：空库升级、存量升级和降级测试。
- [ ] 2.2 `[OP-007][CONTRACT-005]` 实现业务事实与 Outbox 同事务提交、稳定 Message ID 和可重试发布状态；证据：事务回滚、并发和重复提交单元测试。
- [ ] 2.3 `[OP-007][CONTRACT-005]` 实现 Outbox Relay 发布确认、指数退避、积压限流和安全停机恢复；证据：发布后标记前崩溃及恢复集成测试。
- [ ] 2.4 `[OP-007]` 实现 Worker Lease、fencing token、Checkpoint 和终态陈旧消息 no-op；证据：租约过期、并发 Worker 和陈旧命令测试。

## 3. JetStream Infrastructure

- [ ] 3.1 `[GOV-008]` 交付 Development/Minimal 单节点文件持久化部署与 PVC、资源上限、mTLS 和健康检查；证据：安装、重启恢复和卸载测试。
- [ ] 3.2 `[GOV-008]` 交付 Lite HA 及以上三节点拓扑、反亲和、PodDisruptionBudget、持久卷和多数派配置；证据：渲染后的 BOM、拓扑检查和容量报告。
- [ ] 3.3 `[CONTRACT-005]` 配置按服务账号隔离的 Subject ACL、凭据轮换及管理/发布/消费权限分离；证据：允许与拒绝访问的安全集成测试。
- [ ] 3.4 `[CONTRACT-005]` 实现消息大小、Secret/kubeconfig/Token 禁止字段和 Payload Reference 门禁；证据：敏感数据与超限消息拒绝测试。

## 4. Producers and Consumers

- [ ] 4.1 `[OP-007]` 将 Operation Step 调度迁移为 JetStream Durable Pull Consumer，执行前读取权威状态并获取 Lease，提交结果后 ACK；证据：正常执行和重复投递集成测试。
- [ ] 4.2 `[CONTRACT-005]` 将 Projector、Audit 和 Notification 拆分为独立 Durable Consumer，验证事件扇出和独立消费进度；证据：多消费者及消费者重启测试。
- [ ] 4.3 `[CONTRACT-005]` 实现有限重试、failed Subject、人工重新驱动入口和审批检查；证据：毒消息、重试耗尽和安全重驱测试。
- [ ] 4.4 `[OP-007]` 保持 Portal 只通过鉴权后的 API SSE/WebSocket 或查询 Read Model 获取进度，不直接连接 NATS；证据：网络策略、租户越权和实时进度 E2E 测试。

## 5. Reliability and Conformance

- [ ] 5.1 `[CONTRACT-005][OP-007]` 覆盖 ACK 丢失、重复、乱序、Relay 崩溃、Worker 崩溃、Broker 停机和数据库停机的 Conformance Harness；证据：故障矩阵报告。
- [ ] 5.2 `[OP-007]` 验证 NATS 不可用时 Operation/Outbox 保持 Queued、恢复后自动续投且不产生重复业务效果；证据：端到端故障注入报告。
- [ ] 5.3 `[GOV-008]` 在 Lite HA 演练单 Pod、单节点和 Leader 故障，记录消息丢失、Consumer Lag 和 RTO；证据：HA 演练报告。
- [ ] 5.4 `[CONTRACT-005]` 验证事件版本兼容、批准窗口内重放和 Projector 重建不会触发目标写操作；证据：契约兼容和重放测试报告。

## 6. Observability and Performance

- [ ] 6.1 `[CONTRACT-005][OP-007]` 暴露 Publish/ACK 延迟、Consumer Lag、Redelivery、Outbox Age、failed Subject、Queued 时长和 Stream 容量指标；证据：仪表盘、告警和示例链路。
- [ ] 6.2 `[CONTRACT-005]` 验证 Message ID、Correlation ID、Operation ID 和 Step ID 全链路关联且日志不包含消息 Secret；证据：追踪查询和敏感信息扫描报告。
- [ ] 6.3 `[GOV-008]` 测量各档位峰值消息率、P95/P99、平均/最大消息大小、保留期、磁盘和副本容量，固定默认预算；证据：绑定版本与环境的压测报告。

## 7. Upgrade, Rollback, and Documentation

- [ ] 7.1 `[GOV-008]` 验证 NATS Server、Go Client、Chart 和持久格式的滚动升级与回滚顺序；证据：升级/降级演练报告和兼容矩阵。
- [ ] 7.2 `[GOV-008][OP-007]` 编写安装、证书轮换、容量扩展、积压处理、failed Subject、备份恢复、灾难恢复和卸载 Runbook；证据：文档评审及桌面演练记录。
- [ ] 7.3 `[OP-007]` 以影子事件、只读消费者、低风险命令和 Operation Step 的顺序完成灰度迁移，并在安全点验证旧调度入口回滚；证据：灰度和回滚记录。
- [ ] 7.4 `[CONTRACT-005][OP-007][GOV-008]` 执行完整 E2E：提交长 Operation、实时查看进度、注入故障、恢复、重放、升级和回滚；证据：E2E 报告及 Requirement 映射。
- [ ] 7.5 `[CONTRACT-005][OP-007][GOV-008]` 运行 `openspec validate --all --strict`、完成 verify 并同步规格后归档；证据：零阻断校验、verify 报告和 sync/archive 记录。
