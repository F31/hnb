## Context

已归档的 `web-resource-cluster-management` change 建立了列表、详情、向导和 RuntimeIntent 骨架，但当前实现尚未形成可验收的闭环：

- `cmd/apiserver/internal/handler/resource_cluster.go` 仍在内存中完成状态过滤、计数和分页，且把新鲜度过期直接折叠为单一 `STALE` 状态；
- `runtime_target_nodes` 已存在，但没有来自 Agent 或 CloudCore 的规范化观测事件、幂等投影和删除收敛机制；
- 集群 RuntimeIntent 可以进入 Operation Engine，但缺少按目标类型解析并固化的 lifecycle Provider 步骤；
- platform-api 已有 `/v1/operations` Read Model API，浏览器侧尚无完整的 Operation BFF 与 Operation Center 跟踪闭环；
- `PageRenderer` 已能校验 PageSchema、注册 DataSource、隔离区块错误和执行 Action，但资源页面并未完整由 L2 Schema 驱动，且 Renderer 内仍存在原生按钮等绕过 `@hnb/ui-kit` 的通用 UI；
- `cluster-agent` 已有受信身份、WebSocket 隧道和 heartbeat；`edge-provider` 已有 CloudCore client/discovery；两者都应复用，而不是新增浏览器直连或第二套采集通道；
- Kubernetes Provider、Edge Provider 和 operation-worker 已统一使用 `POST /v2/steps:execute`、execution attempt 与单调 fencing generation，应在该契约上扩展 lifecycle Step，禁止另建同步写路径。

本 change 采用“full closure”：只有从观测进入 Read Model、Schema 页面展示、动作级授权、STALE 服务端确认、RuntimeIntent 规划、Provider 执行、Operation 终态到审计/可观测全部通过，能力才视为完成。

## Goals / Non-Goals

### Goals

- 集群页只展示和操作 `KubernetesTarget` 与 `EdgeRuntimeTarget`；统一外部标识为 `targetId`、`targetKind`。
- 生命周期、健康、连接性和新鲜度分维建模；`STALE` 是 freshness，不覆盖最后已知 lifecycle/health/connectivity。
- 列表、详情、节点和字典均由租户隔离的 Read Model 提供数据库级过滤、精确计数和分页。
- KubernetesTarget 观测复用 Agent tunnel，EdgeRuntimeTarget 观测复用 Edge Provider 的 CloudCore discovery，并通过统一事件投影。
- 所有 create/import/upgrade/unmanage 写动作只经 RuntimeIntent、不可变 ExecutionPlan、Operation 和 Provider HTTP v2 执行。
- 所有 STALE 写动作必须在服务端重新判定，并由用户显式确认；确认后服务端策略仍可允许、转审批或拒绝。
- 复用 platform-api Operation Read Model/API，通过浏览器可用的 BFF 提供 Operation Center 列表、详情和终态跟踪。
- 列表和详情实际由 `PageRenderer` 消费 L2 PageSchema；向导、节点面板和 Operation 跟踪保留为 L3 注册组件。
- 页面与 Schema Engine 必须复用 `@hnb/ui-kit`。通用组件能力不足时，先增强并测试 ui-kit，再由页面复用；不得在资源插件内复制通用按钮、表格、分页、确认框、状态、空态、错误态或骨架屏。
- OpenAPI/JSON Schema 为权威契约并生成 TypeScript/Go 类型；完成契约、数据库、Provider、BFF、Vue、Playwright、故障注入和 live-stack 验收。

### Non-Goals

- 不把 `ContainerEngineTarget`、`ExternalServiceConnector` 或 Karmada member registry 展示为集群；`ContainerEngineTarget` 后续进入独立运行时目标页面。
- 不实现命名空间、工作负载、网络、存储、GPU、cordon/drain、备份或集群内资源 CRUD。
- 不实现联邦调度、Placement、DR Placement 或集群拓扑编辑。
- 不允许 Web Console 直连 Provider、Agent、CloudCore、Kubernetes API 或 NATS。
- 不引入新数据库、中间件、第二套 Operation 状态机或 Provider v1 兼容路径。

## Architecture

```text
                              Query / presentation
 Browser
   | bootstrap, PageSchema, dictionaries, cluster/operation queries
   v
 apiserver BFF ------------------------------------------------------+
   | trusted service identity + original actor/tenant context       |
   v                                                               |
 PostgreSQL Read Models <--- idempotent projectors <--- JetStream   |
   |  cluster targets/nodes        ^                    ^            |
   |  operation_read_model         |                    |            |
   +-------------------------------+                    |            |
                                                        |            |
 KubernetesTarget                                      | EdgeRuntimeTarget
 cluster-agent -- mTLS/WebSocket tunnel --> Agent GW --+-- edge-provider
      | typed observations                                 | CloudCore discovery
      + no browser/direct inbound access                   + no EdgeCore direct access

                               Command / execution
 Browser -- RuntimeIntent + idempotency/correlation --> apiserver BFF
   --> platform-api /v1/intents --> immutable ExecutionPlan --> Operation
   --> Outbox/JetStream --> operation-worker
   --> Provider HTTP v2 /v2/steps:execute
          | Kubernetes lifecycle Provider
          + Edge lifecycle Provider --> CloudCore/KubeEdge
   --> Operation events/read model --> BFF polling --> Operation Center

 Web rendering
 PageSchema --> PageRenderer --> trusted ComponentRegistry/DataSource/ActionEngine
                              --> @hnb/ui-kit primitives
                              --> L3 domain components where required
```

PostgreSQL remains authoritative for intents, plans, operations, audit and projections. JetStream transports events but is not queried by the browser. Read requests never fan out to targets; write requests never mutate `runtime_targets` directly.

## Decisions

### 1. Cluster scope is exactly two target kinds

The public cluster contract accepts only `KubernetesTarget` and `EdgeRuntimeTarget`. All list queries include this predicate even when no `targetKind` filter is supplied. Detail and node endpoints return `404` for other target types to avoid exposing a target through an incompatible cluster model.

The database may retain existing `runtime_targets.target_type` values. Contract mapping is one-way:

| Database value | Public `targetKind` | Cluster page |
|---|---|---|
| `kubernetes` | `KubernetesTarget` | included |
| `edge_runtime` | `EdgeRuntimeTarget` | included |
| `container_engine` | none | excluded |
| `external_service` | none | excluded |

### 2. State dimensions are orthogonal

`lifecycleState`, `healthState`, `connectivityState`, and `freshness` are independently projected. The UI may give STALE a prominent warning badge, but must continue to show the last known values and `lastKnownStateAt`. No projector rewrites lifecycle or health merely because time elapsed.

Freshness is calculated from the target's configured threshold and latest accepted observation at query/projection time. A periodic sweeper may materialize it for indexed filtering, but the derivation remains deterministic.

### 3. STALE writes require explicit confirmation and server policy

Client-side button conditions are advisory only. For every upgrade or unmanage request, platform-api reloads the target under the trusted tenant, evaluates freshness and action policy, and records the decision.

If the target is STALE and `spec.riskConfirmation` is absent or does not match the current observation, submission fails with RFC 9457 type `stale-confirmation-required`. The response includes only non-secret confirmation context: `targetId`, action, `lastKnownStateAt`, current lifecycle/health/connectivity, policy outcome class and a short-lived opaque `confirmationToken` bound to tenant, actor, target, action, normalized intent digest and observation version.

The UI must render that context in a `@hnb/ui-kit` confirmation component and require an explicit user action. Resubmission carries `riskConfirmation.confirmationToken`, `acknowledged: true` and an optional bounded reason. The server verifies token binding/expiry and reevaluates current state. A valid confirmation is necessary but not sufficient: policy returns one of `allow`, `require_approval`, or `deny`; the latter two create `pending_approval` or reject respectively. Confirmation and policy result are written to security audit without token contents.

The initial `stale-confirmation-required` response creates no Intent/Plan/Operation and does not reserve the idempotency key. The confirmed submission uses the original key; replay of an accepted normalized intent returns the existing record, while a different semantic digest returns `409 idempotency-conflict`.

### 4. Existing observation transports feed one canonical projector

- KubernetesTarget: extend the authenticated Agent tunnel with typed target, capability and node observations. Do not create a public agent ingest endpoint or use the unfinished generic kube-proxy path for discovery.
- EdgeRuntimeTarget: run bounded discovery through the existing Edge Provider CloudCore client. CloudCore is the source of edge node state; HNB never connects directly to EdgeCore and does not install HNB Agent on KubeEdge nodes.
- Both producers publish the same versioned EventEnvelope payloads through the existing Transactional Outbox/JetStream path. Projectors deduplicate and reject older source revisions.

### 5. Lifecycle execution reuses Operation and Provider HTTP v2

The planner resolves a lifecycle-capable Provider from `provider_registry`, target kind, tenant, capability and compatibility matrix. It freezes Provider ID/version, target identity, action, approved parameters, SecretReferences and compatibility evidence into the immutable ExecutionPlan. Endpoint addresses and bearer credentials remain operation-worker configuration, not user input or plan data.

Lifecycle Step types are namespaced and allowlisted, for example `runtime_target.kubernetes.create`, `runtime_target.kubernetes.import`, `runtime_target.kubernetes.upgrade`, `runtime_target.kubernetes.unmanage`, and corresponding `runtime_target.edge.*` steps. Provider calls continue to use schema `2.0.0`, idempotency, execution attempt, checkpoint and fencing generation. Arbitrary commands, URLs and provider IDs are rejected.

### 6. Browser operation access is a BFF projection, not a second engine

apiserver adds tenant-scoped proxies for platform-api `GET /v1/operations` and `GET /v1/operations/{id}`. It forwards a trusted service identity and original actor audit context; it never accepts browser-supplied tenant identity. The first UI release polls with backoff and stops at terminal state. SSE may later accelerate updates, but polling remains the correctness path.

### 7. L2 Schema is real, L3 remains bounded, ui-kit is mandatory

Cluster list/detail routes load registered PageSchema and pass it to the existing `PageRenderer`, `ComponentRegistry`, `DataSourceManager`, `ActionEngine` and condition context. The Schema may reference only registered `componentType`, `endpointId`, `actionId`, dictionaries and extension point `resource.cluster.detail.tabs`.

`ClusterRegisterWizard`, node panel and operation progress are L3 domain components because they contain specialized workflows. They still compose `@hnb/ui-kit`. Before adding page-local UI, implementation must inventory ui-kit. If a generally reusable capability is missing, such as modal confirmation, tabs, pagination, alert or operation progress, it is added to `web/packages/ui-kit` with tokens, accessibility and tests first. `PageRenderer` itself must use ui-kit primitives rather than native styled controls.

### 8. Capability and navigation are fail-closed

apiserver bootstrap decides whether the menu, route and PageSchema are available based on deployed contract/schema/provider/projector capabilities. Missing or incompatible capability removes the entry and rejects direct route actions. Build-time flags are deployment overrides only and cannot enable a server-disabled capability.

## Data Model

The implementation evolves the existing `runtime_targets`, `capability_snapshots`, `runtime_target_nodes`, `runtime_intents`, `execution_plans`, `operations`, `operation_read_model`, `security_audit_events` and outbox tables. Exact migration names are assigned during implementation; no new persistence service is introduced.

### Target projection

`runtime_targets` gains or normalizes these projection fields:

| Field | Meaning |
|---|---|
| `id` | canonical `targetId`; unchanged UUID |
| `target_type` | internal type; cluster queries allow only `kubernetes`, `edge_runtime` |
| `lifecycle_state` | last accepted lifecycle state |
| `health_state` | last accepted health result |
| `connectivity_state` | last accepted transport/discovery connectivity |
| `observed_at` | producer observation time of latest accepted target observation |
| `last_known_state_at` | time represented by last complete state snapshot |
| `observation_source` | `agent` or `cloudcore` |
| `observation_source_id` | stable producer instance/epoch identity |
| `observation_revision` | monotonic revision within source identity |
| `stale_threshold_seconds` | existing per-target freshness threshold |
| `projection_version` | optimistic projection revision |

`freshness` is `FRESH` when `now - observed_at <= stale_threshold_seconds`, otherwise `STALE`; absent observation is STALE. If materialized for filtering, it must be maintained by the observation projector and freshness sweeper and must not be used as a lifecycle source.

### Capability and node projections

- `capability_snapshots` remains append-only/versioned per target; add source identity/revision and a digest unique per target/source/revision.
- `runtime_target_nodes` keeps one current row per `(target_id, source_node_uid)` and adds `tenant_id`, stable `source_node_uid`, `observed_at`, `last_known_state_at`, source identity/revision, labels/capacity as bounded JSON, and `deleted_at` for snapshot-based disappearance.
- Tenant is denormalized into node rows for database-level isolation and indexed with target/name/status. A composite FK `(target_id, tenant_id)` references `runtime_targets`.
- A node is marked absent only after a complete snapshot with a greater revision omits it; partial snapshots never delete unseen nodes.

### Projection inbox/cursor

An observation inbox or cursor table records `(tenant_id, target_id, source, source_id, revision, message_id, payload_digest, processed_at)`. The tuple is unique. Equal revision/equal digest is an idempotent replay; equal revision/different digest is a producer protocol error; lower revisions are ignored and measured. Source epoch changes require an authenticated registration/reset event and may not silently reset ordering.

### Intent, plan and operation linkage

- `runtime_intents.runtime_target_id` and all target lifecycle plans must use the tenant composite FK already established by migration 025.
- ExecutionPlan stores `targetId`, `targetKind`, provider ID/version, compatibility decision, observation version used for policy, normalized `riskConfirmation` evidence reference, SecretReferences and lifecycle steps. It never stores secret values or a caller-selected endpoint.
- `operations.tags`/`operation_read_model.tags` include non-secret `targetId`, `targetKind`, `intentKind` for Operation Center filtering and deep links.
- STALE confirmation and policy decisions are append-only `security_audit_events`; raw opaque confirmation tokens are not persisted.

## API Contracts

Contracts are published under `contracts/openapi` and `contracts/schema`, validated in CI, and used to generate Go/TypeScript types. BFF responses use `apiVersion: ui.hnb.io/v1`; errors use RFC 9457 `application/problem+json` with stable `type`/`code`, correlation ID and safe extensions.

### Cluster Read Model BFF

```http
GET /api/v1/resources/clusters?page&pageSize&keyword&targetKind&lifecycleState&healthState&connectivityState&freshness
GET /api/v1/resources/clusters/{targetId}
GET /api/v1/resources/clusters/{targetId}/nodes?page&pageSize&keyword&status
GET /api/v1/dictionaries/resource.cluster.lifecycle
GET /api/v1/dictionaries/resource.cluster.health
GET /api/v1/dictionaries/resource.cluster.connectivity
GET /api/v1/dictionaries/resource.cluster.freshness
```

List semantics:

- `page >= 1`; default `pageSize=20`, maximum 100; node maximum 200.
- Filtering, `COUNT(*)`, ordering and `LIMIT/OFFSET` occur in PostgreSQL, not after a scan cap.
- Stable order is `(updatedAt DESC, targetId DESC)` unless the contract later adds an allowlisted sort.
- `targetKind` accepts only `KubernetesTarget` or `EdgeRuntimeTarget`; omitted means both, never all RuntimeTargets.
- Unknown filters are `400`; inaccessible or wrong-kind detail is `404` to avoid tenant/type enumeration.

Core response shape:

```json
{
  "targetId": "uuid",
  "targetKind": "KubernetesTarget",
  "displayName": "prod-a",
  "source": "created",
  "lifecycleState": "ACTIVE",
  "healthState": "HEALTHY",
  "connectivityState": "CONNECTED",
  "freshness": "FRESH",
  "observedAt": "2026-08-01T12:00:00Z",
  "lastKnownStateAt": "2026-08-01T12:00:00Z",
  "staleThresholdSeconds": 300,
  "runtimeVersion": "v1.x",
  "nodeCount": 3,
  "capabilitySnapshot": { "version": 7, "digest": "sha256:...", "observedAt": "..." },
  "createdAt": "...",
  "updatedAt": "..."
}
```

### RuntimeIntent BFF

```http
POST /api/v1/runtime-intents
Idempotency-Key: <1..128 ASCII>
X-Correlation-ID: <uuid>
Content-Type: application/json
```

Allowed cluster intent kinds are `CreateKubernetesTarget`, `ImportRuntimeTarget`, `UpgradeRuntimeTarget`, and `DeleteRuntimeTarget`. Header and body idempotency/correlation values must agree. BFF action authorization maps kinds to `cluster:create`, `cluster:update`, or `cluster:delete`; platform-api receives a trusted service identity plus immutable original actor/tenant/scope context.

`targetKind` is explicit for create/import and must be one of the two cluster kinds. Existing-target actions carry `targetId`; the server does not trust a tenant ID or Provider ID from the body. Credentials are only `SecretReference` objects. For a confirmed STALE action:

```json
{
  "riskConfirmation": {
    "acknowledged": true,
    "confirmationToken": "opaque",
    "reason": "bounded optional operator reason"
  }
}
```

Successful submission returns `202` with `intentId`, `executionPlanId`, `operationId`, normalized status and replay indicator. Accepted replay returns the same identifiers. `stale-confirmation-required`, `stale-confirmation-expired`, `stale-policy-denied`, `idempotency-conflict`, `secret-reference-denied`, `target-version-conflict`, and `provider-incompatible` are stable Problem Details codes.

### Operation BFF

```http
GET /api/v1/operations?page&pageSize&status&type&targetId
GET /api/v1/operations/{operationId}
```

These are projections of platform-api `/v1/operations`; they do not query operation write tables directly in the browser path. Detail includes steps, progress, safe failure code/message, timestamps, target deep link and audit-safe correlation IDs. Polling starts at 2 seconds, exponentially backs off to 15 seconds, pauses when hidden/offline, resumes with jitter, and stops at `succeeded`, `failed`, or `cancelled`.

## Event Contracts

All events use `contracts/schema/common/v1/event-envelope.schema.json`. Proposed message types are:

- `hnb.runtime-target.observed.v1`
- `hnb.runtime-target.capability-observed.v1`
- `hnb.runtime-target.nodes-observed.v1`
- `hnb.runtime-target.source-reset.v1`

Required envelope values include UUID `messageId`, message type/schema version, `occurredAt`, trusted `tenantId`, correlation/idempotency keys, `aggregateId=targetId`, and payload. Observation payloads require:

```json
{
  "targetId": "uuid",
  "targetKind": "KubernetesTarget",
  "source": "agent",
  "sourceId": "stable-instance-or-epoch",
  "revision": 42,
  "observedAt": "2026-08-01T12:00:00Z",
  "complete": true,
  "lifecycleState": "ACTIVE",
  "healthState": "HEALTHY",
  "connectivityState": "CONNECTED",
  "nodes": []
}
```

Event constraints:

- Producer identity is derived from Agent/Provider authentication and must agree with payload target/tenant.
- `targetKind` is immutable after target registration.
- `revision` is monotonic per `(targetId, source, sourceId)`; timestamps alone never order events.
- Node payload count and total bytes are bounded; large complete snapshots may use a trusted `payloadRef` with digest.
- Projector writes target, capability, nodes, cursor and projection outbox/audit in one transaction.
- Poison events enter bounded dead-letter handling with alerting; they are never silently acknowledged as projected.

Operation events remain the existing Operation Engine contract. This change consumes their Read Model and does not introduce a parallel operation event vocabulary.

## State Machines

### Target lifecycle

```text
REGISTERING --> PROVISIONING --> ACTIVE --> UPGRADING --> ACTIVE
      |              |             |           |
      +------------> FAILED <------+-----------+
                                     |
                                     v
                                 DELETING --> TERMINATED
```

Imported targets may transition `REGISTERING -> ACTIVE`. `FAILED` may retry through a new Operation into `PROVISIONING` or `UPGRADING`. `TERMINATED` is terminal for that target ID. Lifecycle changes originate from accepted Operation/provider outcomes or authenticated observations according to an explicit precedence rule; freshness never changes lifecycle.

### Health, connectivity and freshness

```text
health:       UNKNOWN <--> HEALTHY <--> DEGRADED <--> UNHEALTHY
connectivity: UNKNOWN <--> CONNECTED <--> DISCONNECTED
freshness:    FRESH -- threshold elapsed --> STALE -- newer accepted observation --> FRESH
```

Disconnect does not immediately imply STALE: the last observation remains fresh until its threshold expires. STALE does not imply unhealthy. The UI displays all dimensions rather than inventing a single merged truth.

### STALE write policy

```text
submit action
  --> server reloads tenant target and policy
       --> fresh: normal plan/operation path
       --> stale + no/mismatched confirmation: Problem Details + bound challenge
       --> stale + valid confirmation: reevaluate
              --> allow ------------> queued Operation
              --> require_approval -> pending_approval Operation
              --> deny -------------> rejected, audited, no side effect
```

### Operation tracking

The existing ten-state Operation machine remains authoritative:

```text
pending -> pending_approval -> queued -> in_progress -> succeeded
                    |            |           |-------> failed
                    |            +-> queued_offline
                    +-------------------------------> cancelled
in_progress <-> paused -> compensating -> failed/cancelled
```

## Failure Modes

| Failure | Required behavior |
|---|---|
| Agent disconnect or CloudCore unavailable | retain last known state, update connectivity when trustworthy, transition freshness by threshold, never fabricate health |
| Duplicate observation | cursor/digest makes replay a no-op |
| Out-of-order or conflicting revision | ignore older; dead-letter equal-revision/different-digest; emit metric/audit |
| Partial node snapshot interruption | retain unseen nodes; only a newer complete snapshot may mark absence |
| Projector unavailable | JetStream redelivery; expose projection lag; Read Model remains last known, eventually STALE |
| Read Model query failure | PageRenderer shows ui-kit block-level ErrorState and independent retry; no target fan-out fallback |
| Unknown Schema/component or invalid props | fail closed for affected region with safe ui-kit error placeholder; no arbitrary component/URL execution |
| Tenant switch with late response | generation/context key discards the response and cancels polling where possible |
| STALE confirmation expires or observation changes | reject before plan creation and request new confirmation |
| SecretReference missing/cross-tenant | reject before plan creation; never fetch or log secret value in BFF/UI |
| No compatible lifecycle Provider | `provider-incompatible`; no Operation side effect |
| Provider timeout/lost response | existing HTTP v2 idempotency, checkpoint and fencing recovery; never infer success from timeout |
| Provider returns wrong attempt/generation | protocol failure; worker cannot commit success |
| Operation polling fails | preserve operation link/last state, back off, allow manual retry; execution continues server-side |
| Capability/navigation service unavailable | fail closed: no menu/action publication; direct APIs still enforce auth |
| Database/JetStream outage | reject new writes safely; recover from PostgreSQL authority and outbox after service restoration |

## Alternatives Considered

| Alternative | Decision |
|---|---|
| Keep one `status` and map expired data to STALE | Rejected: destroys last known lifecycle/health information and causes invalid transitions |
| Include ContainerEngineTarget in cluster table | Rejected: it lacks the Kubernetes cluster/node semantics required by this page |
| Let UI confirmation alone authorize STALE writes | Rejected: UI is not a security boundary and state may change between render and submit |
| Always reject or always queue STALE writes | Rejected: deployment policy must choose allow/approval/deny, but explicit confirmation is mandatory whenever execution is permitted |
| Query Agent/CloudCore synchronously from list/detail | Rejected: violates CQRS, amplifies failures and leaks target latency into browser requests |
| Add direct cluster CRUD REST endpoints | Rejected: bypasses immutable Intent/Plan, Operation audit, idempotency and fencing |
| Add a new observation transport | Rejected: reuse Agent tunnel, CloudCore discovery, Outbox and JetStream |
| Fork operation state into the BFF | Rejected: platform Operation Read Model/API is authoritative |
| Build cluster pages entirely as custom Vue | Rejected: standard list/detail must exercise L2 PageSchema/PageRenderer; only specialized workflows use L3 |
| Implement missing primitives inside resource plugin | Rejected: improve `@hnb/ui-kit` first to prevent inconsistent and inaccessible duplicates |
| Support Provider HTTP v1/v2 together | Rejected: existing fencing design explicitly forbids mixed execution protocols |

## Security, Tenant, Secret and Audit

### Security and authorization

- Browser authentication terminates at apiserver. BFF derives tenant/actor/scope from trusted context and ignores spoofed body/query tenant values.
- Permissions are `cluster:list`, `cluster:read`, `cluster:create`, `cluster:update`, `cluster:delete`; action kind determines required permission. No unrelated browser `cluster:execute` or `intent:create` permission is required.
- platform-api authorizes the trusted service caller and validates delegated original actor context; target ownership and instance-level authorization are checked again.
- Provider endpoints, IDs, step types, PageSchema endpoints/actions and component types are registry/allowlist controlled. User input cannot select arbitrary URL, command, script or Provider.
- Confirmation tokens are short-lived, single-purpose, integrity protected and bound to actor, tenant, target, action, intent digest and observation version. They are not bearer authorization for any other action.
- Existing Provider HTTP v2 audience-bound service tokens, execution attempt IDs and fencing generations remain mandatory.

### Tenant isolation

- Every target, node, capability, cursor, intent, plan, operation and audit lookup includes trusted `tenant_id`; composite foreign keys prevent cross-tenant references.
- Caches, PageSchema snapshots, dictionary data where scoped, polling keys and frontend stores include tenant/context generation.
- Cross-tenant target references return non-enumerating errors. Tenant switch clears DataSources and Operation polling before loading the new context.

### Secrets

- kubeconfig, CloudCore credentials, service tokens and certificates are accepted only as `SecretReference`; UI may show provider/scope/name metadata but never values.
- Server verifies secret tenant/scope ownership, allowed provider, purpose and actor access before planning. Provider receives only runtime-resolved material through the established trusted mechanism; immutable plan stores references and versions, not plaintext.
- Secret values and confirmation tokens are excluded from logs, traces, metrics, events, Problem Details, operation output and audit detail. Rotation does not require editing stored target projection records.

### Audit

Append-only audit links actor, tenant, permission/policy version, target, intent, plan, operation, correlation and trace IDs. Required events include action allowed/denied, STALE challenge issued, explicit confirmation accepted/rejected, approval decision, SecretReference validation outcome, Provider resolution/compatibility decision, projection conflict, and terminal Operation outcome. Reasons are bounded and sanitized.

## Performance and Capacity

| Area | Initial target/budget |
|---|---|
| Cluster list BFF | P95 <= 500 ms at database, default 20/max 100 rows; exact count from indexed query |
| Cluster detail/node BFF | P95 <= 500 ms; node default 50/max 200 |
| PageSchema payload | <= 200 KiB per page revision |
| Console first useful render | <= 2.5 s on the standard internal network profile |
| Projection lag | P95 <= 10 s under normal load; alert at sustained > 60 s |
| Agent observation cadence | default 30 s heartbeat; full node snapshot cadence configurable and jittered |
| CloudCore discovery | reuse bounded configurable interval, default 60 s, with jitter/backoff |
| Operation polling | 2 s to 15 s backoff, terminal stop, hidden-tab pause; no SSE dependency |
| Read Model capacity | at least 10,000 cluster targets platform-wide and 5,000 edge nodes per CloudCore domain before release calibration |
| Event handling | bounded payload/node count, batch upserts, no per-node transaction or unbounded goroutine |

Indexes must support tenant + target kind + active + state/freshness + stable sort, and tenant + target + node sort/filter. Capacity tests calibrate actual thresholds and PostgreSQL query plans; the existing 2,000-row scan cap is removed rather than raised.

Backpressure applies at producers and consumers: Agent/CloudCore snapshots are jittered, projector concurrency is bounded, JetStream retention/dead-letter limits are explicit, and a target may coalesce superseded complete snapshots without changing revision semantics.

## Migration, Rollback, DR and Observability

### Schema/data migration

- Add nullable state/source/revision fields and projection indexes first; retain legacy `status` during the compatibility window.
- Backfill `targetKind` mapping only for `kubernetes` and `edge_runtime`. Derive conservative dimensions from legacy status: unknown values become `UNKNOWN`, never optimistic `HEALTHY`/`CONNECTED`.
- Backfill node tenant/source identifiers from owning targets; quarantine rows with broken ownership rather than guessing.
- Build new indexes concurrently where supported, then enable dual projection and compare old/new counts before switching reads.
- Existing intent/operation/audit history remains immutable. No history table is truncated or rewritten.

### Rollback

- Capability/navigation can disable the new UI and writes independently while leaving observations and Operation history intact.
- Before contract cutover, BFF may adapt the old raw Read Model to the new generated response during one migration window. New frontend code consumes only the generated contract.
- Database additions remain backward compatible during rollback; rollback code ignores new columns. Do not roll back by deleting target/node/operation/audit data.
- In-flight Operations are drained or allowed to reach terminal state before disabling lifecycle Provider routing. Never fall back to direct mutation or Provider v1.
- After a Provider v2 lifecycle side effect, recovery is roll-forward using the same fencing/idempotency contract.

### Disaster recovery

- PostgreSQL backups include runtime targets, observations/cursors, Read Models, intents/plans/operations/audit, outbox, leases and fencing sequence state.
- JetStream is rebuildable transport: after PostgreSQL restore, replay surviving events/outbox and rebuild cluster/operation projections from authoritative records or a controlled source resnapshot.
- Restore verifies fencing sequence monotonicity before workers resume. A reset/reused generation is fatal and dispatch remains stopped.
- Agent and CloudCore producers reconnect with jitter and issue authenticated source epoch/full snapshots; mass reconnect and 72-hour stale recovery are DR exercises.
- RPO/RTO inherit the platform PostgreSQL/JetStream profile and must be measured in live-stack drills, including one CloudCore replica/domain failure.

### Observability

- Metrics: BFF latency/error/count, filtered cardinality, query plan regressions, projector lag, accepted/duplicate/out-of-order/conflicting events, stale target count/age, Agent/CloudCore connectivity, STALE policy outcomes, Provider resolution/failures, Operation polling and terminal duration.
- Traces: propagate correlation/trace across browser -> BFF -> platform-api -> intent -> plan -> operation -> worker -> Provider; observation traces carry target/source but no secret.
- Logs: structured tenant/target/operation/source revision and safe error code; hash or omit sensitive subject identifiers as policy requires; never log request bodies containing secret refs/tokens indiscriminately.
- Alerts: projection lag/dead letter, observation conflict, mass STALE transition, Provider incompatibility/unavailability, Operation failure-rate increase, audit write failure and DR/fencing invariant failure.
- Dashboards link target freshness/connectivity to projector lag and Operation outcomes so source outage is distinguishable from target failure.

## Compatibility Matrix and Conformance Plan

### Compatibility matrix

| Boundary | Supported combination | Enforcement |
|---|---|---|
| Cluster page target kind | `KubernetesTarget`, `EdgeRuntimeTarget` only | OpenAPI enum, BFF SQL predicate, detail authorization, UI generated type |
| UI/PageSchema | `ui.hnb.io/v1`, current and previous stable readable revision | Schema validation and PageRenderer compatibility test |
| UI components | cluster pages + `PageRenderer` use current workspace `@hnb/ui-kit`; no private primitive forks | lint/import review, component tests, visual/a11y E2E |
| RuntimeIntent | `hnb.io/v1` cluster kinds with generated types | JSON Schema and BFF/platform contract tests |
| Worker/Provider | Provider HTTP `2.0.0` only | startup capability check and conformance suite |
| Kubernetes observation | registered cluster-agent version advertising observation v1 | tunnel handshake/capability gate |
| Edge observation | edge-provider observation v1 + compatible CloudCore/KubeEdge/CRD versions | provider compatibility matrix and discovery probe |
| Operation browser API | BFF v1 mapped from platform-api `/v1/operations` Read Model | consumer-driven contract test |
| Database | migrations 010/015/025/048 plus this change's additive migration | startup schema check/migration CI |

No exact Kubernetes, KubeEdge or CloudCore release is guessed in this design. Release engineering must pin tested versions in the repository compatibility data before enabling lifecycle actions. An unknown or expired matrix entry fails closed for writes while preserving read-only last-known views.

### Conformance plan

1. Contract generation validates OpenAPI/JSON Schema examples and produces clean Go/TypeScript output with no handwritten duplicate DTOs.
2. Database tests prove tenant predicates, exact count/pagination, two-kind exclusion, index-backed query plans, observation dedup/order/conflict and complete/partial node snapshot semantics.
3. Agent conformance authenticates target/tenant binding, reconnect/source epoch, revision monotonicity, bounded snapshots and no direct browser/edge-node path.
4. Edge conformance uses CloudCore discovery fixtures for node add/update/remove, disconnect, version skew, partial failure and resnapshot; it verifies no EdgeCore direct connection.
5. Provider conformance runs every lifecycle Step against HTTP v2, including replay, lost response, stale fencing, wrong echo, checkpoint resume, incompatible version, SecretReference failure and compensation behavior.
6. STALE policy tests cover no confirmation, expired/mismatched token, observation race, allow, approval, deny, idempotent replay and immutable audit evidence.
7. BFF tests cover action-permission mapping, delegated actor context, tenant spoofing, target ownership, Problem Details and upstream failure mapping.
8. Schema/UI tests prove list/detail are rendered through `PageRenderer`, only registered endpoints/components execute, and all general controls come from `@hnb/ui-kit`.
9. Playwright covers create/import/upgrade/unmanage to terminal Operation, Operation deep links, permission withdrawal, tenant switch/late responses, STALE confirmation, mobile layouts, keyboard/focus, screen-reader labels, empty/error/offline states and feature rollback.
10. Live-stack acceptance requires both target kinds, real Agent/CloudCore observations, Provider side effects, Operation terminal states, correlation traces, audit records, restart/replay and rollback/DR drills.

The exit criterion is not compilation or a `202` receipt: both target kinds must complete the full observation/read/action/operation/audit chain and all mandatory conformance suites must pass.

## Risks / Trade-offs

| Risk/trade-off | Mitigation |
|---|---|
| Orthogonal states increase contract/UI complexity | generated types, separate dictionaries and reusable ui-kit status composition; avoid lossy merged status |
| STALE confirmation adds a two-step interaction | server-bound context prevents blind execution; Operation Center keeps the asynchronous flow understandable |
| Policy may allow a confirmed action on old information | action-specific policy, approval option, observation/version binding, immutable audit and Provider-side preconditions |
| Agent/CloudCore producers have different semantics | normalize only shared facts, retain source metadata, test complete/partial snapshot behavior independently |
| Full snapshots can create database/event bursts | bounded payloads, jitter, batching, coalescing and capacity tests |
| Dual-read/dual-projection migration can diverge | shadow comparison metrics and a fail-closed cutover gate |
| Improving ui-kit expands initial scope | changes are limited to demonstrably general missing primitives and reduce long-term duplication/accessibility risk |
| Operation polling adds request load | backoff/jitter/visibility pause/terminal stop; optional SSE later without correctness dependence |
| Provider lifecycle behavior may differ by implementation | explicit namespaced Step contracts, compatibility matrix and provider conformance before capability publication |
| Additive rollback leaves unused columns/data | intentional safety trade-off; clean-up occurs only in a later proven migration, never during emergency rollback |

## Migration Plan

1. Finalize OpenAPI/JSON Schema, state dictionaries, observation schemas and generated types; pin compatibility entries and acceptance fixtures.
2. Improve missing general `@hnb/ui-kit` primitives and update `PageRenderer` to consume them; complete component, accessibility and visual tests before cluster-specific composition.
3. Apply additive database migration and indexes; backfill target/node ownership and conservative state dimensions; run invariant reports.
4. Deploy observation consumers/projectors dark, then enable Agent observation and Edge Provider CloudCore discovery producers. Compare projections without serving new reads.
5. Switch cluster BFF reads to database-level filtered projections, initially shadowing old responses; verify cardinality, freshness and tenant isolation.
6. Deploy platform lifecycle planning/provider routing and Provider HTTP v2 lifecycle implementations with writes capability-disabled; run provider and STALE policy conformance.
7. Add Operation BFF and Operation Center polling over the existing platform operation APIs/read model; verify deep links and terminal tracking.
8. Publish PageSchema/navigation to an internal tenant cohort. Enable read-only first, then create/import, then upgrade/unmanage after live observations and compatibility checks pass.
9. Run full Playwright/live-stack, failure injection, tenant switch, mobile/a11y, rollback and DR exercises for both target kinds.
10. Remove the old response adapter, legacy permission names and legacy single-status consumption only after the compatibility window and telemetry show no consumers. Legacy database columns are removed in a separate future change.

Each phase has an independent capability gate. A failed phase rolls back its publication or read route, preserves accepted observations/operations/audit, and never introduces a direct-write fallback.

## Open Questions

1. Which concrete Kubernetes, KubeEdge, CloudCore, EdgeCore and CRD versions will be pinned for the first enabled compatibility matrix rows?
2. What are the initial per-action STALE policy defaults and approval timeout for `upgrade` versus `unmanage` in each deployment tier?
3. Which service owns and signs short-lived STALE confirmation tokens: platform-api policy code or the shared policy service, and what is the approved TTL?
4. What bounded full-snapshot size triggers `payloadRef`, and which existing artifact/object path is approved for that payload without adding storage?
5. What measured PostgreSQL/JetStream RPO/RTO and projector-lag SLO are required for production capability publication?
