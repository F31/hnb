## Context

主规格已要求统一遥测、Operation 滞留告警、Provider 超时告警、供应链安全事件和故障演练，但没有告警权威模型、生命周期、路由、降噪及通知渠道契约。`adopt-nats-jetstream-messaging` 已定义内部持久事件与 Notification Consumer，却不负责通知业务状态和外部渠道送达。

本 change 面向平台/租户管理员、运维和值班人员、安全人员、SRE、渠道管理员和 Portal 用户。设计必须保持多租户隔离、SecretReference-only、Minimal 轻量、渠道可替换和通知降噪；监控产品可以替换，不能把某一指标后端或 Alertmanager 写入 Core。

### Architecture

```text
Metrics / Logs / Traces / Domain Events
                  |
      Rule Engine / Source Adapters
                  |
                  v
         Alert Normalizer
                  |
                  v
+-------------------------------------------+
| PostgreSQL Alert Store                    |
| Rule / Instance / Silence / Route         |
| Contact / Channel / Job / Delivery        |
| Transactional Outbox                      |
+-------------------------------------------+
                  |
             NATS JetStream
                  |
        Notification Dispatcher
        |          |          |
        v          v          v
   Portal API    Email      Webhook      SMS Provider
    SSE/WS       Worker      Worker       (T2 optional)
        |
        v
 Vue Alert Center / Message Bell
```

Alert Store 是告警、策略和送达记录的唯一权威。来源系统保留原始指标、日志、链路和安全事件；Alert Store 只保存归一化摘要与引用。NATS 只负责通知任务和领域事件传输。Portal、浏览器和外部 Provider 不连接 NATS，也不访问其他平面数据库。

## Goals / Non-Goals

**Goals:**

- 满足 ALERT-001 至 ALERT-010 和 UX-004，形成检测、归一化、降噪、路由、通知、处置和审计闭环。
- T1 提供 Portal、Email/SMTP 和通用 Webhook，T2 通过 Provider SPI 接入 SMS。
- 准确表达不同渠道的 Accepted、Delivered 和 Read 能力，避免虚假送达状态。
- 以 fingerprint、聚合、抑制、静默和维护窗口控制告警疲劳。
- 复用 PostgreSQL、Transactional Outbox 和 NATS JetStream，不新增强制消息中间件。
- 单一渠道故障不阻塞告警事实、Portal 或其他渠道。

**Non-Goals:**

- 不自研时序数据库、日志存储、链路存储或完整指标规则引擎。
- 不把 Prometheus、Alertmanager、Grafana 或云监控固定为唯一来源。
- 不在 T1 实现专用钉钉、飞书、企业微信、PagerDuty、Opsgenie 或语音渠道。
- 不保证 Email 已阅读；没有渠道回执时不推断 Delivered。
- 不允许告警确认、静默或通知动作直接修改 RuntimeTarget。

## Decisions

### Decision 1: 新建独立 Alert/Notification 领域服务

Alert/Notification 作为运行治理平面的 T1 服务，拥有独立 PostgreSQL Schema/数据库权限和 Outbox。它不编译具体监控、SMTP、Webhook 或 SMS 实现进入 HNB Core。Source Adapter 将标准领域事件或外部规则事件归一化，Channel Worker 独立部署和扩展。

备选方案是把告警逻辑放入 Observability 后端或 Platform API，但会让领域告警依赖具体监控产品，并使外部渠道故障扩大 Platform API 故障面，因此拒绝。

### Decision 2: 来源事件与 AlertInstance 分离

来源事件记录“观察到什么”，AlertInstance 记录去重后的当前告警事实。Normalizer 使用 `tenant + source + resource + rule + labels` 的规范化 fingerprint 查找活动实例，并以期望版本更新 LastSeenAt、计数和状态。恢复事件只能解决匹配活动实例；确认和静默不改变来源健康事实。

原始大日志、指标样本和链路不复制到 Alert Store，只保存摘要、时间范围、查询参数或不可变引用。这样避免 Alert Store 成为第二套可观测数据平台。

### Decision 3: 告警与送达使用两个状态机

```text
AlertInstance
Pending -> Firing -> Acknowledged -> Resolved
                |          |
                -> Silenced-

DeliveryRecord
Pending -> Sending -> Accepted -> Delivered -> Read
             |          |
             |          -> Failed
             -> Failed / Suppressed / Cancelled
```

`Acknowledged` 表示有人响应，不代表故障恢复；`Silenced` 表示通知受抑制，来源事件仍更新实例；`Resolved` 只能由恢复事件、规则评估或经授权人工流程产生。Delivery 状态按 Channel Capability 单调推进，Email 默认最高到 Accepted，Portal 可到 Read，SMS/Webhook 仅在受认证回执存在时到 Delivered。

### Decision 4: 策略先路由再生成独立 DeliveryRecord

Policy Engine 按 Tenant、Project、Environment、Severity、Source、Resource、Label、时间和值班计划计算 ContactGroup、Channel、重复间隔和升级路径。每个渠道生成独立 NotificationJob/DeliveryRecord，一个渠道失败不会回滚其他渠道或 AlertInstance。

Critical 告警始终存在平台批准的默认安全路由。用户偏好只能在组织策略允许范围内收窄非关键通知，不能关闭强制 Critical 路由。升级计时与重试分别建模，避免渠道短暂重试阻塞值班升级。

### Decision 5: T1 渠道与 T2 Provider 分层

| 渠道 | Tier | 送达能力 | 约束 |
|---|---:|---|---|
| Portal | T1 必选 | Delivered、Read | 通过鉴权 API SSE/WebSocket，不直连 NATS |
| Email/SMTP | T1 标准 | Accepted；有独立证据时可扩展 | TLS、SecretReference、限速、模板 |
| 通用 Webhook | T1 标准 | Accepted；认证回执可扩展 | HMAC、重放保护、SSRF 防护、白名单 |
| SMS | T2 Provider | Accepted/Delivered 取决于 Provider | 区域、模板、签名、费用、配额、回执 Conformance |

钉钉、飞书、企业微信和值班平台优先通过通用 Webhook 接入；确需专用能力时新增 Provider change。Minimal 可只启用 Portal，未配置外部渠道不影响 T0/T1 核心运行。

### Decision 6: NATS 负责传输，PostgreSQL 保存事实

Alert、Job、Delivery 与 Outbox 同事务提交。Dispatcher 和 Channel Worker 使用 JetStream Durable Consumer、至少一次投递和幂等键。Worker 在渠道调用前读取 DeliveryRecord 和策略快照，调用后持久化 Attempt/Delivery，再 ACK。

NATS 不可用时告警事实和 Outbox 继续保存，Portal 可查询已落库 Alert；外部通知延迟并触发独立健康信号。恢复后 Outbox 续投。消息只携带 ID、路由快照摘要和模板/数据引用，不携带渠道 Secret 或未脱敏正文。

### Decision 7: Portal 实时体验经 Platform API

Platform API 提供租户鉴权的 Alert 查询、处置 API 和 SSE/WebSocket 事件。Portal 使用 Read Model/Alert API 初始化列表，再应用实时增量；断线后通过 last event/version 恢复或重新查询。通知铃铛、未读计数和详情基于服务端权威 read state，不依赖浏览器本地计数。

浏览器不能获得 SMTP/SMS/Webhook 凭据或 NATS 地址。确认、认领、静默等写操作携带期望版本，防止多人处置覆盖。

### Decision 8: 通知模板与个人信息最小化

模板按 Channel、Locale 和主版本管理，并限制可用字段。默认外部消息只包含严重等级、摘要、脱敏资源、发生时间和短期/受控 Portal 链接；详情在 Portal 权限校验后查看。

Contact 数据按租户加密与脱敏，渠道凭据只保存 SecretReference。Webhook DNS/IP 在保存和发送前均校验，阻止重绑定与内网探测。测试通知使用显式测试标记并完整审计。

## Data Model

```text
AlertRule
- id, tenant_scope, source_type, severity, expression_ref
- labels, annotations, enabled, version

AlertInstance
- id, tenant_id, project_id, environment_id
- rule_id, source, severity, resource_ref, fingerprint
- state, summary, first_seen_at, last_seen_at, resolved_at
- occurrence_count, assignee_id, acknowledged_by
- correlation_id, operation_id, runbook_ref, source_ref, version

Silence
- id, tenant_id, matchers, starts_at, ends_at
- reason, created_by, status, version

NotificationPolicy
- id, tenant_scope, matchers, contact_group_id
- channels, repeat_interval, escalation_steps
- active_schedule, recovery_notification, version

ContactGroup
- id, tenant_id, name, members, schedule_ref, version

NotificationChannel
- id, tenant_id, type, capability, config_ref
- secret_ref, enabled, conformance_ref, version

NotificationJob
- id, alert_id, policy_snapshot, channel_id
- idempotency_key, priority, scheduled_at, state

DeliveryRecord
- id, job_id, channel_type, destination_masked
- state, provider_message_id, accepted_at, delivered_at, read_at
- attempt_count, last_error_class, next_attempt_at, version

DeliveryAttempt
- id, delivery_id, attempted_at, result_class
- response_code, duration, trace_id
```

Policy snapshot 固定当次路由依据，后续策略修改不篡改历史 Delivery。Contact 与 Destination 查询按权限脱敏；审计保留 ID 和动作，不保存明文消息或 Secret。

## API and Event Contracts

公共 API 至少包括：

```text
GET  /alerts
GET  /alerts/{id}
POST /alerts/{id}:acknowledge
POST /alerts/{id}:assign
POST /alerts/{id}:silence
POST /alerts/{id}:unsilence
GET  /notification-policies
POST /notification-channels:test
GET  /notification-deliveries
GET  /alert-events                    # SSE/WebSocket upgrade
```

写 API 使用 IdempotencyKey、Correlation ID 和期望版本。内部事件示例：

```text
hnb.event.alert.firing.v1
hnb.event.alert.resolved.v1
hnb.command.notification.dispatch.v1
hnb.event.notification.delivery-changed.v1
```

外部来源通过认证 Adapter API/Webhook 或标准遥测规则集成；外部渠道回执必须校验 Channel、Tenant、Provider Message ID、签名和时间窗口。Schema First 生成 Go/TypeScript SDK，同一主版本保持兼容。

## State Machines

```text
Source event
  -> normalize and validate tenant/resource
  -> calculate fingerprint
  -> create/update AlertInstance
  -> apply silence/inhibition/grouping
  -> snapshot matching policy
  -> create NotificationJob + Delivery + Outbox
  -> dispatch through JetStream
  -> channel attempt
  -> persist result
  -> ACK
  -> Portal projection/SSE update

Channel failure
  -> retry with backoff
  -> circuit open / alternate channel
  -> escalation timer
  -> Failed after finite budget

Recovery event
  -> AlertInstance Resolved
  -> optional recovery deliveries
```

## Security and Isolation

- **租户隔离:** 所有 Rule、Alert、Silence、Policy、Contact、Channel、Job 和 Delivery 包含 Tenant ID；查询、SSE 和 Worker 执行均再次校验租户。
- **Secret:** SMTP、Webhook 和 SMS 凭据只使用 SecretReference；模板、事件、日志和消息不解析或输出 Secret。
- **个人信息:** 邮箱、手机号和值班信息最小化采集、加密、脱敏，并设置保留和删除策略；审计保留业务证据但不复制完整 PII。
- **权限:** 区分查看、确认、认领、静默、策略管理、渠道管理、模板管理、测试发送和人工重驱权限。
- **Webhook:** 强制 TLS、DNS/IP 双重校验、目标白名单/策略、HMAC、时间戳、幂等键和重放窗口，防止 SSRF。
- **供应链:** Channel Worker 和 SMS Provider 作为独立镜像进入 BOM、SBOM、签名和漏洞门禁。
- **审计:** 记录规则/策略/渠道变化、测试发送、Alert 状态变化、每次 Delivery Attempt、回执和人工重驱。
- **执行边界:** 通知只提供链接或建议；任何修复动作仍创建 Operation，不形成执行旁路。

## Performance, Capacity, and Observability

- 性能指标包括 Source-to-Firing、Firing-to-Portal、Firing-to-First-Attempt、API/SSE P95/P99 和策略评估时间。
- 容量模型包括活跃告警数、事件峰值、fingerprint 基数、策略/联系人数量、每告警渠道数、重试率和 Delivery 保留期。
- 大规模告警风暴下先执行去重聚合、按 Tenant/Channel 限速，并保证 Critical 和恢复事件优先；不得无限增长内存队列。
- 自监控包括 Normalizer 错误、Alert Store 写入、Outbox Age、JetStream Lag、待发送/最老 Job、成功率、重试、失败、抑制、熔断和 Provider 可用性。
- Notification Dispatcher 故障使用独立 Kubernetes 健康检查、平台组件状态和备用管理信号暴露，避免完全依赖自身渠道。
- 日志和链路携带 Tenant、Alert、Delivery、Correlation 和 Message ID，不记录正文、Secret 或完整联系方式。

## Compatibility and Conformance

| 接口 | Conformance 要求 |
|---|---|
| Source Adapter | Tenant/Resource 校验、fingerprint 稳定、Firing/Resolved 配对、Schema 兼容 |
| Portal API/SSE | 租户隔离、断线恢复、未读一致性、期望版本并发控制 |
| Email Worker | TLS、SecretReference、Accepted 语义、限速、重试、模板安全 |
| Webhook Worker | SSRF 防护、HMAC、重放保护、幂等、超时、熔断 |
| SMS Provider | 区域/模板/费用/配额/回执能力声明、安全和故障测试 |
| NATS transport | 重复投递、ACK 丢失、重放、积压和恢复，不改变 Alert/Delivery 权威状态 |

本 change 不修改 RuntimeTarget、Gateway 或现有 Domain Provider 生命周期，相关兼容矩阵为 N/A。SMS 是新的可选 Channel Provider，必须执行契约、功能、故障、安全、费用和性能 Conformance。

## Failure Modes

- `[来源事件重复或乱序]` -> fingerprint、来源时间和期望版本合并；恢复先于 Firing 时进入可观测异常而不误关闭新告警。
- `[告警风暴]` -> 去重、聚合、抑制、租户限速和优先级队列；Critical 不被低优先级流量饿死。
- `[NATS 不可用]` -> Alert/Delivery/Outbox 继续持久化到容量边界，Portal 可查询已提交事实，恢复后续投。
- `[SMTP/SMS/Webhook 故障]` -> 渠道独立重试、熔断和升级，不阻塞 Portal 或其他渠道。
- `[Worker 发送后崩溃]` -> 使用 Delivery 幂等键和 Provider Message ID 对账，无法确认时标记 Unknown/Failed 供人工处理而不伪造送达。
- `[SSE 断线]` -> 客户端按游标恢复或重新读取 Alert Read Model，未读计数由服务端校正。
- `[策略误配置无接收人]` -> Critical 使用默认安全路由并产生配置告警；非 Critical 记录 Suppressed/Unroutable。
- `[Webhook DNS 重绑定]` -> 发送时重新解析并验证最终连接 IP，禁止跳转到未批准地址。
- `[通知系统自身故障]` -> 通过独立健康检查、组件状态和 Runbook 暴露，不递归依赖同一 Dispatcher。

## Risks / Trade-offs

- `[新增领域模型和 Portal 复杂度]` -> 先交付 Portal-only、基础路由和 Email/Webhook，再启用值班升级与 SMS。
- `[通知疲劳]` -> 默认去重、聚合、防抖、维护窗口和重复间隔；所有抑制可解释可审计。
- `[外部渠道无法证明真实阅读]` -> 严格使用渠道能力状态，不把 Accepted 推断为 Delivered/Read。
- `[SMS 费用与区域合规]` -> T2 Provider、默认关闭、费用预算、区域/数据驻留声明和 Conformance。
- `[Critical 默认路由可能打扰用户]` -> 仅组织管理员配置强制路由，提供演练与清晰策略预览，不允许普通用户关闭。
- `[Alert Store 变成第二观测平台]` -> 只保存摘要和引用，原始遥测仍由来源系统负责。

## Alternatives Considered

- **仅使用监控产品自带通知:** 无法统一 Operation、安全、市场、AI 和 Edge 领域告警，也难以统一租户权限和审计。
- **直接从每个服务发送 Email/SMS:** 凭据分散、重复通知、无统一路由和送达记录，拒绝。
- **Portal 直接订阅 NATS:** 暴露内部基础设施和租户边界，拒绝。
- **MVP 强制 SMS:** 引入费用、区域和厂商依赖，不符合 Minimal；改为 T2 Provider。
- **把 Alert 存在 JetStream:** 保留期和消费状态不等于业务生命周期，无法满足查询、处置和审计；使用 PostgreSQL Alert Store。

## Migration Plan

1. 创建数据库 Schema、Alert API、Normalizer 和 Source Adapter，影子接收现有告警但不通知。
2. 比较现有来源与 fingerprint/状态结果，修复重复、恢复配对和租户映射。
3. 启用 Portal 告警中心、只读实时更新和默认 Portal-only 路由。
4. 启用确认、认领、静默、维护窗口和审计。
5. 灰度 Email，再启用通用 Webhook；逐渠道验证测试通知、限速、失败和回滚。
6. SMS Provider 独立完成 Conformance 后按租户显式启用。
7. 完成告警风暴、渠道故障、NATS 故障、SSE 断线、租户越权和灾难恢复演练。

回滚时先暂停外部 Channel Worker，保留 Alert/Delivery/Outbox 和审计；Portal 退回只读或隐藏 Alert Pack 入口。来源监控不受影响，未处理 Critical 告警导出并交由 Runbook 人工接管。不得在回滚中删除未解决告警或伪造送达状态。

## Upgrade, Rollback, and Disaster Recovery

- Schema 采用 expand/migrate/contract，先升级消费者和 API，再启用新生产者字段。
- Channel Worker 可独立升级回滚；模板主版本与 Worker 兼容矩阵进入 BOM。
- Alert Store 纳入平台数据库备份恢复，恢复后通过 Source Adapter 和 Outbox 对账近期状态。
- Delivery 历史按合规保留；恢复时不盲目重发过期通知，按策略重新评估活动 Critical 告警。
- 灾备演练验证 Alert/Policy/Contact/Delivery 一致性、SecretReference 可解析性和 Portal 未读计数重建。

## Open Questions

- T1 默认 SMTP 实现与模板引擎、Vue UI 组件库和 SSE 库的精确版本是什么？
- Critical/Warning/Info 的默认重复间隔、升级时间和通知 SLO 分别是多少？
- ContactGroup 的值班计划由 HNB 内置最小模型管理，还是仅对接企业排班 Provider？
- DeliveryRecord、个人联系方式和通知正文摘要的默认保留期限如何满足目标市场合规要求？
- 通知系统自身故障的批准备用管理通道是 Kubernetes Event、外部探针还是平台安装器状态？
