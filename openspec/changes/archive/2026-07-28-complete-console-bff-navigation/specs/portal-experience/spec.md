## ADDED Requirements

### Requirement: [UX-006] Apiserver-owned final navigation view
The platform SHALL expose the final browser-facing Console navigation view only through apiserver `GET /api/v1/navigation/menus`. The response SHALL be computed from authenticated subject, tenant/space context, scoped permissions, installed plugin/menu/route metadata, capability availability, feature/license state and locale. The Web Console SHALL NOT call platform-api for final user menus.

**Traceability:** UX-001, P1-CONSOLE-001, P1-CONSOLE-002, CONTRACT-001

#### Scenario: User lacks a route permission
- **GIVEN** an installed plugin registers a route requiring `cluster:update`
- **WHEN** a user without that scoped permission requests `/api/v1/navigation/menus`
- **THEN** apiserver omits that route and menu item from the returned `NavigationResponse`
- **AND** a direct request to the protected backend route remains independently denied

#### Scenario: Browser attempts to call platform-api menu endpoint
- **GIVEN** the Web Console is configured with the apiserver base URL
- **WHEN** navigation is loaded
- **THEN** the browser requests `/api/v1/navigation/menus` from apiserver
- **AND** platform-api does not expose a public final menu endpoint for browser use

### Requirement: [UX-007] Navigation cache invalidation and tenant safety
The navigation service SHALL scope ETag, L1 cache and optional distributed cache entries to subject, tenant, space, locale and version vector. A change to permission version, plugin catalog version, navigation version, license/feature version or tenant SHALL invalidate stale navigation and fail closed for protected routes.

**Traceability:** UX-001, P1-CONSOLE-003, TENANT-005, CONTRACT-003

#### Scenario: Permission version changes
- **GIVEN** a user has a cached navigation response with permission version `p1`
- **WHEN** their role binding changes to permission version `p2`
- **THEN** the next navigation request is recomputed or returns a fresh ETag for `p2`
- **AND** routes no longer granted by `p2` are absent

#### Scenario: Tenant switch with persisted LKG
- **GIVEN** the browser has a last-known-good navigation snapshot for tenant A
- **WHEN** the user switches to tenant B
- **THEN** apiserver returns tenant B navigation only
- **AND** the tenant A snapshot cannot authorize or display tenant B protected routes
