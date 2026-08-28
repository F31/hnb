# Tasks 8.1-8.5 Schema Engine Gap Closure

## 8.1 ui-kit primitives in PageRenderer

- `PageRenderer.vue` now renders page-level and block-level states with the
  tested ui-kit primitives `HNBPageState`, `HNBAlert`, and `HNBButton` instead
  of native HTML placeholders/buttons. Block error isolation via
  `RegionWrapper` is retained: an unknown component, a props-validation failure,
  or an unregistered action renders a safe block placeholder and never breaks
  the rest of the page.
- `builtins.ts` registers additional tested ui-kit components as trusted
  `componentType`s (PageState, Alert, Tabs, Pagination, OperationProgress,
  StatusGroup) with props schemas, extending the trusted-component allowlist.

## 8.2 endpoint/action/component/dictionary allowlist

- `DataSourceManager`:
  - `allowEndpoint(prefix)` declares the trusted endpoint path allowlist.
  - `isTrustedPath` rejects absolute URLs, protocol-relative `//`, `javascript:`
    /`data:`/`file:` schemes, and any query/fragment; only allowlisted relative
    paths register.
  - `clear()` does not clear the allowlist (configured once by the caller).
- `PageRenderer` resolves each region's `componentType` via `ComponentRegistry`
  and each referenced `actionId` against `spec.actions`; unknown component or
  action produces a block-level safe error (fail-closed).
- Shell `SchemaPage.vue` configures the cluster/operation endpoint allowlist
  (`/api/v1/resources/clusters`, `/api/v1/operations`, `/api/v1/runtime-intents`,
  `/api/v1/dictionaries/cluster.status`, `/api/v1/clusters`).

## 8.3 `resource.cluster.detail.tabs` declarative extension point

- New `ExtensionRegistry` validates, at registration time:
  - namespace against a controlled pattern (`[a-z0-9-](\.[a-z0-9-])*`),
  - `componentType` is registered,
  - `permission` is not a wildcard/empty,
  - `minShellVersion` does not exceed the current shell (version compatibility).
- `PageRegion` gains `extensionPoint`; `PageRenderer` resolves such regions via
  the extension registry and renders only extensions the caller has permission
  for, ordered by `order`.
- Shell `SchemaPage.vue` registers `resource.cluster.detail.tabs.overview` and
  `resource.cluster.detail.tabs.config` (builtin DetailPanel/DescriptionList).

## 8.4 DataSource tenant/context generation

- `DataSourceManager.invalidateContext()` bumps a generation counter; in-flight
  responses whose generation no longer matches are discarded (thrown as
  `discarded: stale context response`), so a tenant/space switch can never show
  stale data.
- `DataQuery.signal` is forwarded as an `AbortSignal`; aborted requests are not
  applied.
- `cacheKey(dataSourceId, query)` isolates by `dataSourceId + contextKey +
  stable-serialized params`, so the same params under different tenants/contexts
  do not share a cache entry.
- Shell `SchemaPage.vue` watches tenant/space and calls `invalidateContext()`.

## 8.5 minShellVersion / revision fail-closed + six block states

- `SchemaEngine.declareSupportedRevision(id, cap)`; a schema whose `revision`
  exceeds the declared cap is rejected as `INCOMPATIBLE` (fail-closed), in
  addition to the existing `minShellVersion` check.
- `PageRenderer` renders six states via `HNBPageState`
  (`loading` / `empty` / `error` / `no-permission` / `offline` / `incompatible`);
  incompatible schemas render the incompatible state and execute zero actions.

## Verification

- `pnpm typecheck` in `@hnb/schema-engine`, `@hnb/shell`, `@hnb/plugin-resource`:
  pass.
- Vitest from `web/` root:
  - `@hnb/schema-engine` 40 tests pass (incl. new `closure.test.ts`: revision
    fail-closed, allowlist rejection, context-generation discard, cache-key
    isolation, extension permission/version/namespace validation).
  - `@hnb/ui-kit` 32 tests pass; `@hnb/plugin-resource` polling tests pass.
- `pnpm build` in `@hnb/plugin-resource`: pass.

## Pre-existing environment failures (not caused by 8.x)

- `@hnb/api-client` and shell `NavigationManager`/`SchemaPage` suites fail in
  this workspace due to environment mock gaps (`Response.text is not a
  function`, plain-string token triggering `refresh token is missing`). These
  paths are untouched by 8.x.
