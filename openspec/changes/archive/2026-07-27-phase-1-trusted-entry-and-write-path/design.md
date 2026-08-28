# Design: Phase 1 Trusted Entry and Canonical Runtime Write Path

## Context

The Phase 0 evidence baseline found incompatible token issuance and
verification, handler-local trust in identity headers, caller-controlled
tenant/actor fields, an unauthenticated platform API, and runtime lifecycle
seams which are not connected to the canonical Operation engine. Phase 1
defines the contract that closes those gaps while retaining current service
ownership.

## Goals

- Establish one fail-closed identity and authorization boundary.
- Make authenticated context authoritative and automatically propagated.
- Reduce public runtime mutation input to reviewed, typed intents.
- Prove the only target-write path is
  `Release/CompositionRelease -> ExecutionPlan -> Operation`.
- Bootstrap the Web Console from server-provided capabilities and permissions.
- Preserve the existing microkernel, Provider, app-market, and Operation
  foundations.

## Non-Goals

- Implement cluster reads, Kubernetes proxying, or Marketplace hardening.
- Make the gateway a business-state owner.
- Put executable Provider steps, credentials, or policy decisions in user
  requests or event payloads.
- Share internal databases across logical planes.
- Treat client-side route hiding as authorization.

## Architecture

```text
Browser / CLI / Automation
          |
          v
 Trusted Ingress
 token verify -> context derive -> authorize -> sanitize
          |
          +----------------------> Read APIs
          |
          v
 Typed RuntimeIntent
          |
          v
 Existing domain owner (including app-market)
          |
          v
 Release / CompositionRelease
          |
 policy + compatibility + approval
          v
 immutable ExecutionPlan
          |
 atomic commit
          v
 Operation Store + Outbox -> Worker -> Provider -> RuntimeTarget
```

The ingress owns no business database. App Market owns products and releases.
The platform owns execution state. Providers own only adapter-specific
execution, and RuntimeTargets are never addressed directly by public callers.

## Trusted Identity Model

The normalized context contains:

| Field | Source | Rule |
|---|---|---|
| subject ID | verified `sub` claim | required and immutable |
| subject type | verified claim | user, workload, or service |
| tenant ID | verified active membership/default plus explicit selection | request value must be authorized, never self-asserted |
| project/environment/namespace | route/resource selection | must be within grants |
| permissions | policy decision | not accepted from the client |
| token ID and auth time | verified claims | audit only; token is never logged |
| correlation ID | validated request value or server-generated | propagated end to end |

Tokens use a versioned claim profile with explicit issuer, audience, expiry,
not-before, algorithm, and key identifier. Verifiers reject `none`, algorithm
substitution, unknown keys, missing tenant membership, and expired or
not-yet-valid credentials. Key rotation supports overlapping verification keys
for a bounded interval and a separately protected signing key.

Ingress removes all inbound identity headers before setting internal context.
Downstream components consume a typed context, not raw headers. Service calls
use workload identity or short-lived service credentials with audience
restriction; they may not replay an end-user bearer token to arbitrary
services.

## Authorization Model

The decision input is:

```text
subject + tenant + resource-kind + resource-id +
project/environment/namespace scope + action + request risk
```

The decision output is `allow` or a fail-closed denial plus policy version and
reason code. High-risk writes may additionally require an approval policy, but
approval does not replace the initial permission check. Resource lookup and
authorization use the same tenant predicate. Cross-tenant non-disclosure may
return 404, while audit records preserve the denial reason.

Capability discovery and authorization are distinct: capability indicates
that a deployment can offer a feature; permission indicates that the subject
may use a concrete action in scope.

## Runtime Intent Contract

Public callers submit a versioned intent such as:

```json
{
  "apiVersion": "hnb.io/v1",
  "kind": "InstallRelease",
  "metadata": {
    "idempotencyKey": "client-stable-key",
    "correlationId": "uuid"
  },
  "spec": {
    "releaseId": "immutable-release-id",
    "targetRef": "authorized-target-ref",
    "scopeRef": "authorized-scope-ref",
    "parameters": {},
    "secretReferences": []
  }
}
```

Allowed kinds include install, uninstall, upgrade, rollback, configuration
change, and other separately reviewed domain actions. The caller cannot submit
steps, Provider IDs, command payloads, target credentials, artifact bytes,
policy outcomes, approval outcomes, or fencing tokens.

The domain owner validates entitlement and immutable Release identity. A
planner pins artifact digests, target capability snapshot, Provider versions,
policy results, parameters, and SecretReferences into an immutable
ExecutionPlan. A single transaction commits the intent reference,
ExecutionPlan, Operation, initial steps, audit record, read model, and outbox
record.

Idempotency is scoped to authenticated tenant plus intent kind and key.
Replays with the same semantic digest return the existing Operation; reuse with
a different digest is rejected.

## API and Event Contracts

Minimum logical endpoints:

- authenticated session/bootstrap: subject, selected tenant, memberships,
  capabilities, permissions, policy/version metadata;
- submit typed runtime intent;
- query intent/Operation by tenant-scoped identifier;
- approve, reject, or cancel through authorized Operation controls;
- discover capabilities separately from permissions.

All public schemas are versioned and generated from the contract source.
Durable messages carry references and immutable digests, not secrets,
credentials, artifact bytes, or caller-authored executable plans. The Operation
outbox remains the source of execution commands.

## State Machines

Authentication:

```text
Unauthenticated -> Verified -> Authorized -> Handled
       |              |            |
       +-------------> Denied <-----+
```

Intent:

```text
Received -> Validated -> Planned -> OperationCommitted
    |           |          |
    +----------> Rejected <-+
```

`OperationCommitted` is irreversible as an audit fact. Cancellation and
rollback are new authorized Operation transitions, not deletion of history.

## Data Model and Migration

Logical additions/reconciliation:

- identity subject and tenant membership with version/disable state;
- role/policy binding with scoped resource selectors;
- immutable runtime intent with semantic digest and Operation reference;
- security audit record with subject, scope, decision, policy version, and
  correlation;
- key metadata containing identifiers and lifecycle only, never private key
  material in ordinary application tables.

Migration authors must first reconcile Phase 0 collisions (`005`/`021` and
`010`/`022`). Forward migrations are additive, constraints are backfilled
before enforcement, and down migrations must not erase audit or completed
Operation facts. A schema compatibility check blocks rollout if duplicate
objects differ.

## Web Console Integration

The Console:

- stores tokens only through the approved session strategy;
- obtains subject, active tenant, capabilities, and permissions from bootstrap;
- builds navigation and plugin activation from capabilities;
- gates routes and actions from permissions;
- attaches authentication and correlation through one API client;
- clears privileged state on tenant switch, logout, expiry, or permission
  version change;
- treats server denial as authoritative and displays a safe recovery path.

Plugins cannot declare themselves authorized. Their manifests declare required
capabilities and permissions which the shell intersects with server data.

## Cross-Plane Proof

- No plane reads another plane's internal tables.
- App Market publishes immutable product/release facts through contracts.
- The platform resolves those facts into plans and Operations.
- Artifact bytes remain in OCI/data-plane storage.
- AI may propose an intent but cannot approve it or bypass Operation.
- No service proxies target credentials or executable plans through a plane
  boundary.

## Security and Privacy

- Tenant isolation: required at ingress, authorization, repository, events,
  audit, and cache keys.
- Secrets: `SecretReference` only; no token or secret values in logs/events.
- Permissions: deny by default; cache invalidation is versioned and bounded.
- Audit: append-only logical history for authentication, authorization,
  intent, approval, cancellation, and execution linkage.
- Supply chain: artifacts are digest-pinned; Phase 4 adds publication gates.
- Abuse controls: bounded payloads, per-subject/tenant rate limits, replay
  protection, and uniform error envelopes.

## Performance and Capacity

- Authentication and authorization p95 budget: 50 ms excluding external
  identity-provider latency; local verification is preferred.
- Public request body maximum: 1 MiB; intent parameter count and nesting are
  bounded by schema.
- Permission cache maximum stale interval: 60 seconds and immediately
  invalidated on subject disable where the identity system supports it.
- Audit and intent retention are configurable but cannot be shorter than the
  Operation evidence retention.

## Observability

Required signals:

- auth successes/failures by reason and issuer, without subject cardinality in
  metric labels;
- authorization decisions and latency;
- intent validation/planning/commit outcomes;
- idempotent replay/conflict counts;
- Operation/outbox correlation and lag;
- Console bootstrap and permission-version failures;
- structured security audit with correlation and trace identifiers.

## Failure Modes

| Failure | Required behavior |
|---|---|
| Unknown/expired token or signing key | fail closed; no handler execution |
| Tenant selection not in membership | deny and audit |
| Policy service/cache unavailable | fail closed for protected actions |
| Domain owner or planner unavailable | no Operation and no target effect |
| Transaction fails after planning | no partial Operation/outbox commit |
| Message bus unavailable after commit | outbox retries; Operation remains authoritative |
| Permission changes during Console session | next bootstrap/decision removes access; server still denies stale UI |
| Legacy route still trusts headers | keep route non-public and block Phase 1 exit |

## Compatibility Matrix and Conformance

| Surface | Phase 1 contract | Conformance |
|---|---|---|
| KubernetesTarget | receives only fenced Provider commands | negative direct-write test |
| ContainerEngineTarget | same | negative direct-write test |
| EdgeRuntimeTarget | same; offline queue stays Operation-owned | reconnect/replay test |
| Gateway | verifies/normalizes context, owns no business facts | spoofing and key-rotation tests |
| Provider | accepts generated step contract only | caller-authored step rejection |
| Web Console | server bootstrap drives visibility and actions | plugin/permission E2E |

## Upgrade, Rollback, Backup, and DR

1. Inventory legacy identity inputs and publish compatibility telemetry.
2. Deploy shared verification/context libraries and contract tests.
3. Enforce on read-only routes, then typed write routes.
4. Disable public legacy write routes before enabling canonical intents.
5. Roll back binaries only while retaining enforcement at the outer boundary.
6. Back up identity binding, policy, intent, Operation, outbox, and audit stores
   using their owning service procedures; test point-in-time recovery and
   correlation preservation.

## Alternatives Considered

1. Trust a front proxy to set identity headers. Rejected because downstream
   exposure or misconfiguration reopens impersonation and lacks a typed
   context contract.
2. Let each service verify different tokens. Rejected because issuer, secret,
   claim, and tenant semantics drift.
3. Accept arbitrary Operation steps from trusted users. Rejected because
   authentication does not make executable plans safe or policy-complete.
4. Create a new Marketplace ingress. Rejected because `app-market` is the
   existing domain foundation.

