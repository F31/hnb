## MODIFIED Requirements

### Requirement: [UX-006] Apiserver-owned final navigation view
The platform SHALL expose the final browser-facing Console navigation view only through apiserver `GET /api/v1/navigation/menus`. The response SHALL be computed from authenticated subject, tenant/space context, scoped permissions, database-backed plugin/menu/route metadata, capability availability, feature/license state, locale, and stored ordering metadata. The Web Console SHALL NOT hardcode final menu order, generate menus from plugin manifests, or call platform-api for final user menus.

#### Scenario: Navigation order comes from metadata
- **GIVEN** database-backed navigation metadata stores first-level menu items with `sort_order`
- **WHEN** an authenticated subject requests `/api/v1/navigation/menus`
- **THEN** apiserver returns menus ordered by `sort_order`
- **AND** Web renders the returned order without applying a hardcoded order list

#### Scenario: Home has no sidebar from data shape
- **GIVEN** the returned first-level home navigation item is a leaf route or has no visible children
- **WHEN** Web renders the Console layout on that route
- **THEN** the top navigation shows the home item
- **AND** the side navigation is not shown because the returned navigation item has no children

### Requirement: [UX-011] Shared UI component baseline
The Web Console SHALL provide reusable UI Kit components for standard pages, toolbars, buttons, tables, forms, select/date inputs, status, detail panels, empty/error/loading states, and action bars. Shell, Schema Renderer, and plugins SHALL prefer these components over local duplicate styling for standard CRUD, list, detail, and dashboard pages.

#### Scenario: Schema page uses registered UI Kit components
- **GIVEN** a PageSchema references a standard registered component type such as `DataTable`, `MetricCard`, or `DescriptionList`
- **WHEN** the Schema Renderer resolves the component
- **THEN** it uses the `@hnb/ui-kit` registered component
- **AND** unknown component types are isolated to a component-level error placeholder

#### Scenario: Plugin page reuses UI Kit primitives
- **GIVEN** a plugin implements a standard list page
- **WHEN** it renders table, toolbar, button, empty, loading, and error states
- **THEN** it uses `@hnb/ui-kit` primitives and HNB design tokens instead of local hardcoded colors and duplicate table wrappers
