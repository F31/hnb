## ADDED Requirements

### Requirement: [UX-004] Web 告警中心与实时通知
Portal SHALL 提供租户隔离的告警中心、通知铃铛和未读计数，并通过鉴权后的 Platform API SSE/WebSocket 或 Read Model 实时更新；用户 SHALL 可按权限筛选、查看、确认、认领、静默和解除静默告警，并跳转到关联资源、Operation、日志、指标、链路和 Runbook。Portal SHALL NOT 直接连接内部消息系统或外部通知 Provider。

**Traceability:** ALERT-001, ALERT-002, ALERT-004, ALERT-009, TENANT-002

#### Scenario: 租户收到实时告警
- **GIVEN** 租户 A 的授权用户已登录 Portal
- **WHEN** 租户 A 产生一个新 Firing 告警
- **THEN** 通知铃铛和未读计数实时更新且告警出现在告警中心
- **AND** 租户 B 用户无法读取该告警或通知正文

#### Scenario: 只读用户尝试静默告警
- **GIVEN** 一个用户只有告警只读权限
- **WHEN** 其尝试静默或确认告警
- **THEN** Portal 隐藏或禁用不可执行动作且 API 拒绝越权请求
- **AND** 告警状态保持不变并记录越权尝试
