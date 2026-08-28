# Tasks: phase-1-trusted-entry-and-write-path

Tasks are checked only after their implementation and verification complete.

## Contracts and Schema

- [x] **P1-ING-001, P1-ING-002** Publish the versioned token/claim and trusted
  request-context schemas; generate Go and TypeScript bindings.
- [x] **P1-ING-003, P1-ING-004** Publish authorization decision and
  service-identity contracts with explicit scope/action enums.
- [x] **P1-WRITE-001, P1-WRITE-002** Publish RuntimeIntent and immutable
  planning schemas; reject executable steps and credentials at schema level.
- [x] **P1-CONSOLE-001, P1-CONSOLE-002** Publish Console bootstrap,
  capability, permission, tenant-switch, and error contracts.
- [x] **P1-WRITE-004** Add compatibility checks for public API/event schemas
  and generated artifacts.

## Database and Migration

- [x] **P1-ING-003, P1-WRITE-003** Resolve the `005`/`021` and `010`/`022`
  migration collisions before selecting new migration numbers.
- [x] **P1-ING-003** Add/reconcile scoped subject, membership, role, policy
  version, and binding persistence with tenant-safe constraints.
- [x] **P1-WRITE-003, P1-WRITE-005** Add immutable intent and audit linkage
  persistence without recreating existing Operation tables.
- [x] **P1-ING-005** Add key metadata lifecycle storage if the selected key
  provider does not already supply it; never store private material in ordinary
  application tables.
- [ ] **P1-ING-003, P1-WRITE-003** Test forward, rollback, mixed-version, and
  point-in-time recovery paths on a production-shaped database.

## Implementation

- [x] **P1-ING-001, P1-ING-002** Unify token issuance and verification;
  sanitize inbound identity headers and inject typed context.
- [x] **P1-ING-003** Enforce tenant/project/environment/namespace/resource/action
  authorization at ingress, handlers, and repositories.
- [x] **P1-ING-004** Implement audience-restricted workload/service identity.
- [x] **P1-ING-005** Implement signing-key rotation and emergency revocation.
- [x] **P1-WRITE-001, P1-WRITE-002** Replace public arbitrary-step submission
  with typed intent validation and server-side planning.
- [x] **P1-WRITE-003** Atomically commit intent, ExecutionPlan, Operation,
  initial steps, audit, read model, and outbox.
- [x] **P1-WRITE-004** Route Release/install/uninstall/upgrade/rollback/config
  flows through canonical intent and Operation controls.
- [x] **P1-WRITE-005, P1-ING-006** Propagate correlation and append security
  audit evidence across domain boundaries.
- [x] **P1-CONSOLE-001, P1-CONSOLE-002, P1-CONSOLE-003** Hydrate Console
  session/capabilities/permissions and enforce plugin, route, and action gates.

## Verification

- [x] **P1-ING-001 through P1-ING-006** Add unit tests for malformed tokens,
  issuer/audience/algorithm confusion, expiry, header spoofing, scope mismatch,
  cache invalidation, and redaction.
- [x] **P1-WRITE-001 through P1-WRITE-005** Add integration tests for
  idempotency, semantic conflicts, policy denial, atomic failure, outbox outage,
  and absence of direct target writes.
- [x] **P1-CONSOLE-001 through P1-CONSOLE-003** Add Console component and E2E
  tests for capability absence, permission denial, tenant switch, expiry, and
  server-side rejection.
- [x] **P1-ING-003, P1-WRITE-004** Add cross-tenant and cross-scope negative
  security tests for every public route.
- [x] **P1-WRITE-004** Run contract tests proving all runtime lifecycle entry
  points produce/control a canonical Operation.
- [x] **P1-ING-005** Drill routine key rotation and emergency key revocation.
- [x] **P1-ING-006, P1-WRITE-005** Verify logs, traces, and audit reconstruct
  subject-to-Operation evidence without secrets.

## Delivery and Operations

- [x] **P1-ING-001 through P1-CONSOLE-003** Document deployment ordering,
  compatibility window, enforcement cutover, and operator runbook.
- [x] **P1-ING-006** Define SLOs and alerts for auth failures, policy latency,
  intent failures, Operation commit, and outbox lag.
- [ ] **P1-WRITE-003** Execute backup/restore and disaster-recovery rehearsal.
- [x] **P1-ING-002, P1-WRITE-004** Prove rollback does not re-expose
  header-trusting public routes or create a runtime write bypass.
- [x] **P1-ING-001 through P1-CONSOLE-003** Run strict OpenSpec, contract,
  unit, integration, security, E2E, and conformance gates and attach evidence
  before archive.

## Not Applicable

- New middleware product: N/A; existing ingress and identity foundations are
  reconciled.
- New runtime Provider/driver: N/A; Phase 1 constrains their call path but does
  not add one.
- Artifact storage migration: N/A; only immutable digest references cross this
  boundary.
