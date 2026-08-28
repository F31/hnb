## ADDED Requirements

### Requirement: [P1-CONSOLE-001] Authenticated Console Bootstrap
The Web Console SHALL obtain the verified subject, selected tenant,
memberships, deployment capabilities, scoped permissions, and version metadata
from an authenticated server bootstrap contract. It SHALL NOT infer authority
from plugin manifests, locally stored roles, identity headers, or route names.

**Traceability:** UX-001, TENANT-005, TENANT-007, P0-BASE-005

#### Scenario: Plugin declares a permission the user lacks
- **GIVEN** a plugin manifest declares a required runtime-write permission
- **WHEN** bootstrap does not grant that permission to the selected tenant
- **THEN** the Console does not expose the route or action and the server would independently deny a direct request

### Requirement: [P1-CONSOLE-002] Capability and Permission Intersection
Console plugin activation, navigation, routes, and actions SHALL be enabled
only by the intersection of installed capability availability and the
subject's scoped permission. Missing, stale, or failed bootstrap data SHALL
fail closed for protected features.

**Traceability:** UX-001, UX-002, TENANT-007, P0-BASE-005

#### Scenario: Backend capability becomes unavailable
- **GIVEN** a subject retains permission for a plugin action but the deployment capability is disabled
- **WHEN** the Console refreshes bootstrap
- **THEN** the plugin action is removed or disabled and cannot be invoked through the shared API client

### Requirement: [P1-CONSOLE-003] Session and Tenant Transition Safety
The Web Console SHALL clear tenant-scoped caches, pending privileged state, and
plugin-derived data on logout, token expiry, subject disable, tenant switch, or
permission version change. Every API call SHALL use the shared authenticated
client and correlation contract.

**Traceability:** TENANT-005, UX-005, P0-BASE-005

#### Scenario: User switches tenants
- **GIVEN** the Console has cached tenant-A operations and permissions
- **WHEN** the user switches to tenant B
- **THEN** tenant-A data and authority are cleared before tenant-B routes or requests are enabled
