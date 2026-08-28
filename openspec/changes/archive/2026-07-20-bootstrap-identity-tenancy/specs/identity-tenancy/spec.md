## ADDED Requirements

### Requirement: [TENANT-005] 租户上下文传播
平台 SHALL 在 API Gateway、Middleware、Service、Repository、Event、Audit、Provider 和 Observability 各层传播 Tenant ID、Project ID、Environment ID、Namespace ID、Actor ID 和 Correlation ID；Context SHALL 在请求入口由 Middleware 从 JWT 或 API Key 提取，注入 Go context，并在所有下游组件中自动可用。

**Traceability:** TENANT-001

#### Scenario: 跨组件请求追踪
- **GIVEN** 用户通过 API Gateway 发起部署请求
- **WHEN** 请求经过 API、Operation Engine、Provider 和 Runtime Driver
- **THEN** 每个阶段 Log 和 Span 均包含 Tenant ID、Project ID、Environment ID、Namespace ID 和 Correlation ID
- **AND** 审计记录可还原完整跨组件追踪

#### Scenario: 多 Namespace 环境部署
- **GIVEN** Production 环境包含 api、worker、cache 三个 K8s Namespace
- **WHEN** 用户发起部署到 Production 环境
- **THEN** 部署请求携带目标 Namespace ID
- **AND** Middleware 验证用户有该 Namespace 的操作权限

### Requirement: [TENANT-006] 跨租户访问控制中间件
平台 SHALL 实现 Tenant Context Middleware，在 API 请求处理前提取租户信息并验证访问权限；跨租户请求 SHALL 默认被拒绝，显式授权对象 SHALL 是唯一共享机制。

**Traceability:** TENANT-002

#### Scenario: 跨租户资源读取
- **GIVEN** 租户 A 用户持有租户 A 的 JWT
- **WHEN** 其尝试访问租户 B 的 Alert、Operation 或 Market Listing
- **THEN** Middleware 在路由匹配前拒绝请求
- **AND** 返回 403 Forbidden 并记录审计

### Requirement: [TENANT-007] RBAC 角色与权限
平台 SHALL 支持平台管理员、租户管理员、项目管理员、运维人员、发布者和只读用户六种角色，支持角色继承、权限矩阵和策略绑定，授权作用域 SHALL 支持 tenant/project/namespace 三级；高风险 Operation SHALL 绑定审批策略，进入 PendingApproval 状态。

**Traceability:** TENANT-003

#### Scenario: 审批策略阻止高风险操作
- **GIVEN** 运维用户提交生产数据库切换 Operation
- **WHEN** 审批策略要求租户管理员审批
- **THEN** Operation 进入 PendingApproval
- **AND** 只有租户管理员审批后进入 Queued

#### Scenario: Namespace 级授权
- **GIVEN** 用户被授予 Project A 的 Namespace NS1 的 `operator` 角色
- **WHEN** 其尝试操作 Project A 的同环境下另一 Namespace NS2
- **THEN** 授权引擎拒绝该请求
- **AND** 返回 403 Forbidden

### Requirement: [TENANT-008] SecretReference 凭据管理
平台 SHALL 使用 SecretReference 格式引用凭据，格式为 `secret://tenant/{tenant_id}/{secret_name}`；凭据 SHALL 使用 AES-256-GCM 加密存储，仅平台凭据服务有解密权限；运行凭据 SHALL 由目标侧工作负载身份或平台凭据服务按最小权限解析。

**Traceability:** TENANT-004

#### Scenario: 市场触发部署引用 Secret
- **GIVEN** 市场 Release 引用 `secret://tenant/t1/db-password`
- **WHEN** 平台生成 ExecutionPlan
- **THEN** 计划只包含 SecretReference 而不包含明文密码
- **AND** Provider 执行时通过工作负载身份获取实际凭据