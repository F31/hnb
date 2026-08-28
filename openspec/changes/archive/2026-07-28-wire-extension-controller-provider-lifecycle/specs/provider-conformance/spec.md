## ADDED Requirements

### Requirement: [PROV-006] Extension controller owns Provider lifecycle
Provider installation, enablement, upgrade, rollback, health reconciliation and uninstall SHALL be reconciled by extension-controller or an equivalent dedicated lifecycle controller. platform-api SHALL expose Provider catalog and compatibility APIs but SHALL NOT directly deploy Provider Bundles on request paths.

**Traceability:** PROV-001, PROV-003, PROV-004, KERNEL-001

#### Scenario: User installs a Provider Bundle
- **GIVEN** a signed Provider Bundle with a verified manifest and digest
- **WHEN** an authorized user requests installation
- **THEN** the platform creates or correlates an Operation
- **AND** extension-controller reconciles the Bundle deployment and status

#### Scenario: platform-api receives a Provider catalog query
- **GIVEN** a user requests Provider metadata
- **WHEN** platform-api serves the query
- **THEN** it returns catalog and compatibility data
- **AND** it does not deploy or mutate Provider runtime workloads as part of that query

### Requirement: [PROV-007] Capability and navigation metadata registration
After a Provider lifecycle transition succeeds, the controller SHALL update capability registry and raw plugin/navigation metadata snapshots using versioned, tenant-safe records. apiserver SHALL consume these records to compute final navigation, and SHALL NOT infer installed capability solely from browser plugin manifests.

**Traceability:** UX-006, PROV-001, CONTRACT-001

#### Scenario: Provider exposes a new menu route
- **GIVEN** a Provider Bundle declares a menu route and required permission
- **WHEN** the Bundle is enabled successfully
- **THEN** the raw metadata snapshot includes the route and permission
- **AND** apiserver only returns it to users with matching capability and permission

### Requirement: [PROV-008] Safe upgrade and rollback
Provider upgrade SHALL install the target version as a candidate, verify digest/signature, run compatibility and health checks, and promote only after success. Rollback SHALL restore the previous active version and capability/navigation snapshot. Failed upgrades SHALL leave the previous active Provider serving existing Operations unless policy requires disablement.

**Traceability:** PROV-004, PROV-005, GOV-05

#### Scenario: Upgrade health check fails
- **GIVEN** Provider version `v1` is active and version `v2` is requested
- **WHEN** `v2` fails its health or conformance gate
- **THEN** `v1` remains active
- **AND** the Operation records failure evidence and rollback status

### Requirement: [PROV-009] Uninstall refusal and dependency checks
Provider uninstall SHALL be refused while active Operations, RuntimeTargets, capabilities, release plans, navigation routes or protected resources still depend on that Provider. The refusal response SHALL list dependency categories and safe remediation steps.

**Traceability:** KERNEL-002, PROV-003, CONTRACT-003

#### Scenario: Active Operation depends on Provider
- **GIVEN** an active Operation has a step assigned to Provider `p1`
- **WHEN** a user requests uninstall of `p1`
- **THEN** the controller refuses uninstall
- **AND** the response identifies active Operation dependency as a blocker

### Requirement: [PROV-010] Provider lifecycle events contain metadata only
Provider lifecycle commands and events SHALL contain Provider IDs, versions, artifact digests, operation IDs, capability IDs and SecretReferences only. They SHALL NOT contain inline Secret values, kubeconfigs, tokens or Provider artifact bytes.

**Traceability:** CONTRACT-005, PROV-001, SEC-001

#### Scenario: Lifecycle event includes a token
- **GIVEN** a lifecycle event payload contains an inline token field
- **WHEN** the event contract validator runs
- **THEN** the event is rejected before publish
- **AND** logs report only the field path and violation type
