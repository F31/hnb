## ADDED Requirements

### Requirement: [CONTRACT-007] Common HTTP error and trace contract
apiserver and platform-api SHALL expose a compatible HTTP error contract for public and internal HTTP APIs. Error responses SHALL include a stable machine-readable code, human-readable message, and request trace identifier. Services SHALL accept `X-Trace-Id`, mirror it to responses, and MAY mirror `X-Correlation-ID` during transition.

**Traceability:** CONTRACT-001, CONTRACT-002, CONTRACT-003

#### Scenario: platform-api returns a validation error through apiserver
- **GIVEN** a browser submits an invalid request through apiserver
- **WHEN** platform-api rejects the domain request
- **THEN** the browser receives a response containing `code`, `message`, and trace identifier
- **AND** the same trace identifier appears in apiserver and platform-api logs

### Requirement: [CONTRACT-008] Public DTO boundary
apiserver SHALL expose Console-oriented DTOs and platform-api SHALL expose domain DTOs through versioned contracts. Implementations SHALL NOT share internal database rows, unversioned Go structs, or private package types as cross-service public contracts.

**Traceability:** CONTRACT-001, KERNEL-016

#### Scenario: Cluster domain gains an internal field
- **GIVEN** platform-api adds an internal cluster scheduling field
- **WHEN** apiserver returns a Console cluster list
- **THEN** the field is exposed only if it is added to the versioned contract and mapped intentionally
- **AND** browser clients do not depend on platform-api database columns
