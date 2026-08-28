## ADDED Requirements

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
snapshot, Provider versions, policy results, approved parameters,
SecretReferences, and the complete step DAG. Planning failure SHALL create no
runtime side effect.

**Traceability:** OP-001, RT-004, SEC-001, P0-BASE-003

#### Scenario: Target capability is incompatible
- **GIVEN** an accepted intent references a Release unsupported by the target capability snapshot
- **WHEN** the server generates the ExecutionPlan
- **THEN** planning fails with a stable reason and no Operation is queued for target execution

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
kind, and client key. An exact semantic replay SHALL return the original
result; reuse with different semantics SHALL be rejected. Audit evidence SHALL
link subject, intent digest, Release, ExecutionPlan, policy, approval,
Operation, Provider steps, and terminal outcome.

**Traceability:** CONTRACT-003, OP-006, P0-BASE-003

#### Scenario: Idempotency key is reused with a different target
- **GIVEN** a tenant has committed an install intent under an idempotency key
- **WHEN** the same key is submitted with a different target reference
- **THEN** the platform rejects the semantic conflict and does not create a second Operation
