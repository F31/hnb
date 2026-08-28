## ADDED Requirements

### Requirement: [UX-008] Executable Schema-Driven Console Pages
The Web Console SHALL render authenticated schema pages from apiserver and SHALL support declared data source queries and actions through trusted Shell runtime services. Schema pages SHALL NOT execute arbitrary JavaScript, arbitrary URLs, unfiltered HTML, Secret values, or target credentials supplied by schema content.

**Traceability:** UX-001, UX-006, CONTRACT-001, CONTRACT-008

#### Scenario: Schema page loads and queries data
- **GIVEN** an authenticated user has access to a schema route returned by Console navigation
- **WHEN** the user opens the route
- **THEN** the Shell fetches the page schema from apiserver
- **AND** trusted components query data only through registered dataSource endpoint IDs and the shared authenticated API client

#### Scenario: Schema action invokes a backend operation
- **GIVEN** a schema page contains a declared action with a trusted endpoint ID and required permission
- **WHEN** the user triggers the action and the permission check passes
- **THEN** the Shell invokes the endpoint through the shared authenticated API client
- **AND** the result is reported as a safe text notification or operation tracking state

### Requirement: [UX-009] Controlled Schema Visibility and Action Gating
The Web Console SHALL evaluate schema region conditions and action enabled conditions using only controlled Shell context: scoped permissions, capabilities, feature/license state, tenant/space context, and safe record state. Hidden or disabled schema elements SHALL fail closed when required context is missing or stale.

**Traceability:** UX-001, P1-CONSOLE-002, P1-CONSOLE-003

#### Scenario: Region requires missing permission
- **GIVEN** a schema region declares a permission condition
- **WHEN** the authenticated subject lacks that permission in the selected tenant
- **THEN** the Shell does not render the protected region
- **AND** direct backend requests remain independently denied by server authorization

#### Scenario: Action condition becomes false
- **GIVEN** a rendered schema action depends on a capability and scoped permission
- **WHEN** either capability or permission state is unavailable or false
- **THEN** the action is disabled or hidden and cannot call the backend endpoint

### Requirement: [UX-010] Schema Incompatibility User Experience
When a schema page requires a Shell version newer than the running Shell, the Web Console SHALL show an explicit user-facing incompatibility message and SHALL NOT render partial or unsafe page content.

**Traceability:** UX-002, CONTRACT-002

#### Scenario: Schema requires newer Shell
- **GIVEN** apiserver returns a schema whose `minShellVersion` is higher than the running Shell version
- **WHEN** the Shell validates the schema
- **THEN** the user sees a clear upgrade-required message
- **AND** no schema regions or actions from that page are executed
