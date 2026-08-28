## Why

The V2.5 schema engine currently validates and renders static schema pages, but the end-to-end runtime loop is incomplete: Web Shell assumes `/api/v1/schema/page/{id}`, DataSource and Action execution are not wired into real Shell pages, and E2E coverage does not prove interactive schema pages work. This change turns schema-driven UI from a static rendering demo into a verifiable Console runtime path through apiserver.

Change ID: `wire-schema-runtime-e2e`
Tier: T1 Console/runtime experience
Planes: Web Console Shell and apiserver northbound API/BFF
Dependencies: archived `complete-console-bff-navigation`, archived `harden-apiserver-platform-boundary`

## What Changes

- Add an authenticated apiserver schema page endpoint for Web Console schema pages, initially backed by trusted built-in/static schema repository data.
- Wire Shell `SchemaPage` through the shared authenticated API client rather than direct ad hoc `fetch`.
- Connect `DataSourceManager` and `ActionEngine` to `PageRenderer`/trusted components so schema pages can query data and invoke declared actions.
- Add controlled condition evaluation for schema regions/actions using permission, capability, feature/license, and context inputs available to the Shell.
- Add focused unit and E2E coverage for schema load, data source query, action invocation, permission-denied behavior, and incompatible Shell UX.
- No breaking API removal. The endpoint is additive and browser-facing through apiserver only.

Non-goals:
- Remote Bundle cosign/certificate-chain verification.
- SSE/WebSocket runtime event delivery.
- Full tabs/split/drawer advanced layout implementation.
- Production dynamic schema authoring, database-backed schema registry, or arbitrary plugin-provided JavaScript.

Compatibility:
- Existing grid-only schema pages remain valid.
- Existing static plugin/component registration remains valid.
- Existing ETag navigation and polling remain unchanged.

Security:
- Schema remains declarative metadata only; no arbitrary script, URL, HTML, Secret, or target credential execution.
- DataSource and Action API calls resolve only registered endpoint IDs and use the shared authenticated client.
- apiserver remains the browser-facing boundary; platform-api is not exposed directly to Web pages.

Resource and observability expectations:
- Schema endpoint responses should be small Console DTOs and cacheable with ETag where practical.
- Runtime page failures should expose safe user messages and include trace IDs in API errors.
- Tests should include both Go handler coverage and Web unit/E2E coverage.

Rollback strategy:
- Remove the additive apiserver schema route and Shell route usage; existing plugin static pages and navigation continue to operate.
- Revert schema runtime injection without affecting navigation BFF or plugin activation.

Exit criteria:
- Go apiserver tests/build/vet pass.
- Web schema-engine/Shell tests and a schema-page E2E interaction smoke pass.
- `openspec validate --all --strict --no-interactive` passes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `portal-experience`: Adds requirements for executable schema pages, controlled visibility/action gating, and Shell UX for incompatible schema pages.
- `contracts-events`: Adds the apiserver schema page and trusted endpoint resolution contract used by Web Console schema runtime.

## Impact

- Web: `web/shell/src/pages/SchemaPage.vue`, schema-engine runtime components, trusted built-in components, API client usage, Playwright/Vitest coverage.
- apiserver: route and handler for `GET /api/v1/schema/page/{id}`, DTO/repository/service tests, OpenAPI route documentation.
- Contracts: browser-facing schema page DTO and trusted endpoint ID semantics; no new middleware or database is required for this T1 slice.
