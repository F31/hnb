## 1. Contracts and BOM

- [ ] 1.1 `[ALERT-001][ALERT-002][ALERT-009]` 定义 AlertRule、AlertInstance、Silence、NotificationPolicy、Channel、Job、Delivery 和 Attempt 的版本化 API/Event Schema并生成 Go/TypeScript SDK；证据：Schema lint、兼容检查和生成物测试。
- [ ] 1.2 `[ALERT-004][ALERT-005]` 定义严重等级、策略匹配、ContactGroup、值班时间、升级步骤、默认安全路由和渠道能力 Schema；证据：策略契约测试及示例配置。
- [ ] 1.3 `[ALERT-005][ALERT-006][ALERT-007]` 固定 T1 Portal/Email/Webhook 组件、模板 Schema、镜像 digest 和兼容矩阵；证据：Core/Infrastructure BOM 评审记录。
- [ ] 1.4 `[ALERT-008]` 定义 SMS Provider Manifest、区域、模板、费用、配额、回执和数据驻留 SPI；证据：Provider 契约和 Conformance 用例。
- [ ] 1.5 `[ALERT-001]` 记录指标/日志/链路后端与 Alertmanager 产品选择为 N/A，本 change 只定义可替换 Source Adapter；证据：架构边界评审记录。

## 2. Database and Migration

- [ ] 2.1 `[ALERT-001][ALERT-002]` 创建 AlertRule、AlertInstance 和状态审计的 expand 数据库迁移、索引及租户约束；证据：空库和存量升级测试。
- [ ] 2.2 `[ALERT-003][ALERT-004]` 创建 fingerprint、Silence、MaintenanceWindow、Policy、ContactGroup 和 Schedule Schema；证据：并发、唯一性和策略查询测试。
- [ ] 2.3 `[ALERT-005][ALERT-009]` 创建 Channel、NotificationJob、DeliveryRecord、DeliveryAttempt、用户偏好和 Outbox Schema；证据：迁移、回滚/前向修复和数据保留测试。
- [ ] 2.4 `[ALERT-001][ALERT-009]` 实施数据库租户隔离、联系方式加密/脱敏和最小权限账号；证据：跨租户拒绝和静态安全检查。

## 3. Alert Normalization and Lifecycle

- [ ] 3.1 `[ALERT-001]` 实现版本化 Source Adapter 和 Normalizer，覆盖 Operation、Provider、安全和标准外部告警样例；证据：来源契约和归一化单元测试。
- [ ] 3.2 `[ALERT-001][ALERT-003]` 实现稳定 fingerprint、重复合并、计数、来源时间和 Firing/Resolved 配对；证据：重复、乱序、抖动和恢复测试。
- [ ] 3.3 `[ALERT-002]` 实现 Pending/Firing/Acknowledged/Silenced/Resolved 状态机、期望版本和审计；证据：合法/非法迁移及并发处置测试。
- [ ] 3.4 `[ALERT-003]` 实现聚合、父子抑制、防抖、重复间隔、维护窗口和有期限静默；证据：时间控制测试和解释性输出。

## 4. Routing and Dispatch

- [ ] 4.1 `[ALERT-004]` 实现按租户、项目、环境、严重等级、来源、资源、标签和时间的策略匹配及快照；证据：优先级、冲突和默认路由测试。
- [ ] 4.2 `[ALERT-004]` 实现 ContactGroup、值班时间、重复通知、升级计时、备用渠道和停止条件；证据：虚拟时钟升级测试。
- [ ] 4.3 `[ALERT-004][ALERT-005]` 实现用户偏好与组织强制 Critical 路由求交集；证据：允许偏好和禁止关闭 Critical 测试。
- [ ] 4.4 `[ALERT-009]` 实现 NotificationJob/Delivery 与 Outbox 同事务提交、稳定幂等键和 JetStream Dispatcher；证据：事务回滚、重复投递和 Dispatcher 重启测试。
- [ ] 4.5 `[ALERT-009][ALERT-010]` 实现按渠道限速、退避、熔断、失败隔离、人工重驱和升级不阻塞；证据：渠道故障和积压集成测试。

## 5. Notification Channels

- [ ] 5.1 `[ALERT-006]` 实现 SMTP TLS、SecretReference、收件人组、模板、语言/时区、测试邮件和 Accepted 状态；证据：模拟 SMTP 成功、超时、拒绝和无回执测试。
- [ ] 5.2 `[ALERT-007]` 实现 Webhook HMAC、时间戳、幂等键、重放保护、超时和响应状态；证据：签名、重试和回执契约测试。
- [ ] 5.3 `[ALERT-007]` 实现 Webhook URL、DNS/IP、跳转和发送时重校验的 SSRF 防护；证据：内网、DNS 重绑定和恶意跳转安全测试。
- [ ] 5.4 `[ALERT-008]` 创建不绑定厂商的示例 SMS Provider 和 Conformance Harness，覆盖回执、区域、模板、费用、配额和脱敏；证据：契约、故障、安全和费用门禁报告。
- [ ] 5.5 `[ALERT-005]` 验证未安装 SMS Provider 时能力隐藏且 Portal/Email/Webhook 不受影响；证据：T1-only 集成测试。

## 6. Web Portal

- [ ] 6.1 `[UX-004][ALERT-001]` 使用 Vue 实现告警中心列表、详情、严重等级/状态/来源筛选及关联资源、Operation、日志、指标、链路和 Runbook 跳转；证据：组件和浏览器 E2E 测试。
- [ ] 6.2 `[UX-004][ALERT-009]` 实现通知铃铛、服务端未读计数、SSE/WebSocket 增量更新、断线恢复和重新同步；证据：断线、乱序和多标签页测试。
- [ ] 6.3 `[UX-004][ALERT-002]` 实现按权限显示确认、认领、静默和解除静默动作及期望版本冲突处理；证据：RBAC 和并发处置 E2E。
- [ ] 6.4 `[ALERT-004][ALERT-005]` 实现策略、ContactGroup、用户偏好、Email/Webhook 配置和测试通知界面；证据：表单 Schema、权限和敏感字段遮蔽测试。

## 7. Security, Reliability, and Observability

- [ ] 7.1 `[ALERT-001][ALERT-004][UX-004]` 覆盖 Alert/API/SSE/Policy/Contact/Delivery 全链路租户隔离和越权审计；证据：跨租户安全测试报告。
- [ ] 7.2 `[ALERT-006][ALERT-007][ALERT-008][ALERT-010]` 扫描模板、NATS 消息、日志、链路和审计，确认无 Secret、Token、kubeconfig 或完整 PII；证据：敏感信息测试报告。
- [ ] 7.3 `[ALERT-010]` 暴露 Source-to-Firing、Firing-to-Portal/First-Attempt、积压、最老任务、成功率、重试、失败、抑制、熔断和 Provider 可用性指标；证据：仪表盘、告警和示例链路。
- [ ] 7.4 `[ALERT-003][ALERT-010]` 执行告警风暴压测，验证去重、聚合、租户/渠道限速、Critical 优先和容量预算；证据：绑定版本与环境的 P95/P99 报告。
- [ ] 7.5 `[ALERT-009][ALERT-010]` 注入 NATS、数据库、Dispatcher、SMTP 和 Webhook 故障，验证事实不丢失、渠道隔离和恢复；证据：故障矩阵和恢复报告。
- [ ] 7.6 `[ALERT-010]` 建立不依赖 Notification Dispatcher 的健康信号和人工 Runbook；证据：Dispatcher 全停演练及外部可见结果。

## 8. Rollout, Recovery, and Documentation

- [ ] 8.1 `[ALERT-001][ALERT-003]` 以影子模式接入存量告警，比较 fingerprint、状态、租户映射和通知数量，不发送外部消息；证据：差异报告。
- [ ] 8.2 `[UX-004][ALERT-004]` 按 Portal-only、处置动作、Email、Webhook 的顺序灰度，并验证每阶段回滚；证据：灰度和回滚记录。
- [ ] 8.3 `[ALERT-008]` 将 SMS 保持默认关闭，完成独立 Provider Conformance 后再按租户启用；证据：能力开关和认证记录。
- [ ] 8.4 `[ALERT-001][ALERT-009]` 将 Alert Store 纳入备份恢复并验证活动告警、策略、Delivery、Outbox 和未读计数重建；证据：灾难恢复演练与 RPO/RTO。
- [ ] 8.5 `[ALERT-001][ALERT-010][UX-004]` 编写规则、路由、静默、值班、渠道、模板、失败重驱、隐私、故障和回滚 Runbook；证据：文档评审和桌面演练。
- [ ] 8.6 `[ALERT-001][ALERT-002][ALERT-003][ALERT-004][ALERT-005][ALERT-006][ALERT-007][ALERT-008][ALERT-009][ALERT-010][UX-004]` 执行完整告警产生、降噪、Portal、Email、Webhook、可选 SMS、升级、恢复和审计 E2E；证据：Requirement 映射报告。
- [ ] 8.7 `[ALERT-001][ALERT-002][ALERT-003][ALERT-004][ALERT-005][ALERT-006][ALERT-007][ALERT-008][ALERT-009][ALERT-010][UX-004]` 运行 `openspec validate --all --strict`、完成 verify、同步规格并归档；证据：零阻断校验、verify 和 sync/archive 记录。
