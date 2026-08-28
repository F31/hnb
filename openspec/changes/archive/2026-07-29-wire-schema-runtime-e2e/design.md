## Context

The Console already has the main pieces of the V2.5 schema runtime, but they are not connected end-to-end. `DataSourceManager` can call registered endpoints, `ActionEngine` can execute declared actions, and `PageRenderer` can validate/render grid regions. `SchemaPage` currently fetches `/api/v1/schema/page/{id}` directly, while apiserver does not expose that route yet. Region conditions are defined in the schema types but are not evaluated.

This change is a T1 Console/apiserver integration slice. It does not introduce a new database, message broker, remote bundle trust model, or runtime mutation path. Mutating actions still go through existing authenticated HTTP APIs and backend authorization; this change only wires the declarative UI runtime.

```
Browser Schema Route
        |
        v
  SchemaPage.vue
        |
        v
 shared API client  --->  apiserver GET /api/v1/schema/page/{id}
        |
        v
 SchemaEngine validate
        |
        v
 PageRenderer
   |        |        |
   v        v        v
Condition  DataSourceManager  ActionEngine
 checks    endpointId->HTTP   endpointId->HTTP/router/overlay
```

## Goals / Non-Goals

**Goals:**
- Provide an authenticated apiserver schema page endpoint for trusted built-in schema pages.
- Make Shell schema pages use the shared authenticated API client and trace/error contract.
- Wire `DataSourceManager` and `ActionEngine` into schema page rendering.
- Evaluate controlled visibility/enabled conditions and fail closed when context is missing.
- Add E2E coverage for schema load, data query, and action execution.

**Non-Goals:**
- Dynamic user-authored schema registry or schema persistence.
- Remote plugin cosign/certificate verification.
- SSE/WebSocket eventing.
- Advanced layouts beyond the minimum needed for this end-to-end slice.
- New runtime write APIs or Operation semantics.

## Decisions

### Decision 1: apiserver owns browser-facing schema pages

`GET /api/v1/schema/page/{id}` will be served by apiserver, not platform-api. This follows the established northbound boundary: Web Console receives Console DTOs from apiserver, while platform-api remains domain-oriented.

Alternative considered: serve schema pages from platform-api. Rejected because browser direct platform-api coupling would violate the boundary and complicate tenant/context aggregation.

### Decision 2: start with trusted static schema repository

The first implementation will use an in-process trusted repository for a small set of schema pages used by tests and built-in Console routes. This avoids prematurely designing a database-backed schema authoring model.

Alternative considered: add schema tables immediately. Rejected because persistence and authoring workflows are not required for this P0 closure and would expand migration/rollback risk.

### Decision 3: schema runtime resolves endpoint IDs, never URLs

Schema `endpoints`, `dataSources`, and `actions` reference trusted endpoint IDs. The Shell registers or receives allowed endpoint definitions and uses the shared authenticated client to call relative apiserver paths. Unknown endpoint IDs fail before any HTTP request.

Alternative considered: allow schema to include full URLs. Rejected for SSRF, cross-plane boundary, and arbitrary backend invocation risk.

### Decision 4: condition evaluation is defensive UI gating, not authority

The Shell evaluates permission/capability/feature/license/context conditions to hide or disable UI elements. Backend authorization remains authoritative. Missing condition inputs fail closed.

Alternative considered: render all regions and rely only on backend denial. Rejected because V2.5/UX-001 require unavailable capabilities and unauthorized actions not to appear as executable UI.

### Decision 5: user-facing incompatibility error

`minShellVersion` validation errors should be converted to a clear upgrade-required UI rather than a generic schema failure. The page must not partially render incompatible content.

## Data Model

No database migration is required.

Schema envelope remains declarative:
- `apiVersion`, `kind`, `metadata`, `spec`
- `metadata.minShellVersion`, `metadata.texts`
- `spec.layout`, `spec.regions`
- optional trusted endpoint/dataSource/action declarations as needed by runtime components

No Secret, token, kubeconfig, arbitrary code, raw HTML, or external URL is allowed in schema responses.

## API / Event Contract

### `GET /api/v1/schema/page/{id}`

Request:
- Authenticated browser request through apiserver.
- Tenant/context comes from the same auth/context mechanism used by other Console APIs.

Response:
- `200`: versioned schema envelope.
- `304`: optional later optimization if ETag is added.
- `401/403/404`: bounded error with `code`, `message`, trace/request identifier.

Events:
- None in this change. Existing navigation polling remains unchanged.

## State Machine

Schema page runtime state is local to the browser:

```
idle -> loading -> validating -> ready
                 -> incompatible
                 -> failed

ready -> querying dataSource -> ready | failed-region
ready -> executing action -> ready | action-error
```

No backend execution state is introduced by this change. Actions that create Operations use existing backend APIs and their existing Operation state machines.

## Failure Modes

- Schema page not found -> show safe page-load failure.
- Schema validation failure -> show safe schema failure.
- `minShellVersion` too high -> show upgrade-required UX and render no regions.
- Unknown component/dataSource/action endpoint ID -> fail closed and do not send HTTP request.
- Missing permission/capability/license/context -> hide/disable protected UI.
- Backend action failure -> show safe text error; backend remains source of truth.

## Security, Tenancy, and Secret Assessment

- Tenant isolation: schema endpoint must use authenticated subject and tenant context; no cross-tenant schema or data leakage.
- Secret handling: schemas and action payloads must not include Secret values or credentials; only ordinary request parameters and SecretReference strings where already allowed by backend contracts.
- Supply chain: remote bundle signing is out of scope. This change does not weaken PluginLoader digest checks.
- Authorization: UI gating is defense-in-depth; backend authorization is still required for every data/action request.
- Audit: no new audit requirement for read-only schema fetch. Mutating actions inherit the target API/Operation audit behavior.

## Performance, Capacity, and Observability

- Schema pages are small metadata responses; no large file proxying.
- DataSource queries should use paginated endpoints when lists are involved.
- Errors should preserve trace IDs from apiserver responses.
- Tests should include unit coverage for condition evaluation and endpoint resolution, plus E2E coverage for one interactive schema page.

## Migration Plan

1. Add apiserver schema endpoint and tests.
2. Wire Shell schema runtime with shared client, DataSourceManager, ActionEngine and ConditionEvaluator.
3. Add Web unit tests and schema-page E2E smoke.
4. Run Go/Web/OpenSpec validation.

Rollback:
- Remove or disable the schema route and dynamic schema navigation entry.
- Shell static plugin pages remain unaffected.

## Risks / Trade-offs

- Static repository may become a dumping ground -> keep this slice limited and define a later schema registry change if needed.
- UI gating could be mistaken for authorization -> tests must include backend-denied behavior and docs must state backend remains authoritative.
- E2E can become flaky -> keep the smoke mocked/deterministic and avoid live external dependencies.
- Schema type expansion can drift from docs -> add TypeScript tests around envelope/action/dataSource shape.

## Open Questions

- Should schema pages receive endpoint definitions embedded in the schema envelope or from a Shell-owned registry keyed by schema ID?
- Which first built-in page should be used as the canonical E2E fixture: cluster list, application list, or provider catalog?
- Should ETag support be included now or deferred until dynamic schema registry exists?
