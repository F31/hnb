## ADDED Requirements

### Requirement: [UXLOOP-001] UI 与 CLI 的旅程状态一致
Portal 与 CLI SHALL 消费相同的受权威约束的计划、Operation、目标/应用观测、原因字典及支持矩阵，分别呈现受理、审批、执行、等待观测、验证、Ready、失败和降级等适用状态，以及最后观测时间、阻断原因和可执行的恢复步骤。动作 SHALL 按能力、Conformance、目标兼容及权限交集开放；客户端不得以缓存、HTTP 受理或执行终态代替实际可用性，租户切换时旧响应不得污染当前视图。

**Traceability:** UX-001, UX-003, UX-009, P1-CONSOLE-003, RT-005, ADL-002

#### Scenario: 两个入口查看同一未就绪应用
- **GIVEN** 应用执行成功但仍在等待新鲜观测
- **WHEN** 同一授权用户分别用 Portal 和 CLI 查看状态
- **THEN** 两者显示一致的执行结果、等待原因和最后观测时间，不报告应用 Ready

### Requirement: [UXLOOP-002] 可授权追踪的故障导航与诊断
平台 SHALL 支持从应用或告警定位相关资源、Operation、步骤、事件、日志/指标和 Runbook，并提供有界、脱敏的诊断导出与受权限控制的日志/终端入口。每次查询/流式会话 SHALL 验证 tenant/target/namespace/action，缺少高级遥测后端时 SHALL 保留最小健康/事件/执行诊断并明确不可用项；任何导航链接或客户端隐藏 SHALL NOT 替代服务端授权。

**Traceability:** UX-004, OBS-001, OBS-002, TENANT-006, P1-ING-006

#### Scenario: 从告警定位失败步骤
- **GIVEN** 授权租户的应用发生一次部署故障
- **WHEN** 用户从告警打开诊断链并导出资料
- **THEN** 能定位同一资源/Operation/步骤及可用日志、指标和恢复说明
- **AND** 导出不包含 token、私钥、kubeconfig、Secret 值和业务载荷，越权日志/终端请求被拒绝
