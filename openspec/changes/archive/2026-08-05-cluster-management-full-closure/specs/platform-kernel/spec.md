## MODIFIED Requirements

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

## ADDED Requirements

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
