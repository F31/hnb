# Cross-format semantics

| Meaning | OpenAPI / JSON | Protobuf | Required |
|---|---|---|---|
| Tenant scope | `tenantId` | `tenant_id` | Yes for tenant resources and events |
| Project scope | `projectId` | `project_id` | No |
| Environment scope | `environmentId` | `environment_id` | No |
| Actor | `actorId` | `actor_id` | Yes for request context; optional for system events |
| Correlation | `correlationId` | `correlation_id` | Yes |
| Causation | `causationId` | `causation_id` | No |
| Idempotency | `idempotencyKey` | `idempotency_key` | Yes for writes and event envelopes |
| Schema version | `schemaVersion` | `schema_version` | Yes for event envelopes |
| Aggregate version | `aggregateVersion` | `aggregate_version` | No; required when optimistic concurrency applies |
| Verified subject | `subjectId` | `subject_id` | Yes for trusted contexts and decisions |
| Subject kind | `subjectType` | `subject_type` | Yes; user, workload, or service |
| Tenant membership | `membershipId` | `membership_id` | Yes for a trusted interactive context |
| Selected token tenant | `tenantId` | `tenant_id` | Yes in access-token claims; selected membership must resolve to this tenant |
| Authorization action | lower-camel enum value | prefixed `AUTHORIZATION_ACTION_*` enum | Yes for a decision or scoped permission |
| Policy evidence | `policyVersion` | `policy_version` | Yes for an authorization decision |
| Signed policy snapshot | `scopedPermissions` | `scoped_permissions` | Yes in access-token claims and trusted contexts; may be empty |
| Service action ceiling | `allowedActions` | `allowed_actions` | Required for workload/service tokens; exact enum values only |
| Runtime intent kind | PascalCase string | prefixed `RUNTIME_INTENT_KIND_*` enum | Yes |
| Immutable digest | `semanticDigest` | `semantic_digest` | Yes for accepted intents and plans |

JSON names use lower camel case. Protobuf source uses snake case and generated JSON
mapping uses lower camel case. IDs are opaque strings unless the field explicitly declares
UUID format. Times are UTC RFC 3339 strings or `google.protobuf.Timestamp`.

Tenant IDs received from a client are context, not proof of authorization. The owning API
must derive and verify scope from authenticated identity and resource ownership.

`AccessTokenClaims` contains verified claim values, never an encoded credential. Its
selected `membershipId` must occur in `tenantMembershipIds`, and `tenantId` is the
tenant resolved for that membership by the issuer at signing time. The v1 profile is
ES256-only, carries explicit non-wildcard audiences, and has a maximum 60-second
access lifetime/revocation propagation bound. Each verifier requires its own audience
to be present; additional explicitly approved audiences are allowed. The issuer adds
the active `policyVersion` and the canonical scoped permissions resolved for the
selected membership. This is a signed server-side policy-decision snapshot, not a
client assertion. It contains at most 64 entries. Tenant IDs are exact and can never
be `*`; `resourceKind` may be the explicit `*` wildcard, while `action` is always one
of the published action enum values and cannot be wildcarded. Each optional
project/environment/namespace/resource selector narrows a grant and must match
exactly. `scoped_roles.permissions` is a strict JSON array of these
`ScopedPermission` objects: unknown keys, missing required fields, invalid hierarchy,
wildcard tenants, and unknown actions make issuance fail closed. Active binding
scope is applied to each role permission; binding actions are merged with the role's
resource selectors (or with an explicit resource binding). Missing or invalid active policy data denies
issuance or authorization. Permission changes reach verifier-only services when the
at-most-60-second access token expires; apiserver additionally rechecks subject and
membership disable state on every request.
`ServiceIdentity` is the service-boundary projection of `AccessTokenClaims`, not a
second credential format. Its `subjectId`, workload/service `subjectType`, one exact
target audience, tenant IDs, action ceiling, and scoped permissions all come from the
verified access-token claims. `allowedActions` must equal the distinct actions in the
signed `scopedPermissions`; it cannot add authority. A user token can never be
projected to `ServiceIdentity` or used as a service credential. Service verifiers
reject multiple or wildcard audiences, actions, and tenants.
`TrustedRequestContext` is ingress-authored and replaces caller-supplied identity
headers or body fields and carries the same verified policy snapshot. Capabilities describe deployment availability; scoped
permissions describe subject authority, and both are required to expose a Console
feature.

Public callers author `RuntimeIntent` only. They cannot provide steps, Provider
commands, credentials, artifact bytes, fencing data, or policy/approval results.
`ExecutionPlan` is an immutable server-authored snapshot; Protobuf and generated
clients transport that snapshot but do not grant callers authority to construct it.
All public errors use `application/problem+json` and `ProblemDetails`.

Cluster browser identity is always `targetId` plus `targetKind`; cluster contracts admit
only `KubernetesTarget` and `EdgeRuntimeTarget`. Lifecycle, health, connectivity, and
freshness are independent server projections. `STALE` never replaces the last-known
lifecycle, health, or connectivity value. The legacy `resource.cluster.status`
dictionary is a display-only precedence projection and cannot authorize writes.

RuntimeTarget observations are ordered by authenticated observer generation and sequence,
not by timestamps. The authenticated lease must match tenant, target, kind, observer, and
generation fields. A source reset establishes a strictly greater generation before its
observations are accepted. Full node inventories tombstone omissions; Delta inventories
change only listed nodes. Contract validation additionally enforces the 300-second future
clock-skew and 1 MiB encoded observation bounds that JSON Schema cannot compare dynamically.

The lifecycle compatibility matrix is the single routing authority. Kubernetes supports
create/import/upgrade/unmanage through `runtime-target.lifecycle.kubernetes`; Edge supports
import/upgrade/unmanage through `runtime-target.lifecycle.edge`, while Edge create is
unsupported. Lifecycle Step input schemas are planner-authored and intentionally contain no
Provider selection, Step type, command, execution URL, callback URL, or secret value.
