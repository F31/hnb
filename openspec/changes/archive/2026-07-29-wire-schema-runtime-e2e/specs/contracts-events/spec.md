## ADDED Requirements

### Requirement: [CONTRACT-009] Apiserver Schema Page Contract
apiserver SHALL expose browser-facing Console schema pages through a versioned authenticated endpoint. The response SHALL be declarative schema metadata and SHALL contain only trusted component types, endpoint IDs, action IDs, text keys, layout metadata, and condition metadata needed by the Shell runtime.

**Traceability:** CONTRACT-001, CONTRACT-008, UX-008

#### Scenario: Browser requests a schema page
- **GIVEN** an authenticated browser route references schema page `cluster-list`
- **WHEN** the Shell requests `GET /api/v1/schema/page/cluster-list` through apiserver
- **THEN** apiserver returns a versioned schema envelope or a bounded error with trace identifier
- **AND** the response does not include executable code, Secret values, target credentials, or arbitrary external URLs

#### Scenario: User lacks schema page permission
- **GIVEN** a schema page requires a permission the subject lacks
- **WHEN** the subject requests the schema page directly
- **THEN** apiserver denies the request or omits protected declarations according to the page contract
- **AND** no protected backend endpoint becomes reachable through schema metadata

### Requirement: [CONTRACT-010] Trusted Schema Endpoint Resolution
Schema-declared data sources and actions SHALL resolve backend calls by trusted endpoint ID, not arbitrary URL. The Shell SHALL use its shared authenticated API client for those calls, and apiserver/platform-api SHALL remain authoritative for authorization and validation.

**Traceability:** CONTRACT-001, CONTRACT-003, KERNEL-016, UX-008

#### Scenario: Schema declares an unknown endpoint ID
- **GIVEN** a schema action or dataSource references an endpoint ID not registered in the trusted runtime registry
- **WHEN** the Shell attempts to execute the query or action
- **THEN** execution is rejected before any HTTP request is sent

#### Scenario: Authorized-looking action reaches backend
- **GIVEN** the Shell executes a schema action using a trusted endpoint ID
- **WHEN** the backend receives the request
- **THEN** the backend performs normal authentication, tenant scoping, authorization, validation, and trace/error handling
