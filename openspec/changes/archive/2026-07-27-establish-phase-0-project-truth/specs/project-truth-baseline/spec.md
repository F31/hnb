## ADDED Requirements

### Requirement: [P0-BASE-001] Source-Anchored Project Truth
The Phase 0 baseline SHALL identify the inspected repository snapshot and SHALL
anchor material implementation claims to repository files, commands, or
validation output. Unverified claims SHALL be labelled as unverified rather
than inferred from names, routes, or task checkboxes.

**Traceability:** HNB-V3.8.6-PHASE-0, GOV-01

#### Scenario: Reviewer verifies a maturity claim
- **GIVEN** the baseline rates a capability above L0
- **WHEN** a reviewer follows the cited evidence
- **THEN** the reviewer can locate the handler, persistence path, security control, test, or validation output that supports the rating

### Requirement: [P0-BASE-002] L0-L5 Maturity Classification
The Phase 0 baseline SHALL assess relevant surfaces using L0 Route, L1 Handler,
L2 functional/persistence, L3 authentication/authorization/tenant isolation,
L4 tested, and L5 production readiness. A surface SHALL NOT receive a
consecutive maturity rating above the first unmet level, while evidence at a
higher non-consecutive level MAY be recorded separately.

**Traceability:** HNB-V3.8.6-METH-01, GOV-02

#### Scenario: Tests exist before security closure
- **GIVEN** a persisted handler has unit or integration tests but lacks an authenticated tenant boundary
- **WHEN** the surface is rated
- **THEN** the baseline records the test evidence but does not promote the consecutive rating above L2

### Requirement: [P0-BASE-003] Architecture-Invariant Gap Recording
The Phase 0 baseline SHALL verify the microkernel plus Provider/CapabilityPack
boundary, four-plane separation, and the
`Release/CompositionRelease -> ExecutionPlan -> Operation` runtime write path.
Any observed bypass, disconnected producer, or direct runtime mutation SHALL
be recorded as a blocker and SHALL NOT be normalized as an alternative
architecture.

**Traceability:** KERNEL-001, KERNEL-002, OP-001

#### Scenario: A component emits a non-canonical install command
- **GIVEN** an existing component emits an install or uninstall event
- **WHEN** no evidence connects that event to the canonical Operation engine
- **THEN** the baseline marks the seam as disconnected and blocks production-readiness claims for that flow

### Requirement: [P0-BASE-004] Security Boundary Evidence
The Phase 0 baseline SHALL inspect identity derivation, tenant derivation,
authorization, repository scoping, service-to-service trust, secret handling,
and runtime-target credentials for every in-scope externally reachable write
path. Client-supplied identity headers or body fields SHALL NOT count as L3
evidence unless a trusted authenticated boundary derives or overwrites them.

**Traceability:** TENANT-005, TENANT-006, TENANT-007

#### Scenario: Service trusts tenant headers directly
- **GIVEN** a handler reads a tenant or actor identity from an HTTP header
- **WHEN** no verified middleware derives and overwrites that header from authenticated claims
- **THEN** the baseline rates the security level as unmet and records the cross-tenant risk

### Requirement: [P0-BASE-005] Capability-Driven Console Evidence
The Phase 0 baseline SHALL distinguish a Web Console capability or permission
declaration from a working capability- and permission-driven integration.
L2 or higher evidence SHALL require a reachable backend contract, authenticated
permission hydration, functional route/component loading, and failure handling.

**Traceability:** UX-001, UX-003, TENANT-007

#### Scenario: Console manifest declares a required capability
- **GIVEN** a plugin manifest declares required permissions and capabilities
- **WHEN** the permission store is not hydrated or the capability endpoint is not implemented
- **THEN** the console integration remains below L2 even if the menu and route declarations exist

### Requirement: [P0-BASE-006] Marketplace Foundation Continuity
The Phase 0 baseline SHALL treat the existing `app-market` code, schema, and
OpenSpec capability as the Marketplace foundation. Phase 0 SHALL NOT introduce
an independent replacement Marketplace service, and any future closure work
SHALL extend or migrate the existing foundation through an approved change.

**Traceability:** MKT-001, MKT-004, HNB-V3.8.6-ARCH-FOUR-PLANES

#### Scenario: Marketplace integration is incomplete
- **GIVEN** the existing app-market install flow is not connected to Operation
- **WHEN** Phase 0 records the gap
- **THEN** it identifies an integration closure on the existing foundation instead of proposing a parallel Marketplace

