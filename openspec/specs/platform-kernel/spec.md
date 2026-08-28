# platform-kernel

## Purpose
定义 HNB Cloud T0 微内核允许承担的最小职责、唯一执行边界、查询与控制分离方式，以及控制面故障时的数据面隔离要求。
## Requirements
### Requirement: [KERNEL-001] 最小内核边界
HNB Core SHALL 仅包含身份与租户上下文、Operation Engine、ExecutionPlan Engine、Read Model、Resource Graph、Provider/Capability Registry、Policy Hook、Audit 与 Agent Gateway；具体 CNI、CSI、数据库、中间件、Gateway、AI Runtime 和边缘实现 SHALL NOT 编译进入内核。

**Traceability:** CTN-01, AI-ARCH-01, ART-STO-24

#### Scenario: 卸载可选能力后内核独立运行
- **GIVEN** 一个已安装 HNB Cloud 的环境
- **WHEN** 全部 T2/T3 能力包被停用或卸载
- **THEN** T0 内核组件仍可启动并通过健康检查
- **AND** 内核镜像依赖清单中不存在具体 Provider 实现镜像

### Requirement: [KERNEL-002] Operation 唯一写入口
所有部署、升级、回滚、扩缩容、备份、恢复、切换、删除、GC、OTA 和高风险配置变更 SHALL 通过持久化 Operation 执行；任何门户、Copilot、Provider 或 Controller SHALL NOT 绕过该状态机直接改变 RuntimeTarget。

**Traceability:** CMPOS-03, CMPOS-04, AI-OPS-02, GW-07, EDGE-18

#### Scenario: 外部组件请求资源变更
- **GIVEN** Gateway Provider 或 Copilot 生成资源变更计划
- **WHEN** 用户或策略批准执行
- **THEN** 平台创建可审计 Operation 并由 Runtime Driver 执行
- **AND** 直接调用集群写 API 的旁路请求被拒绝

### Requirement: [KERNEL-003] 查询与控制解耦
列表、搜索和聚合查询 SHALL 读取 Read Model；控制器 SHALL 通过事件或投影器更新 Read Model，查询接口 SHALL NOT 在请求路径实时遍历全部运行目标。

**Traceability:** METH-04, GW-13, EDGE-14

#### Scenario: 大规模目标下查询应用列表
- **GIVEN** 平台已纳管 100 个以上 RuntimeTarget
- **WHEN** 用户查询应用、Route 或边缘节点列表
- **THEN** 请求从 Read Model 返回
- **AND** 响应时延不随 RuntimeTarget 数量线性增长
- **AND** 结果包含 lastObservedAt 或 lastKnownStateAt

### Requirement: [KERNEL-004] 控制面故障不影响数据面
市场、平台控制面或 AI Extension Plane 不可用时，已运行应用、数据库、中间件、Gateway 数据面和已下发边缘负载 SHALL 继续运行。

**Traceability:** INT-05, AI-ARCH-02, EDGE-04

#### Scenario: 中心控制面中断
- **GIVEN** 一个生产应用已经成功部署并对外提供服务
- **WHEN** 市场和平台 API 同时停止
- **THEN** 应用数据面与 Gateway 数据面继续提供服务
- **AND** 恢复后控制面能够重新对账实际状态

### Requirement: [KERNEL-005] Gateway 不编译进内核
Gateway 控制面（GatewayProfile 管理、能力协商、多租户）作为内核逻辑层但仍在内核进程内；具体 Gateway Provider（Profile→YAML 转换 + K8s Apply）SHALL 作为独立 CapabilityPack 部署，NOT 编译进 HNB Core。

**Traceability:** GW-003, CTN-01

#### Scenario: 卸载 Gateway Provider
- **GIVEN** 环境未部署任何 Gateway Provider
- **WHEN** 内核启动
- **THEN** 内核不加载 Gateway 相关执行逻辑
- **AND** StepType "configure_gateway" 的 Step 无 Provider 可用时进入 Failed

### Requirement: [P1-WRITE-001] Typed Runtime Intent Boundary
Public and cross-plane callers SHALL express runtime mutations through a
versioned, typed RuntimeIntent referencing an immutable Release or
CompositionRelease, authorized target and scope, bounded parameters, and
SecretReferences. Callers SHALL NOT submit executable steps, Provider commands,
target credentials, artifact bytes, fencing tokens, or policy/approval results.

**Traceability:** KERNEL-002, OP-001, CONTRACT-001, P0-BASE-003

#### Scenario: Caller submits executable steps
- **GIVEN** an otherwise authenticated install request contains caller-authored Operation steps
- **WHEN** the RuntimeIntent contract is validated
- **THEN** the request is rejected before planning and no runtime command is emitted

### Requirement: [P1-WRITE-002] Server-Owned Immutable Planning
The platform SHALL resolve an accepted RuntimeIntent into an immutable
ExecutionPlan that pins Release identity, artifact digests, target capability
snapshot, policy results, approved parameters, SecretReferences, and the
complete step DAG. During planning, the platform SHALL resolve every step to
exactly one eligible Provider and SHALL persist that Provider's immutable
identity, version and artifact digest together with the complete validated
per-step inputs, SecretReferences, timeout, retry and compensation metadata.
Retries, resume, approval, worker restart and replay SHALL use the persisted
Provider resolution and step inputs and SHALL NOT silently re-resolve a newer
Provider or reconstruct inputs from mutable catalog, target or request state.
Planning failure SHALL create no runtime side effect.

**Traceability:** OP-001, RT-004, SEC-001, P0-BASE-003, PROVIDER-001, CONTRACT-003

#### Scenario: Target capability is incompatible
- **GIVEN** an accepted intent references a Release unsupported by the target capability snapshot
- **WHEN** the server generates the ExecutionPlan
- **THEN** planning fails with a stable reason and no Operation is queued for target execution

#### Scenario: Provider catalog changes after commitment
- **GIVEN** an ExecutionPlan pins Provider version `v1` and validated inputs for every step
- **WHEN** Provider version `v2` becomes preferred before a failed step is retried
- **THEN** the retry uses the pinned `v1` Provider identity, digest and persisted step inputs
- **AND** adopting `v2` requires a newly planned Operation rather than mutation of the committed plan

### Requirement: [P1-WRITE-003] Atomic Operation Commitment
The platform SHALL atomically persist the intent reference, immutable
ExecutionPlan, Operation, initial steps, audit evidence, read model, and
transactional outbox records before execution. Partial commitment SHALL NOT
produce a command, and the Operation store SHALL remain the only execution
state authority.

**Traceability:** KERNEL-002, OP-007, CONTRACT-004, PAG-001

#### Scenario: Outbox insert fails during submission
- **GIVEN** planning succeeded but the outbox record cannot be persisted
- **WHEN** the Operation transaction attempts to commit
- **THEN** the entire submission is rolled back and no worker or RuntimeTarget receives a command

### Requirement: [P1-WRITE-004] Complete Canonical Mutation Coverage
Every Release publication control and runtime mutation entry point SHALL create
or control the canonical
`Release/CompositionRelease -> ExecutionPlan -> Operation` chain. No public
route, Marketplace event consumer, agent, Console action, CLI, AI extension,
or Provider SHALL write a RuntimeTarget outside that chain.

**Traceability:** KERNEL-002, AI-004, P0-BASE-003, P0-BASE-006

#### Scenario: Marketplace install is requested
- **GIVEN** an entitled subject requests installation of a published Release
- **WHEN** app-market accepts the lifecycle request
- **THEN** the request is correlated to one canonical Operation and no separate install command writes the target

### Requirement: [P1-WRITE-005] Intent Idempotency and Evidence Chain
RuntimeIntent idempotency SHALL be scoped to the authenticated tenant, intent
kind, action and client key. The platform SHALL compute and persist a canonical
semantic request digest before commitment. An exact semantic replay SHALL
return the original HTTP status and the original intent, plan and Operation
identifiers and result representation without creating another RuntimeIntent,
ExecutionPlan, Operation, step, audit decision, outbox record or Provider
command; reuse of the same scoped key with a different semantic digest SHALL be
rejected as a conflict. apiserver and platform-api SHALL enforce the same
idempotency key and digest semantics across the BFF boundary. Audit evidence
SHALL link the preserved actor, intent digest, Release, target, ExecutionPlan,
policy, approval, Operation, Provider steps, and terminal outcome.

**Traceability:** CONTRACT-003, OP-006, P0-BASE-003, KERNEL-016, KERNEL-018

#### Scenario: Idempotency key is reused with a different target
- **GIVEN** a tenant has committed an install intent under an idempotency key
- **WHEN** the same key is submitted with a different target reference
- **THEN** the platform rejects the semantic conflict and does not create a second Operation

#### Scenario: Exact browser replay crosses the BFF boundary
- **GIVEN** apiserver has successfully submitted a cluster action and received its RuntimeIntent and Operation result
- **WHEN** the same actor repeats the semantically identical action with the same idempotency key
- **THEN** the browser receives the original status and result identifiers
- **AND** neither service creates or publishes any additional execution work

### Requirement: [PAG-001] Operation 提交事务
platform-api SHALL 在单个数据库事务内持久化 ExecutionPlan、Operation、全部 OperationStep、创建审计记录和 Read Model 投影；对初始状态为 queued 的 Operation，SHALL 在同一事务内为每个无依赖（ready）Step 写入一条 `hnb.command.operation.step-requested.v1` outbox 待发布事件。platform-api SHALL NOT 直连 NATS，也 SHALL NOT 发起执行态状态迁移。

**Traceability:** KERNEL-002, OP-007

#### Scenario: 提交低风险 Operation
- **GIVEN** 调用方提交 operationType 为 deploy 且 steps 定义合法的请求
- **WHEN** platform-api 完成提交
- **THEN** Operation 以 queued 状态持久化并返回 HTTP 201
- **AND** 同一事务内存在每个无依赖 Step 的 step-requested outbox 事件

#### Scenario: 幂等重复提交
- **GIVEN** 同一 tenant_id 下已存在相同 idempotency_key 的 Operation
- **WHEN** 调用方以相同 idempotency_key 再次提交
- **THEN** platform-api 返回 HTTP 200 与已有 Operation
- **AND** 不创建新的 Operation、Step 或 outbox 事件

### Requirement: [PAG-002] 高风险 Operation 审批门控
operationType 为 delete、rollback 或 config_change 的 Operation SHALL 以 pending_approval 初始状态创建；approve 接口 SHALL 仅对 pending_approval 状态生效，批准后转为 queued 并为 ready Steps 补发 step-requested outbox 事件；reject 接口 SHALL 将其转为 cancelled；所有审批动作 SHALL 写入 operation_audit 并记录审批人。

**Traceability:** KERNEL-002, TENANT-003

#### Scenario: 批准后进入队列
- **GIVEN** 一个 pending_approval 状态的 delete Operation
- **WHEN** 审批人调用 approve
- **THEN** Operation 转为 queued 且 approved_by 被记录
- **AND** operation_audit 新增 approved 事件
- **AND** ready Steps 的 outbox 事件在同一事务内写入

#### Scenario: 非待审批状态拒绝审批
- **GIVEN** 一个 queued 状态的 Operation
- **WHEN** 调用方调用 approve 或 reject
- **THEN** platform-api 返回 HTTP 409
- **AND** Operation 状态保持不变

### Requirement: [PAG-003] Operation 取消语义
cancel 接口 SHALL 仅允许从 validTransitions 定义的可取消源状态（pending、pending_approval、queued、paused、compensating）迁移到 cancelled；终态及 in_progress、queued_offline 状态 SHALL 拒绝取消并返回 HTTP 409；取消 SHALL 写入 operation_audit 并更新 Read Model。

**Traceability:** KERNEL-002

#### Scenario: 取消排队中的 Operation
- **GIVEN** 一个 queued 状态的 Operation
- **WHEN** 发起人调用 cancel
- **THEN** Operation 转为 cancelled，Read Model 同步更新
- **AND** 后续到达的 step-requested 命令被 worker 识别为过期并确认

#### Scenario: 终态不可取消
- **GIVEN** 一个 succeeded 状态的 Operation
- **WHEN** 调用方调用 cancel
- **THEN** platform-api 返回 HTTP 409

### Requirement: [PAG-004] 只读查询出口
列表与详情查询 SHALL 只读 operation_read_model（详情附带 operation_steps），SHALL 按 tenant_id 过滤并支持状态、类型过滤与分页；每个响应 SHALL 携带 last_state_changed_at 与 lastObservedAt 字段。

**Traceability:** KERNEL-003, TENANT-002

#### Scenario: 按租户分页查询
- **GIVEN** tenant-a 下存在多个不同状态的 Operation
- **WHEN** 调用方查询 `GET /v1/operations?tenant_id=tenant-a&status=queued&limit=50`
- **THEN** 响应仅包含 tenant-a 的 queued Operation，且每项携带 lastObservedAt
- **AND** 查询不访问写侧 operations 表

### Requirement: [PAG-005] 租户隔离与 Secret 约束
所有写请求 SHALL 要求显式 tenant_id，所有查询 SHALL 要求 tenant_id 参数；跨租户访问 SHALL 一律返回 HTTP 404 拒绝。请求中的敏感配置 SHALL 仅以 secretReference 字符串传递；platform-api SHALL 拒绝疑似明文 Secret 的输入键（如 password、token、private_key）与私钥材料，且 SHALL NOT 在日志或错误响应中输出请求体内容。

**Traceability:** TENANT-002, CFG-002

#### Scenario: 跨租户访问被拒绝
- **GIVEN** tenant-a 创建的 Operation
- **WHEN** tenant-b 查询详情或调用 cancel
- **THEN** platform-api 返回 HTTP 404
- **AND** Operation 状态与审计不受影响

#### Scenario: 明文 Secret 被拒绝
- **GIVEN** 提交请求中 step inputs 包含名为 dbPassword 的键
- **WHEN** platform-api 校验请求
- **THEN** 请求被拒绝并返回 HTTP 400
- **AND** 提示改用 secretReference

### Requirement: [KERNEL-016] Northbound and domain API separation
The platform SHALL route Web Console and ordinary user CLI traffic through
apiserver as the northbound API/BFF. Browser cluster reads, RuntimeIntent
submissions, Operation reads and Operation actions SHALL use versioned
browser-facing apiserver contracts; a browser SHALL NOT call platform-api or
NATS directly. platform-api SHALL own platform resource, RuntimeTarget,
RuntimeIntent, ExecutionPlan, Provider catalog and Operation domain records.
apiserver SHALL call platform-api only through versioned service APIs and SHALL
NOT read or write platform-api-owned tables, share a database access path,
publish execution commands to NATS, invoke Providers or RuntimeTargets, or
otherwise bypass RuntimeIntent, immutable planning, Operation commitment and
worker execution. Loss of platform-api connectivity SHALL fail closed and
SHALL NOT enable a database, message-bus or direct-execution fallback.

**Traceability:** KERNEL-001, KERNEL-002, CONTRACT-001, CONTRACT-008, TENANT-005, P1-WRITE-001

#### Scenario: Browser lists clusters
- **GIVEN** a browser requests the cluster list
- **WHEN** the request enters HNB
- **THEN** it is authenticated and context-enriched by apiserver
- **AND** platform-api performs resource-level authorization before returning cluster domain data

#### Scenario: Browser submits a cluster lifecycle action
- **GIVEN** an authenticated browser requests cluster upgrade through apiserver
- **WHEN** apiserver accepts the browser-facing action contract
- **THEN** apiserver translates it to the corresponding typed RuntimeIntent and submits it through the versioned platform-api service contract
- **AND** the browser receives an Operation reference rather than an execution command or Provider endpoint
- **AND** neither the browser nor apiserver writes execution state or publishes a step command

#### Scenario: apiserver lacks platform-api connectivity
- **GIVEN** apiserver cannot reach platform-api
- **WHEN** a request requires platform resource domain state
- **THEN** apiserver returns a bounded upstream RFC 9457 Problem Details response with trace ID
- **AND** it does not read or write platform-api-owned tables or use NATS or direct execution as an untracked fallback

#### Scenario: Browser attempts a direct internal connection
- **GIVEN** browser code attempts to call platform-api or subscribe to NATS
- **WHEN** network and service authorization policies evaluate the connection
- **THEN** the connection is denied
- **AND** no platform domain data, event stream or execution capability is exposed

### Requirement: [KERNEL-017] One-way synchronous dependency
platform-api SHALL NOT synchronously call apiserver to complete domain logic. Domain state changes SHALL be exposed through persistent state, Outbox and NATS events, and apiserver MAY consume those events to invalidate BFF caches or notify clients.

**Traceability:** KERNEL-003, CONTRACT-004, CONTRACT-005

#### Scenario: Cluster status changes
- **GIVEN** platform-api updates a cluster status
- **WHEN** the update commits
- **THEN** a domain event or read-model update is emitted through the approved asynchronous path
- **AND** platform-api does not make an HTTP callback to apiserver

### Requirement: [KERNEL-018] Dual-layer action authorization and actor-preserving service identity
apiserver SHALL authenticate the human actor, derive tenant and scope from the
trusted session, and authorize each cluster route against the specific
permission `cluster:list`, `cluster:read`, `cluster:create`, `cluster:update` or
`cluster:delete`. RuntimeIntent actions SHALL map to one of those permissions;
the browser SHALL NOT be required to hold unrelated generic permissions such
as `cluster:execute` or `intent:create`. apiserver SHALL call platform-api with
an authenticated trusted service identity and integrity-protected delegation
context that preserves the original actor ID, tenant, scope, correlation ID and
authorization evidence. platform-api SHALL authenticate the service,
independently authorize the actor for the requested action and resource
instance, validate the state transition, and record both service and actor in
audit evidence. Service identity SHALL NOT replace, widen or fabricate the
actor's authority.

**Traceability:** TENANT-005, TENANT-006, KERNEL-002, SEC-001, AUDIT-001

#### Scenario: User has route permission but not resource ownership
- **GIVEN** a user has `cluster:read` permission in tenant A
- **WHEN** they request a cluster outside tenant A through apiserver
- **THEN** platform-api denies the resource-level request
- **AND** no domain data for the foreign tenant is returned

#### Scenario: Upgrade permission is evaluated specifically
- **GIVEN** an actor has `cluster:read` but not `cluster:update` for a target
- **WHEN** the actor requests `UpgradeRuntimeTarget` through apiserver
- **THEN** apiserver denies the action and platform-api would independently deny it if reached
- **AND** possession or absence of `cluster:execute` or `intent:create` does not alter that decision

#### Scenario: Trusted service call preserves the actor
- **GIVEN** apiserver submits an authorized cluster create action using its service credential
- **WHEN** platform-api commits the resulting Operation
- **THEN** platform-api verifies the service identity and protected delegation context
- **AND** audit evidence identifies apiserver as caller and the original human as actor
- **AND** policy and authorization decisions are evaluated for the original actor and selected tenant

#### Scenario: Delegation context is missing or forged
- **GIVEN** a service-authenticated request lacks valid actor delegation evidence
- **WHEN** platform-api evaluates the request
- **THEN** platform-api rejects it before domain mutation
- **AND** no RuntimeIntent, Operation, step or execution event is created

### Requirement: [KERNEL-019] Authoritative target, scope and freshness validation
Before accepting a cluster RuntimeIntent, platform-api SHALL resolve all target,
parent-scope, endpoint and SecretReference identifiers from authoritative
server-owned state and SHALL validate tenant ownership, target kind, actor
scope, action compatibility, expected version and reference ownership. For an
action against an existing target, platform-api SHALL evaluate lifecycle,
connectivity, capability snapshot and observation freshness against the policy
version used for planning. A stale target SHALL be explicitly allowed with
recorded risk confirmation, routed to approval or queued-offline semantics, or
rejected according to server policy; a browser-provided status or risk decision
SHALL NOT override authoritative state. All validation and policy evidence
SHALL be pinned into the immutable ExecutionPlan before execution.

**Traceability:** RT-003, RT-004, RT-005, TENANT-002, P1-WRITE-002, KERNEL-018

#### Scenario: Cross-tenant target reference is submitted
- **GIVEN** an actor in tenant A submits an upgrade intent referencing a target owned by tenant B
- **WHEN** platform-api resolves the target and authorization scope
- **THEN** platform-api returns not found or denied according to the non-disclosure policy
- **AND** no plan, Operation, Provider call or target write is produced

#### Scenario: Stale target requires explicit policy treatment
- **GIVEN** a target's authoritative `lastObservedAt` exceeds the configured freshness threshold
- **WHEN** an authorized actor submits an upgrade with the required risk confirmation
- **THEN** platform-api evaluates the current server policy and records the freshness, confirmation and policy decision
- **AND** the intent is allowed, approval-gated, queued offline or rejected exactly as that policy decides

#### Scenario: Client status conflicts with authoritative state
- **GIVEN** a browser labels a target healthy but the authoritative target state is stale and disconnected
- **WHEN** platform-api validates the action
- **THEN** platform-api ignores the browser-provided status for authorization and planning
- **AND** execution cannot bypass the authoritative freshness decision

### Requirement: [KERNEL-020] RFC 9457 Problem Details across the BFF boundary
All non-success HTTP responses from browser-facing cluster, RuntimeIntent and Operation BFF endpoints SHALL use
`application/problem+json` and conform to RFC
9457 Problem Details. Responses SHALL contain `type`, `title`, `status` and a
stable extension `code`, SHALL carry `detail` and `instance` when safe and
applicable, and SHALL include the request trace identifier and structured field
violations where relevant. apiserver SHALL preserve a safe platform-api domain
problem's status, stable code and trace correlation; transport failures SHALL
be mapped to a distinct bounded upstream problem. Problems and logs SHALL NOT
contain Secret values, credentials, raw request bodies, internal stack traces
or existence details forbidden by tenant non-disclosure policy.

**Traceability:** CONTRACT-001, CONTRACT-007, CONTRACT-008, SEC-001, KERNEL-016

#### Scenario: RuntimeIntent validation fails
- **GIVEN** a browser submits a cluster action with an invalid field
- **WHEN** platform-api rejects the typed RuntimeIntent through apiserver
- **THEN** the browser receives RFC 9457 Problem Details with the original domain status, stable code, trace identifier and safe field violation
- **AND** no execution side effect occurs and no sensitive input is echoed

#### Scenario: platform-api is unavailable
- **GIVEN** apiserver cannot establish the platform-api service call
- **WHEN** a browser requests an Operation detail
- **THEN** apiserver returns a bounded upstream Problem Details response with a retryable service-unavailable code and trace identifier
- **AND** it does not misrepresent the failure as a missing Operation or consult a shared database

### Requirement: [KERNEL-021] Browser Operation BFF surface and action boundary
apiserver SHALL expose versioned browser-facing Operation list, detail and
allowed-action endpoints backed by platform-api domain APIs and Read Models.
List and detail responses SHALL be tenant-scoped and action availability SHALL
be derived from current Operation state plus actor-specific authorization.
Approve, reject and cancel SHALL be explicit action endpoints, SHALL preserve
idempotency and actor context, and SHALL delegate the authoritative transition
to platform-api; apiserver SHALL NOT mutate Operation state, steps, read models
or outbox records. Cluster lifecycle endpoints SHALL return or link the
canonical Operation ID so the same Operation can be opened in Operation Center.

**Traceability:** PAG-002, PAG-003, PAG-004, KERNEL-016, KERNEL-018, UX-022, UX-023

#### Scenario: Browser opens an Operation from a cluster action
- **GIVEN** a cluster lifecycle RuntimeIntent has committed an Operation
- **WHEN** the browser follows the returned Operation reference
- **THEN** apiserver returns the tenant-authorized Operation detail and steps from the platform-api read contract
- **AND** the detail identifies the associated intent, target, timestamps, current state and actions currently allowed for that actor

#### Scenario: Browser cancels a queued Operation
- **GIVEN** a queued Operation is cancellable and the actor is authorized for its originating cluster action
- **WHEN** the actor calls the Operation BFF cancel endpoint
- **THEN** apiserver forwards an actor-preserving idempotent action to platform-api
- **AND** platform-api performs the authoritative transition and audit write
- **AND** apiserver does not write Operation tables or publish an execution message

#### Scenario: Cross-tenant Operation detail is requested
- **GIVEN** an Operation belongs to tenant A
- **WHEN** an actor scoped only to tenant B requests it through the BFF
- **THEN** both BFF scoping and platform-api instance authorization prevent disclosure
- **AND** the response follows the tenant non-disclosure Problem Details policy

### Requirement: [KERNEL-022] Operation Read Model polling authority and optional SSE acceleration
The platform-api Operation Read Model SHALL remain the authoritative source for
browser-visible Operation state and allowed actions. The Web Console SHALL
poll the Operation BFF with bounded exponential backoff, stop or substantially
reduce polling at terminal states, and re-read after reconnect, visibility
resume or action completion. apiserver MAY expose an authenticated,
tenant-filtered SSE endpoint as an optional acceleration signal, but SSE events
SHALL contain only authorized version/cursor or invalidation data and SHALL NOT
be the sole state authority. Event loss, duplication, reordering or disconnect
SHALL be repaired by re-reading the Operation BFF; browsers SHALL NOT subscribe
to NATS or infer a successful terminal state solely from SSE.

**Traceability:** KERNEL-003, PAG-004, CONTRACT-005, KERNEL-016, KERNEL-021

#### Scenario: Operation progresses without SSE
- **GIVEN** SSE is disabled or disconnected after an Operation is submitted
- **WHEN** the browser tracks the Operation
- **THEN** bounded polling of the Operation BFF observes authoritative Read Model transitions through terminal state
- **AND** the user can open the same Operation in Operation Center without direct platform-api or NATS access

#### Scenario: SSE notification is duplicated or reordered
- **GIVEN** the browser receives duplicated or out-of-order notifications for an Operation
- **WHEN** it handles the notification
- **THEN** the browser treats it as a prompt to re-read the Operation BFF
- **AND** the Read Model version and state replace any speculative client state

#### Scenario: SSE attempts to expose another tenant
- **GIVEN** an actor has an authenticated SSE connection scoped to tenant A
- **WHEN** an Operation changes in tenant B
- **THEN** apiserver emits no tenant B notification or payload on that connection
- **AND** the browser cannot use the stream to discover the foreign Operation

