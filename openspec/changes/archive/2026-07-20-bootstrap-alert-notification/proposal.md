## Metadata

- Change ID: `bootstrap-alert-notification`
- Tier: T1
- Planes: runtime-governance, market, ai-extension
- Affected Specs: new `alert-notification/ALERT-001` through `ALERT-010`, new `portal-experience/UX-004`
- Depends On: `bootstrap-identity-tenancy`, `bootstrap-contracts-events`, `bootstrap-operation-engine`, `bootstrap-observability-baseline`, `adopt-nats-jetstream-messaging`
- Target Milestone: MVP
- Risk: medium

## Why

现有规格要求产生基础告警，但没有定义告警生命周期、租户路由、降噪、Portal 告警中心以及 Email、Webhook、SMS 的可靠送达行为。缺少该闭环会导致故障虽然被检测，却无法确保正确人员及时看到、确认、升级和审计。

## What Changes

- 新增统一 AlertRule、AlertInstance、NotificationPolicy、NotificationChannel、Silence 和 DeliveryRecord 模型。
- 定义 Pending、Firing、Acknowledged、Silenced、Resolved 告警生命周期及确认、认领、静默、恢复和审计行为。
- 增加 fingerprint 去重、时间窗口聚合、抑制、防抖、维护窗口和重复通知间隔。
- 增加按租户、项目、环境、严重等级、资源、标签和值班时间的通知路由与升级策略。
- Portal 增加告警中心、通知铃铛、未读计数、实时更新、筛选、确认、认领、静默及关联日志/指标/链路/Operation 跳转。
- T1 默认支持 Portal、Email/SMTP 和签名 Webhook；SMS 作为 T2 可替换 Provider，不成为 Minimal/MVP 强制外部依赖。
- 定义 Accepted、Delivered、Read 等渠道能力相关的送达语义，禁止把 SMTP 接受错误标记为用户已读。
- 使用 Transactional Outbox + NATS JetStream 分发通知任务，PostgreSQL Alert Store 保存权威告警、策略和送达记录。
- 增加通知服务自身的延迟、成功率、积压、重试、失败和渠道可用性监控。

## Capabilities

### New Capabilities

- `alert-notification`: 定义告警归一化、生命周期、降噪、路由、Portal/Email/Webhook/SMS 渠道、送达记录、安全和通知可靠性。

### Modified Capabilities

- `portal-experience`: 增加租户隔离的 Web 告警中心、实时通知、未读状态和告警处置交互。

## Non-Goals

- 不自研指标、日志或链路存储系统，也不在本 change 固定 Prometheus、Alertmanager 或其他监控产品。
- 不让 Portal、用户浏览器、SMTP、SMS 或 Webhook Provider 直接访问 NATS。
- 不把 NATS 当作 Alert/Delivery 权威数据库。
- 不在 T1 强制安装短信、语音电话、钉钉、飞书、企业微信、PagerDuty 或 Opsgenie 专用实现。
- 不允许通知动作绕过权限、审批和 Operation 修改运行目标。

## Impact

- **代码:** Alert Normalizer、Alert Store、Policy/Route Engine、Notification Dispatcher、Portal Alert Center、Email/Webhook Workers 和可选 SMS Provider SPI。
- **API/事件:** 新增版本化 Alert/Notification API、领域事件、SSE/WebSocket 事件和 Provider 契约。
- **数据:** 新增告警规则、告警实例、静默、路由、渠道、用户偏好和送达记录 Schema；数据库迁移需支持回滚或前向修复。
- **依赖:** 复用 PostgreSQL、Transactional Outbox 和 NATS JetStream；T1 增加 SMTP 与通用 Webhook 集成，不新增强制消息中间件。
- **资源:** Minimal 可关闭外部渠道，仅保留 Portal；通知 Worker 按渠道独立扩展，资源与速率预算进入 BOM。
- **运维:** 增加渠道配置、凭据轮换、模板、失败重驱、静默、值班升级和通知自监控 Runbook。

## Compatibility and Migration

- 新增 API 和 Schema，不破坏现有监控、Operation 或 Portal 接口。
- 将现有 Operation、Provider、安全和资源告警通过 Adapter 归一化为 AlertInstance；原始来源仍保持权威。
- 先启用 Portal-only 默认策略，再灰度 Email 和 Webhook；SMS 仅在安装认证 Provider 后出现。
- 旧告警若无法生成稳定 fingerprint，迁移期间标记来源并限制重复通知，避免通知风暴。

## Security and Isolation

- Alert、Policy、Channel、Contact 和 DeliveryRecord 全部绑定 Tenant/Project/Environment，跨租户默认拒绝。
- SMTP、SMS、Webhook 凭据只保存 SecretReference；电话号码、邮箱和消息正文按权限脱敏。
- Webhook 使用 TLS、目标域名白名单、HMAC 签名、时间戳和重放保护，防止 SSRF 与伪造。
- 通知模板禁止渲染 Secret、kubeconfig、Token 或未脱敏日志；高敏事件默认发送摘要和 Portal 链接。
- 确认、认领、静默、策略修改、测试通知和人工重驱均进入审计。

## Reliability and Operations

- Alert/Delivery 与 Outbox 同事务提交，渠道 Worker 按至少一次投递和幂等键处理。
- 各渠道独立限速、超时、熔断、有限重试和失败隔离；单一渠道故障不阻塞 Portal 或其他渠道。
- Portal 通知可准确记录 Delivered/Read；Email 通常只记录 Accepted；SMS 仅在 Provider 提供回执时记录 Delivered。
- 监控告警到首次通知延迟、渠道成功率、最老待发送年龄、重试、失败、抑制数量和 Provider 可用性。
- 通知系统自身故障必须通过独立健康告警或备用管理通道暴露，避免完全依赖自身发送故障通知。

## Rollout and Rollback

1. 创建 Alert Store、Schema 和只读归一化 Adapter，不发送外部通知。
2. 启用 Portal 告警中心和默认只读策略，验证租户隔离和降噪。
3. 灰度 Email，再启用通用 Webhook；每个渠道先执行测试通知和速率限制验证。
4. 认证并按需安装 SMS Provider，默认关闭且设置费用与严重等级门禁。
5. 回滚时先暂停外部渠道，保留 Alert/Delivery/Audit 数据，Portal 退回只读；不得删除未处理严重告警。

## Exit Criteria

- **GIVEN** 同一资源在抖动窗口内重复产生相同故障，**WHEN** 告警进入平台，**THEN** 只形成一个活动 AlertInstance 且通知按聚合策略发送。
- **GIVEN** Critical 告警匹配租户升级策略，**WHEN** 首选渠道持续失败，**THEN** 系统记录失败、按策略重试并升级到备用渠道。
- **GIVEN** 用户登录 Portal，**WHEN** 其租户产生新告警，**THEN** 未读计数实时更新且用户无法读取其他租户告警。
- **GIVEN** SMTP 接受邮件但没有用户阅读证据，**WHEN** 查看 DeliveryRecord，**THEN** 状态为 Accepted 而不是 Delivered 或 Read。
- **GIVEN** 未安装 SMS Provider，**WHEN** 用户配置短信渠道，**THEN** Portal 明确提示能力不可用且不影响 Portal、Email 和 Webhook。
- **GIVEN** 通知模板引用 Secret 或 Webhook 指向未批准内网地址，**WHEN** 保存或发送，**THEN** 请求被拒绝并进入安全审计。
