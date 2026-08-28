## Overview

Move final navigation calculation into apiserver as a Console BFF application service. Web Shell continues to execute runtime navigation/plugin/router logic, while apiserver returns the final visible `NavigationResponse` for the authenticated subject and selected tenant/space.

## Architecture

- `NavigationHandler` remains the HTTP boundary for `/api/v1/navigation/menus`.
- `application/navigation.Service` builds the response from interfaces, not direct database access in the handler.
- Repository/client interfaces provide plugin/menu/route metadata, capability state, license/feature flags and IAM permission versions.
- Cache keys include subject, tenant, space, locale, permission version, plugin catalog version, navigation version and license/feature version.
- Platform-api does not serve browser navigation. It may provide internal domain capability metadata through versioned internal APIs if needed.

## Data Sources

- Trusted identity and scoped permissions from request context.
- Tenant/space context from apiserver middleware and request query.
- Plugin/menu/route metadata from extension registry tables or internal extension service.
- Capability availability from provider/capability registry.
- Locale and mode from request headers/query.
- Optional license/feature source; absence defaults to fail-closed for gated features.

## Security

- The service filters routes server-side before returning them.
- Browser LKG is only a degraded UX cache and cannot grant server authorization.
- Tenant and permission version mismatches invalidate cached navigation.
- Response MUST omit routes for unavailable capabilities or missing permissions.

## Compatibility

- Endpoint path and TypeScript `NavigationResponse` shape remain stable.
- Static MVP menu provider remains as a temporary fallback behind config for rollback.
- ETag and 304 semantics remain supported.

## Observability

- Metrics: generation latency, cache hit/miss, filtered menu count, denied requests, invalidation events.
- Logs include tenant, subject hash, version vector and result counts; no permission list or token values.

## Non-Goals

- RuntimeIntent does not generate menus.
- Web Shell runtime classes are not moved into apiserver.
- No new middleware or database is introduced.
