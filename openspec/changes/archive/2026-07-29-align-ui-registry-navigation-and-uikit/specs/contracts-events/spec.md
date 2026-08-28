## MODIFIED Requirements

### Requirement: [CONTRACT-011] Console bootstrap and navigation metadata contracts
The platform SHALL expose a Console bootstrap contract containing verified subject, selected tenant, memberships, capabilities, scoped permissions, policy version, and permission version. The contract-aligned platform-api path SHALL be available as `/v1/session/bootstrap`, and apiserver SHALL provide the browser-facing BFF endpoint under `/api/v1/session/bootstrap`. Existing `/v1/console/bootstrap` behavior MAY remain as a compatibility alias.

#### Scenario: Web initializes permissions and capabilities from bootstrap
- **GIVEN** an authenticated Console session
- **WHEN** Web calls apiserver `/api/v1/session/bootstrap`
- **THEN** it receives subject, memberships, capabilities, permissions, `policyVersion`, and `permissionVersion`
- **AND** Web updates permission and capability stores from that response before rendering protected dynamic UI

#### Scenario: Navigation response carries version metadata
- **GIVEN** navigation metadata, plugin catalog, or permissions change
- **WHEN** apiserver returns `/api/v1/navigation/menus`
- **THEN** the response includes version metadata sufficient for cache invalidation
- **AND** stale cached navigation cannot authorize or display newly unauthorized routes
