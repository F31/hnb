## MODIFIED Requirements

### Requirement: [PROV-007] Capability and navigation metadata registration
After a Provider lifecycle transition succeeds, the controller SHALL update capability registry and raw plugin/navigation metadata snapshots using versioned, tenant-safe records. The metadata SHALL include route identity, plugin/component or schema target, required permission, capability conditions, parent/child navigation hierarchy, icon, enabled state, locale, and `sort_order`. apiserver SHALL consume these records or their promoted Console registry projection to compute final navigation, and SHALL NOT infer installed capability solely from browser plugin manifests.

#### Scenario: Provider registers ordered menu metadata
- **GIVEN** a Provider Bundle declares a menu route and required permission
- **WHEN** the Provider is enabled
- **THEN** the raw metadata snapshot includes route, permission, parent relationship, locale, and `sort_order`
- **AND** apiserver only returns it to users with matching capability and permission in the stored order

#### Scenario: Provider unregister hides navigation
- **GIVEN** a Provider is disabled or uninstalled
- **WHEN** its navigation metadata is no longer active
- **THEN** apiserver omits its routes and menu items from `/api/v1/navigation/menus`
- **AND** Web unloads or prevents access to those plugin routes after navigation refresh
