## Why

The Web Console currently diverges from the V2.5/V3.6 UI architecture: navigation can still be hardcoded in apiserver/Web, Web stores call APIs directly, and reusable UI Kit components are incomplete. This prevents menu ordering, visibility, routes, and permissions from being extended through database-backed UI metadata and Kimi's bootstrap/navigation work.

## What Changes

- Replace production static Console navigation with a DB-backed navigation repository that reads plugin, route, menu, permission, capability, and ordering metadata.
- Add database-backed seed metadata for the default Console navigation tree; ordering comes from `sort_order`, not code or Web logic.
- Keep apiserver `GET /api/v1/navigation/menus` as the only browser-facing final navigation view and perform permission/capability pruning on the server.
- Align Console bootstrap/session data so Web can initialize permissions and capabilities from an apiserver-owned BFF endpoint while preserving the platform-api bootstrap implementation.
- Remove Web Shell menu/order hardcoding and direct business `fetch` calls where this change touches Auth, Context, Navigation, and Capability flows.
- Establish the first reusable `@hnb/ui-kit` component baseline for standard pages, toolbars, buttons, table actions, forms, select/date inputs, and detail panels using Naive UI underneath and HNB tokens above.
- **BREAKING** for temporary test data only: static navigation entries added during local testing are removed from production code paths.

## Capabilities

### New Capabilities

- `ui-registry-runtime`: Defines database-backed UI/navigation registry metadata, reusable UI component baseline, and Shell consumption rules for dynamic UI metadata.

### Modified Capabilities

- `portal-experience`: Tighten Console navigation and UI rendering requirements so Web consumes server-computed navigation and uses reusable UI Kit components.
- `provider-conformance`: Require Provider/UI metadata registration to include hierarchy/order fields needed by database-backed navigation.
- `contracts-events`: Align Console bootstrap/session contracts and navigation metadata response/version expectations.

## Impact

- Change ID: `align-ui-registry-navigation-and-uikit`.
- Capability tier: T1 Console/platform UX foundation.
- Planes affected: Web Console Shell, apiserver Console BFF, platform-api session/bootstrap contract, PostgreSQL UI metadata, provider lifecycle metadata.
- Affected code: `cmd/apiserver/internal/application/navigation`, `cmd/apiserver/internal/handler`, `cmd/platform-api/internal/api`, `database/postgresql/migrations`, `web/shell`, `web/packages/ui-kit`, `web/packages/schema-engine`.
- Dependencies: builds on archived Console navigation BFF, platform boundary hardening, provider lifecycle metadata, and schema runtime E2E changes.
- Database impact: adds UI registry/navigation metadata tables or extensions plus seed data; migration must be reversible and avoid introducing a new middleware dependency.
- Compatibility: Web continues calling `/api/v1/navigation/menus`; bootstrap alignment should preserve existing `/v1/console/bootstrap` while adding the contract-aligned session path/BFF.
- Security: final menu visibility remains a server decision based on signed `scopedPermissions`; Web rendering and route guards are defense-in-depth only.
- Resource budget: PostgreSQL remains the authority; no Redis or new external datastore. NATS/KV invalidation remains deferred unless already present.
- Observability: navigation generation metrics and filtered-item counters remain active; bootstrap/navigation failures must surface trace-aware errors.
- Rollback: revert migration/seed and switch apiserver handler back to the previous repository implementation for local fallback only; Web remains capable of rendering any valid `NavigationResponse`.
- Exit criteria: no production hardcoded menu/order path, navigation returned from DB metadata and filtered by permission, Web renders top/side navigation from API structure, focused Go/Web tests pass, and OpenSpec strict validation passes.
