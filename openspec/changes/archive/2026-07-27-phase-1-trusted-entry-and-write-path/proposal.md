# Change: Phase 1 Trusted Entry and Canonical Runtime Write Path

## Change Metadata

| Field | Value |
|---|---|
| Change ID | `phase-1-trusted-entry-and-write-path` |
| Tier | T0 security and control-plane foundation |
| Status | Specification ready for independent review |
| Baseline | HNB Cloud OpenSpec V3.8.6 plus `establish-phase-0-project-truth` |
| Affected planes | Cross-cutting authenticated ingress; App Market and Runtime Governance integration; Web Console |
| Affected specs | New `trusted-platform-ingress`, `canonical-runtime-intent`, and `console-access-contract` capabilities |
| Dependencies | `establish-phase-0-project-truth`, `operation-engine-core`, `platform-api-gateway`; identity bootstrap changes already archived into main specs |
| Required before | Phase 2, Phase 3, and Phase 4 |

## Why

Phase 0 proved that several externally reachable services accept tenant, actor,
or execution data supplied by the caller instead of deriving it from a trusted
identity boundary. It also proved that direct Release publication and
Marketplace lifecycle events are not yet demonstrably constrained to the
canonical runtime mutation chain.

Phase 1 freezes the trust and write contracts before adding resource access or
proxy features. It does not replace existing services. It makes the existing
identity, platform API, Operation engine, and Web Console foundations converge
on one authenticated context and one runtime write path.

## What Changes

- Define one verified access-token contract and key/issuer/audience lifecycle
  for interactive users, automation, and service identities.
- Require ingress middleware to derive tenant and actor context from verified
  claims; caller-supplied identity headers and JSON fields become non-authoritative.
- Define scope-aware authorization for tenant, project, environment,
  namespace, resource, and action.
- Define a schema-first `RuntimeIntent` boundary which resolves only into an
  immutable `Release/CompositionRelease`, `ExecutionPlan`, and persisted
  `Operation`.
- Prohibit public callers from supplying arbitrary executable steps, Provider
  commands, target credentials, or policy results.
- Require Release publishing, channel promotion, install, uninstall, upgrade,
  rollback, and configuration mutation to use policy-gated canonical intents.
- Define Web Console bootstrap data for authenticated subject, tenant context,
  capabilities, permissions, and feature visibility.
- Require end-to-end correlation and security audit evidence.

## Capabilities

### New Capabilities

- `trusted-platform-ingress`: token verification, trusted context derivation,
  authorization, service identity, and audit boundary.
- `canonical-runtime-intent`: schema-first intent submission and the only
  admissible conversion to the Operation engine.
- `console-access-contract`: capability- and permission-driven authenticated
  console bootstrap.

### Modified Behavior

These deltas constrain implementations of `identity-tenancy`,
`contracts-events`, `platform-kernel`, `composition-operation`,
`portal-experience`, and `app-market` without changing their approved
architectural ownership.

## User Value

Users receive consistent sign-in, tenant isolation, permissions, and audit
behavior across the Console and APIs. Operators can prove that every runtime
change passed the same immutable plan, policy, approval, and Operation
machinery.

## Non-Goals

- No cluster resource read API; that is Phase 2.
- No generic Kubernetes API proxy; that is Phase 3.
- No Marketplace catalog or supply-chain feature completion; that is Phase 4.
- No replacement identity provider, API gateway, Marketplace, Operation
  engine, Provider, or RuntimeTarget.
- No direct Kubernetes, container-engine, or edge mutation API.
- No authorization based on unverified `X-Tenant-ID`, `X-User-ID`, or body
  identity values.

## Impact

### API and Event Compatibility

Existing routes may remain during a bounded migration window, but identity
headers/body fields become ignored or rejected at the trusted boundary.
Schema-first v1 contracts remain additive. Runtime command subjects SHALL be
produced only after the Operation transaction commits through its outbox.

### Data and Migration

Implementation is expected to add or reconcile durable identity bindings,
authorization policy versions, immutable intent records, and security audit
records. Exact migrations must be reconciled with the Phase 0 migration
collision blockers before allocation of a new migration number. No existing
table may be silently recreated or repurposed.

### Security

Fail closed on token, issuer, audience, key, tenant membership, scope,
permission, policy, or intent validation failure. Identity headers are
sanitized at ingress. Secrets remain `SecretReference` only.

### Resources and Capacity

Authorization and context derivation add one bounded policy decision per
request. The implementation must specify cache invalidation and a maximum
staleness window; it may not trade tenant isolation for availability.
Intent and audit payloads remain bounded and exclude artifact bytes.

### Observability

Metrics and traces must distinguish authentication failure, authorization
denial, policy denial, idempotent replay, intent rejection, Operation commit,
and outbox lag without logging credentials or sensitive request bodies.

### Migration and Rollback

Roll out trusted verification in observe-only mode only for compatibility
measurement, then enforce per route group. Dual interpretation of trusted and
caller-supplied identity is forbidden. Rollback may restore the previous
binary, but public exposure of header-trusting routes must remain disabled.
Already committed Operations continue from their authoritative store.

## Phase 1 Exit Criteria

1. A single token/claim contract is used by issuer, middleware, Console, API,
   and service-to-service tests.
2. All in-scope public routes derive tenant and actor from verified context and
   reject header/body impersonation.
3. Authorization is proven at tenant/project/namespace/resource/action scope,
   including cross-tenant negative tests.
4. Public runtime mutations accept typed intents, not caller-authored steps.
5. Release, install, uninstall, upgrade, rollback, and configuration mutation
   demonstrably create or control the canonical Operation path.
6. Web Console routes and actions are hidden and server-rejected from the same
   capability/permission contract.
7. Audit and correlation evidence links subject, intent, Release,
   ExecutionPlan, Operation, policy, approval, and final outcome.
8. Contract, unit, integration, security, and end-to-end tests pass; migration,
   rollback, and key-rotation drills are recorded.

