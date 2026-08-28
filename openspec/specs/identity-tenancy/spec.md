# identity-tenancy

## Purpose
定义企业多租户体系中租户、项目、环境、身份、角色、权限与审批上下文在 API、事件、Provider 和可观测链路中的传播、隔离和最小凭据暴露行为。

## Requirements

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
平台 SHALL 支持平台管理员、租户管理员、项目管理员、运维人员、发布者和只读用户六种角色，支持角色继承、权限矩阵和策略绑定，授权作用域 SHALL 支持 tenant/project/namespace 三级；高风险 Operation SHALL 绑定审批策略，进入 PendingApproval 状态。`user_roles` 表的变更 SHALL 触发 K8s RoleBinding 的自动同步。

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

#### Scenario: 角色分配触发 K8s 同步
- **GIVEN** 平台管理员为用户授予 Project P1 的 `operator` 角色
- **WHEN** `user_roles` 表写入新记录
- **THEN** K8s RBAC Syncer 在 30 秒内在 P1 的所有 namespace 中创建 RoleBinding
- **AND** 同步结果记录在审计日志中

### Requirement: [TENANT-008] SecretReference 凭据管理
平台 SHALL 使用 SecretReference 格式引用凭据，格式为 `secret://tenant/{tenant_id}/{secret_name}`；凭据 SHALL 使用 AES-256-GCM 加密存储，仅平台凭据服务有解密权限；运行凭据 SHALL 由目标侧工作负载身份或平台凭据服务按最小权限解析。

**Traceability:** TENANT-004

#### Scenario: 市场触发部署引用 Secret
- **GIVEN** 市场 Release 引用 `secret://tenant/t1/db-password`
- **WHEN** 平台生成 ExecutionPlan
- **THEN** 计划只包含 SecretReference 而不包含明文密码
- **AND** Provider 执行时通过工作负载身份获取实际凭据

### Requirement: [P1-ING-001] Unified Verified Identity Contract
Every protected platform entry point SHALL accept only a versioned identity
credential whose signature, algorithm, issuer, audience, expiry, not-before,
key identifier, subject, and subject type are verified against the same
approved claim profile. Verification failure SHALL stop processing before any
domain handler or repository access.

**Traceability:** P0-BASE-004, TENANT-005, TENANT-006

#### Scenario: Token was issued for a different audience
- **GIVEN** a correctly signed token whose audience does not include the HNB platform entry point
- **WHEN** the token is presented to a protected route
- **THEN** the request is rejected before the handler and the reason is recorded without logging the token

### Requirement: [P1-ING-002] Trusted Context Derivation and Header Sanitization
The trusted ingress SHALL derive subject and tenant context from verified
claims and authorized membership, SHALL remove or overwrite inbound identity
headers, and SHALL propagate a typed context to downstream code. A caller
supplied tenant, user, role, permission, or approval value SHALL NOT become
authoritative identity.

**Traceability:** P0-BASE-004, TENANT-005

#### Scenario: Caller spoofs tenant and user headers
- **GIVEN** a valid tenant-A token and caller-supplied headers naming tenant B and another user
- **WHEN** the request crosses trusted ingress
- **THEN** downstream code receives only the verified tenant-A subject context and the spoofed values cannot influence authorization

### Requirement: [P1-ING-003] Scope-Aware Fail-Closed Authorization
Every protected operation SHALL be authorized using subject, tenant,
resource kind, resource identifier, action, and applicable
project/environment/namespace scope. Missing policy data, unavailable policy
evaluation, or scope mismatch SHALL deny the operation, and repositories SHALL
apply the same tenant boundary used by the decision.

**Traceability:** TENANT-006, TENANT-007, P0-BASE-004

#### Scenario: Subject has permission in a different namespace
- **GIVEN** a subject may read pods in namespace A but has no grant in namespace B
- **WHEN** the subject requests a pod in namespace B
- **THEN** authorization denies the request and no namespace-B repository or target query is performed

### Requirement: [P1-ING-004] Audience-Restricted Service Identity
Service-to-service calls SHALL use an authenticated workload or service
identity restricted to the intended audience and action. Services SHALL NOT
trust network location, shared caller-controlled headers, or unrestricted
replayed end-user tokens as service authentication.

**Traceability:** TENANT-005, CONTRACT-001, P0-BASE-004

#### Scenario: Internal request lacks a service audience
- **GIVEN** a request reaches an internal intent endpoint with a user token not issued for that service
- **WHEN** the service authenticates the caller
- **THEN** it rejects the request and creates no intent or Operation

### Requirement: [P1-ING-005] Key Rotation and Credential Revocation
Identity signing and verification keys SHALL have versioned identifiers,
bounded overlap, protected private material, and tested rotation and emergency
revocation procedures. Disabled subjects and revoked credentials SHALL cease
authorizing new requests within the documented propagation bound.

**Traceability:** TENANT-008, OBS-004, P0-BASE-004

#### Scenario: Signing key is emergency-revoked
- **GIVEN** a previously valid token signed by a compromised key
- **WHEN** the key is marked revoked and the maximum propagation bound elapses
- **THEN** every protected entry point rejects that token while accepting tokens signed by active keys

### Requirement: [P1-ING-006] Security Audit and Correlation
Security-sensitive actions SHALL emit tenant-scoped audit evidence for
authentication, tenant selection, authorization, intent submission, approval,
denial, cancellation, and credential lifecycle actions with subject, decision, policy/key version,
resource/action, correlation ID, and outcome. Evidence SHALL redact tokens,
Secrets, credentials, and sensitive request values.

**Traceability:** OP-006, OBS-001, SEC-005

#### Scenario: Auditor traces a denied runtime mutation
- **GIVEN** a subject attempts a runtime mutation without the required scoped permission
- **WHEN** authorization denies the request
- **THEN** audit evidence identifies the verified subject, scope, action, policy version, correlation, and denial outcome without sensitive values
