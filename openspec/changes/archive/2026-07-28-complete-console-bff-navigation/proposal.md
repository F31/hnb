## Why

The Web Console already calls `/api/v1/navigation/menus`, but the final navigation view is still an MVP server-side hardcoded response. HNB needs a real Console BFF navigation contract so browser clients do not bind directly to domain services or infer authority from plugin manifests.

## What Changes

- Change ID: `complete-console-bff-navigation`
- Tier: T0/T1 boundary; the apiserver BFF endpoint is T0 and installed Console/plugin metadata inputs are T1.
- Impacted planes: Web Console Shell, apiserver, extension metadata, IAM, capability registry, optional license/feature flag sources.
- Add a production Navigation Service behind `GET /api/v1/navigation/menus` in apiserver.
- Generate the final `NavigationResponse` from identity, tenant/space context, scoped permissions, installed plugin/menu/route metadata, capability availability, feature/license state and locale.
- Keep Web Console as the only executor of `NavigationManager`, `PluginLoader`, `PluginManager`, `RouterManager`, Pinia and browser LKG behavior.
- Ensure platform-api does not expose final user navigation to browsers.
- Dependencies: none; builds on current apiserver navigation endpoint and existing portal-experience requirements.
- Migration impact: existing Web Console path remains stable; MVP hardcoded menus are replaced by data-backed aggregation.
- Rollback strategy: retain the current static menu provider behind a feature flag for one release while preserving endpoint shape.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `portal-experience`: final console navigation must be served by apiserver BFF from server-side authority sources and fail closed.

## Impact

- Affected code: `cmd/apiserver/internal/handler/navigation.go`, new navigation application service/repositories/clients, Web Shell navigation tests, extension metadata sources, deployment config.
- APIs: `GET /api/v1/navigation/menus` becomes the authoritative browser-facing navigation API; platform-api `/api/v1/menus` must remain removed.
- Dependencies: IAM permission snapshot, tenant context, plugin/menu registry, capability registry, optional license/feature flag source, NATS invalidation events or polling.
- Security risks: stale permission cache or plugin metadata could expose unauthorized routes; mitigated by tenant-scoped cache keys, permission version checks, ETag invalidation and server-side filtering.
- Resource budget: L1 in-process cache and optional NATS KV/read model only; no new database or middleware.
- Observability: cache hit/miss, filtered route counts, generation latency, invalidation events and denied navigation requests.
- Exit criteria: authenticated Web Console can fetch tenant-scoped navigation, unauthorized routes are filtered, ETag/304 works, tenant switch clears stale state, and tests prove browser never calls platform-api for final menus.
