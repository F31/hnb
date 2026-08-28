## ADDED Requirements

### Requirement: [ALERT-001] 统一告警模型与来源归一化
平台 SHALL 将指标、日志、链路、Operation、Provider、Gateway、安全、AI 和 Edge 来源归一化为版本化 AlertRule 与 AlertInstance；AlertInstance SHALL 包含 Tenant、Project/Environment、Source、Severity、Resource、Fingerprint、Summary、FirstSeenAt、LastSeenAt、Correlation ID、可选 Operation ID 和 Runbook Reference，且 SHALL 保留原始来源引用而不复制敏感正文。

**Traceability:** OBS-001, OBS-002, SEC-005, TENANT-001

#### Scenario: Operation 滞留产生统一告警
- **GIVEN** 一个租户 Operation 超过批准的最大滞留时间
- **WHEN** 告警 Adapter 接收滞留事件
- **THEN** 平台创建或更新包含 Tenant、Operation 和 Resource 上下文的 AlertInstance
- **AND** 用户可从告警跳转到对应 Operation、日志、指标和链路

### Requirement: [ALERT-002] 告警生命周期与人工处置
AlertInstance SHALL 支持 Pending、Firing、Acknowledged、Silenced 和 Resolved 状态及确认、认领、静默、解除静默和恢复动作；每次状态变化 SHALL 记录操作者、原因、时间、期望版本和审计，Acknowledged 或 Silenced SHALL NOT 伪造来源故障已恢复。

**Traceability:** OBS-002, TENANT-003, CONTRACT-003

#### Scenario: 运维确认仍在触发的告警
- **GIVEN** 一个 Firing Critical 告警已分配给授权运维人员
- **WHEN** 运维填写原因并确认告警
- **THEN** 告警进入 Acknowledged 且来源健康状态仍显示为故障
- **AND** 确认人、时间和原因进入审计

#### Scenario: 来源恢复
- **GIVEN** 一个告警已被确认或静默
- **WHEN** 来源发送匹配 fingerprint 的恢复事件
- **THEN** 告警进入 Resolved 并记录恢复时间
- **AND** 是否发送恢复通知由通知策略决定

### Requirement: [ALERT-003] 告警去重、聚合与降噪
平台 SHALL 支持基于 fingerprint 的去重、时间窗口聚合、父子抑制、抖动检测、重复通知间隔、维护窗口和有期限静默；所有抑制 SHALL 保留计数和原因，静默过期 SHALL 自动恢复评估，Critical 告警 SHALL NOT 因缺失默认策略而被静默丢弃。

**Traceability:** OBS-002, CONTRACT-003

#### Scenario: 同一资源告警抖动
- **GIVEN** 同一资源在批准防抖窗口内重复产生相同 fingerprint 的故障
- **WHEN** 平台处理这些事件
- **THEN** 只维护一个活动 AlertInstance 并累计发生次数
- **AND** 通知次数遵守聚合和重复通知间隔

#### Scenario: 维护窗口结束
- **GIVEN** 一个资源在维护窗口内产生被抑制告警
- **WHEN** 维护窗口结束且故障仍存在
- **THEN** 平台重新评估并按当前严重等级路由告警
- **AND** 保留维护期内的抑制数量和原因

### Requirement: [ALERT-004] 多租户通知路由与升级
NotificationPolicy SHALL 可按 Tenant、Project、Environment、Severity、Source、Resource 类型、标签、工作时间和值班计划选择 ContactGroup 和 NotificationChannel，并定义初次通知、重复间隔、升级层级、备用渠道和停止条件；未匹配自定义策略的 Critical 告警 SHALL 使用平台批准的默认安全路由。

**Traceability:** TENANT-002, TENANT-003, OBS-002

#### Scenario: Critical 告警升级到备用渠道
- **GIVEN** Critical 告警的首选渠道在策略期限内持续失败
- **WHEN** 升级计时到期
- **THEN** 平台按策略向下一 ContactGroup 或备用渠道创建通知任务
- **AND** 原渠道失败、升级原因和每次尝试均可审计

#### Scenario: 租户修改其他租户路由
- **GIVEN** 租户 A 管理员登录 Portal
- **WHEN** 其尝试读取或修改租户 B 的 NotificationPolicy
- **THEN** 请求被拒绝
- **AND** 越权尝试进入安全审计

### Requirement: [ALERT-005] 渠道能力分级与用户偏好
T1 SHALL 提供 Portal、Email/SMTP 和通用 Webhook 渠道；SMS SHALL 作为 T2 可替换 Provider，仅在安装、授权并通过 Conformance 后可用。用户 MAY 在管理员策略允许范围内配置语言、时区、渠道和严重等级偏好，但 SHALL NOT 绕过组织对 Critical 告警的强制路由。

**Traceability:** GOV-001, GOV-002, UX-001, TENANT-003

#### Scenario: 环境未安装 SMS Provider
- **GIVEN** 当前部署未安装认证 SMS Provider
- **WHEN** 用户查看可用通知渠道或尝试配置短信
- **THEN** Portal 显示 SMS 不可用且不保存不可执行路由
- **AND** Portal、Email 和 Webhook 渠道继续正常工作

### Requirement: [ALERT-006] Email 通知安全与语义
Email 渠道 SHALL 支持使用 SecretReference 的 SMTP 配置、TLS、收件人组、版本化模板、语言/时区、测试通知、限速和有限重试；SMTP 服务接受邮件 SHALL 记录为 Accepted，除非存在独立送达或阅读证据，否则 SHALL NOT 标记 Delivered 或 Read。

**Traceability:** CFG-002, CFG-003, CONTRACT-003

#### Scenario: SMTP 接受告警邮件
- **GIVEN** SMTP 服务成功接受一封告警邮件但未提供送达或阅读回执
- **WHEN** Notification Worker 更新 DeliveryRecord
- **THEN** 状态为 Accepted
- **AND** Portal 不显示为 Delivered 或 Read

#### Scenario: SMTP 凭据出现在模板
- **GIVEN** 一个邮件模板尝试引用 SMTP Secret 或告警敏感字段
- **WHEN** 管理员保存或测试模板
- **THEN** 模板校验拒绝该配置
- **AND** 审计只记录违规字段路径而不记录 Secret 值

### Requirement: [ALERT-007] 通用 Webhook 通知
Webhook 渠道 SHALL 使用 TLS、目标域名或网段策略、SecretReference、HMAC 签名、时间戳、幂等键、重放保护、超时、限速、熔断和有限重试；成功 HTTP 响应 SHALL 记录为 Accepted，目标端业务处理结果仅在具有受认证回执时更新。

**Traceability:** CFG-002, CONTRACT-003, SEC-005

#### Scenario: Webhook 指向未批准内网地址
- **GIVEN** 租户提交一个解析到未批准内网网段的 Webhook URL
- **WHEN** 平台验证或发送测试通知
- **THEN** 请求被拒绝且不会发起网络连接
- **AND** SSRF 防护结果进入安全审计

#### Scenario: Webhook 重试重复请求
- **GIVEN** 接收端已处理通知但响应在网络中丢失
- **WHEN** Notification Worker 重试相同 DeliveryRecord
- **THEN** 请求携带相同幂等键和新的签名时间戳
- **AND** 平台将每次尝试关联到同一 DeliveryRecord

### Requirement: [ALERT-008] 可替换 SMS Provider
SMS Provider SHALL 声明支持区域、号码格式、模板/签名、配额、费用预算、回执、速率和数据驻留能力；平台 SHALL 对手机号脱敏并通过 SecretReference 访问凭据，只有 Provider 提供受认证送达回执时才能将 DeliveryRecord 标记为 Delivered。

**Traceability:** PROV-001, PROV-004, CFG-003, GOV-002

#### Scenario: SMS Provider 返回送达回执
- **GIVEN** 认证 SMS Provider 已接受通知并随后返回有效签名的送达回执
- **WHEN** 平台校验回执中的 Provider Message ID 和租户上下文
- **THEN** 对应 DeliveryRecord 更新为 Delivered
- **AND** Portal 只显示脱敏手机号和送达时间

#### Scenario: 短信超过租户费用预算
- **GIVEN** 租户短信费用或配额已达到策略上限
- **WHEN** 新告警尝试发送短信
- **THEN** 短信任务被抑制或转入批准的备用渠道
- **AND** 预算原因进入 DeliveryRecord 和审计

### Requirement: [ALERT-009] 通知任务与送达记录
每个渠道通知 SHALL 创建具有稳定幂等键的 NotificationJob 和 DeliveryRecord，并按渠道能力使用 Pending、Sending、Accepted、Delivered、Read、Failed、Suppressed 或 Cancelled 状态；发送尝试、响应摘要、重试、升级和人工重驱 SHALL 可追踪，渠道失败 SHALL NOT 修改 AlertInstance 的来源事实。

**Traceability:** CONTRACT-004, CONTRACT-005, OP-007

#### Scenario: Notification Worker 在发送后确认前重启
- **GIVEN** Worker 已向渠道发送通知但尚未持久化最终状态
- **WHEN** Worker 重启并重新收到相同任务
- **THEN** Worker 使用 DeliveryRecord 幂等键和渠道回执决定恢复动作
- **AND** 不创建第二个独立通知事实或错误关闭告警

### Requirement: [ALERT-010] 通知可靠性、安全与自监控
通知系统 SHALL 监控告警到首次通知的 P95/P99、待发送数量、最老任务年龄、渠道成功率、重试、失败、抑制、积压和 Provider 可用性，并对渠道独立执行限速、超时、熔断和失败隔离；通知系统自身不可用 SHALL 通过独立健康检查或备用管理通道暴露，且消息、模板、日志和审计 SHALL NOT 泄露 Secret 或未脱敏个人信息。

**Traceability:** OBS-001, OBS-002, OBS-005, CFG-002

#### Scenario: Email Provider 故障不阻塞 Portal
- **GIVEN** SMTP Provider 持续不可用
- **WHEN** 同一租户产生新的告警
- **THEN** Email 任务按策略重试或失败且 Portal 通知继续更新
- **AND** 渠道可用性、积压和告警到 Portal 延迟可观测

#### Scenario: 通知系统无法发送自身故障告警
- **GIVEN** Notification Dispatcher 自身不可用
- **WHEN** 独立健康检查检测到故障
- **THEN** 平台通过不依赖该 Dispatcher 的管理信号暴露故障
- **AND** Runbook 明确人工确认和恢复路径
