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

JSON names use lower camel case. Protobuf source uses snake case and generated JSON
mapping uses lower camel case. IDs are opaque strings unless the field explicitly declares
UUID format. Times are UTC RFC 3339 strings or `google.protobuf.Timestamp`.

Tenant IDs received from a client are context, not proof of authorization. The owning API
must derive and verify scope from authenticated identity and resource ownership.
