## ADDED Requirements

### Requirement: [KERNEL-016] Northbound and domain API separation
The platform SHALL route Web Console and ordinary user CLI traffic through apiserver as the northbound API/BFF. platform-api SHALL own platform resource, RuntimeTarget, RuntimeIntent, ExecutionPlan, Provider catalog and Operation domain records. apiserver SHALL NOT directly mutate platform-api-owned domain tables on request paths after the boundary migration is complete.

**Traceability:** KERNEL-001, KERNEL-002, CONTRACT-001, TENANT-005

#### Scenario: Browser lists clusters
- **GIVEN** a browser requests the cluster list
- **WHEN** the request enters HNB
- **THEN** it is authenticated and context-enriched by apiserver
- **AND** platform-api performs resource-level authorization before returning cluster domain data

#### Scenario: apiserver lacks platform-api connectivity
- **GIVEN** apiserver cannot reach platform-api
- **WHEN** a request requires platform resource domain state
- **THEN** apiserver returns a bounded upstream error with trace ID
- **AND** it does not read or write platform-api-owned tables as an untracked fallback in production mode

### Requirement: [KERNEL-017] One-way synchronous dependency
platform-api SHALL NOT synchronously call apiserver to complete domain logic. Domain state changes SHALL be exposed through persistent state, Outbox and NATS events, and apiserver MAY consume those events to invalidate BFF caches or notify clients.

**Traceability:** KERNEL-003, CONTRACT-004, CONTRACT-005

#### Scenario: Cluster status changes
- **GIVEN** platform-api updates a cluster status
- **WHEN** the update commits
- **THEN** a domain event or read-model update is emitted through the approved asynchronous path
- **AND** platform-api does not make an HTTP callback to apiserver

### Requirement: [KERNEL-018] Dual-layer authorization
apiserver SHALL perform identity, tenant context and northbound route authorization. platform-api SHALL independently authorize resource instance access and validate state transitions for domain resources, even when requests arrive through apiserver.

**Traceability:** TENANT-005, TENANT-006, KERNEL-002

#### Scenario: User has route permission but not resource ownership
- **GIVEN** a user has generic `cluster:read` route permission in tenant A
- **WHEN** they request a cluster outside tenant A through apiserver
- **THEN** platform-api denies the resource-level request
- **AND** no domain data for the foreign tenant is returned
