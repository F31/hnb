## Context

UI V2.5 and Web Console V3.6 define the Console as a microkernel: Shell owns auth, context, navigation, router, permissions, plugin runtime, API client, schema engine, and reusable components; backend UI Registry/Navigation Service owns menu, route, plugin, permission, capability, and ordering metadata. Current implementation has several gaps: apiserver navigation still has a production `StaticRepository`, Web has direct `fetch` calls, plugin manifests still carry local `menuItems` and `routes`, and `@hnb/ui-kit` only contains a small component subset.

## Goals

- Make `GET /api/v1/navigation/menus` database-backed in production.
- Store menu hierarchy and order in PostgreSQL, including first/second/third-level navigation.
- Preserve server-side pruning by scoped permission and capability.
- Align bootstrap/session data with Kimi's existing platform-api bootstrap implementation and expose it safely through apiserver.
- Keep Web Shell as a renderer of server-computed navigation, not a generator or sorter.
- Establish a reusable UI Kit baseline that Schema pages and plugin pages can share.

## Non-Goals

- Full remote bundle signature/certificate-chain verification.
- Full NATS JetStream KV L2 cache implementation for navigation snapshots.
- Complete migration of every plugin page from placeholder Vue pages to Schema pages.
- Full FormSchema/TableSchema authoring UI.
- Replacing Naive UI; HNB UI Kit wraps and standardizes it.

## Architecture

### Navigation Data Model

Add UI/navigation registry tables that can be seeded locally and later populated by Provider lifecycle registration:

- `console_plugins`: plugin identity, version, display name, tier, mode, enabled state, capabilities, permissions, and lifecycle metadata.
- `console_routes`: route name/path, plugin/component or schema page target, permission, capability, feature/license fields, `sort_order`, enabled state.
- `console_navigation_items`: tree nodes with `parent_id`, `level`, title/default title, icon, route reference, permission/capability conditions, `sort_order`, enabled state, locale.
- `console_navigation_versions`: version vector/revision source for navigation and plugin catalog.

The repository builds a `Snapshot` sorted by `sort_order`, then the existing service filters routes and recursively prunes menus.

### Permission Model

The authorization source of truth remains IAM. Login tokens carry signed canonical `scopedPermissions`; apiserver middleware verifies the token and injects `TrustedContext`. Navigation filtering maps route/menu permission strings such as `cluster:read` to `ScopedPermission{ResourceKind: cluster, Action: read}` and rejects unmatched items. Wildcard resource kind may be supported only if it was signed into the token by IAM.

### Bootstrap Alignment

platform-api already implements `GET /v1/console/bootstrap`; generated contracts refer to `GET /v1/session/bootstrap`. This change should add the contract-aligned path and an apiserver BFF endpoint, while preserving existing behavior. Web should initialize permissions/capabilities from bootstrap when available and fall back to JWT-derived permissions only as a compatibility measure.

### Web Shell

`LayoutShell` renders first-level navigation from the response order. Side navigation uses the active item children. Whether a route shows no sidebar must be data-driven, e.g. a leaf first-level item or metadata, not a hardcoded group name. Navigation, context, and capability calls should use `@hnb/api-client`.

### UI Kit Baseline

Use Naive UI as the lower-level component library and expose HNB semantic wrappers with tokens and consistent states:

- `HNBPage`
- `HNBToolbar`
- `HNBButton`
- enhanced `HNBTable`
- `HNBSelect`
- `HNBDatePicker` / `HNBDateRangePicker`
- `HNBForm` / `HNBFormField`
- `HNBDetailPanel`
- `HNBActionBar`

The Schema Engine `ComponentRegistry` should register available stable wrappers and reject unknown types.

## Migration Strategy

1. Add new tables/columns and seed default navigation metadata.
2. Implement DB repository and handler wiring.
3. Keep test-only repositories for unit tests.
4. Expose bootstrap BFF and update Web initialization.
5. Remove hardcoded menu/order behavior from Web and apiserver production path.
6. Add UI Kit components and register them incrementally.

## Risks

- Incomplete seed data can hide all menus because default behavior is fail closed.
- Bootstrap path inconsistency can leave Web with stale permission/capability stores.
- Route/plugin ID mismatches can load menus but fail route registration.
- UI Kit expansion can grow scope; this change should prioritize reusable primitives over migrating all pages.

## Verification

- Unit tests for DB navigation ordering, filtering, and parent pruning.
- Handler tests for navigation response and bootstrap BFF.
- Web tests for top navigation, sidebar behavior, and API client usage in touched stores/managers.
- UI Kit tests for newly introduced reusable components.
- `openspec validate --all --strict --no-interactive`.
