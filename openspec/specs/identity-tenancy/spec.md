# identity-tenancy

## Purpose
定义企业多租户体系中租户、项目、环境、身份、角色、权限与审批上下文在 API、事件、Provider 和可观测链路中的传播、隔离和最小凭据暴露行为。

## Requirements

### Requirement: [TENANT-001] 租户上下文全链路传播
Tenant ID、Project ID、Environment ID、Actor ID 和 Correlation ID SHALL 贯穿 API、数据库、缓存、事件、审计、Provider 调用和可观测数据。

**Traceability:** AI-GOV-03, INT-07

#### Scenario: 跨组件执行一次租户部署
- **GIVEN** 租户 A 在生产环境提交一个部署请求
- **WHEN** 请求经过平台、Provider 和 Runtime Driver
- **THEN** 每个阶段均可使用同一 Correlation ID 追踪
- **AND** 本次部署产生的租户域落库记录均包含 Tenant ID

### Requirement: [TENANT-002] 跨租户访问默认拒绝
租户资源、日志、制品授权、模型端点、知识引用、Gateway Route 和成本数据 SHALL 默认隔离；跨租户共享 SHALL 通过显式授权对象建立。

**Traceability:** MKT-08, AI-GOV-03, GW-11

#### Scenario: 租户尝试读取其他租户资源
- **GIVEN** 租户 A 与租户 B 均存在
- **WHEN** 租户 A 查询租户 B 的运行实例或 AI 调用日志
- **THEN** 请求被拒绝
- **AND** 越权尝试进入审计

### Requirement: [TENANT-003] 权限与审批分层
平台 SHALL 支持平台管理员、租户管理员、项目管理员、运维人员、发布者和只读用户等角色，并允许高风险 Operation 绑定审批策略。

**Traceability:** AI-OPS-06, MKT-08

#### Scenario: 高风险数据库切换
- **GIVEN** 普通运维用户提交数据库主备切换
- **WHEN** 策略判断该操作需要审批
- **THEN** Operation 保持 PendingApproval
- **AND** 只有授权审批人确认后才能执行

### Requirement: [TENANT-004] 凭据最小暴露
市场、Portal、Copilot 和普通 Provider SHALL 仅持有 SecretReference；运行凭据 SHALL 由平台凭据服务或目标侧工作负载身份按最小权限解析。

**Traceability:** MKT-09, INT-07

#### Scenario: 市场触发部署
- **GIVEN** 市场发布了一个需要数据库密码的 Release
- **WHEN** 平台生成 ExecutionPlan
- **THEN** 计划只包含 SecretReference 而不包含明文密码
- **AND** 市场数据库中不存在运行 Secret
