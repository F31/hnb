## Why

apiserver currently mixes Console BFF responsibilities with direct access to platform domain tables, while platform-api and apiserver expose different error and trace conventions. This makes browser/API topology, authorization and testing harder to reason about as HNB grows.

## What Changes

- Change ID: `harden-apiserver-platform-boundary`
- Tier: T0 boundary hardening.
- Impacted planes: apiserver, platform-api, IAM, public contracts, deployment config.
- Define apiserver as the unified northbound API/BFF for Web Console and ordinary user CLI traffic.
- Define platform-api as the platform resource/intent domain API that re-authorizes resource instances and owns RuntimeTarget/Cluster/RuntimeIntent/ExecutionPlan/Operation records.
- Remove direct platform domain table access from apiserver request handlers in favor of application services and platform-api clients.
- Standardize trace/correlation headers and minimal error shape across apiserver and platform-api.
- Dependencies: none; builds on current `PLATFORM_API_URL` forwarding and shared `pkg/iam`.
- Migration impact: browser calls remain under apiserver paths; internal platform-api paths remain versioned `/v1/...`.
- Rollback strategy: keep read-only SQL fallback for one release behind config only where required, then remove after deployment verification.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `platform-kernel`: clarify northbound/domain service boundaries and single-direction synchronous dependencies.
- `contracts-events`: require consistent error, trace and DTO contracts for apiserver/platform-api interactions.

## Impact

- Affected code: `cmd/apiserver`, `cmd/platform-api`, `pkg/iam`, OpenAPI contracts, deploy compose/helm values, Web API client tests.
- APIs: apiserver keeps browser-compatible `/api/v1/...`; platform-api keeps internal/domain `/v1/...`; no platform-api-to-apiserver synchronous callbacks.
- Dependencies: no new middleware; reuse PostgreSQL, Outbox, NATS and IAM packages.
- Security risks: proxying can accidentally skip domain authorization; mitigated by retaining platform-api resource-level authorization and forwarding trusted identity only through verified tokens.
- Resource budget: apiserver adds bounded client timeouts and retry policy; no large caches or new data plane.
- Observability: trace IDs must flow across apiserver and platform-api; errors must include stable code and trace/request ID.
- Exit criteria: browser traffic routes through apiserver, platform-api refuses unauthenticated direct user requests, handlers are unit-testable via interfaces, and error/trace contract tests pass.
