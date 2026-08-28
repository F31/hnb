## Context

The Operation Worker already owns authoritative validation, leases, retries, checkpoints, timeout cancellation, transactional Step commits, and Outbox publication. Its `StepRunner` is deliberately `nil`, while the completed RuntimeTarget engine only models targets and an in-memory registry. The missing boundary is a real, testable call from the T0 worker to independently deployed T1 Providers without sharing databases or allowing Providers to commit Operation state.

Stakeholders are Operation Engine owners, Provider authors, platform operators, and auditors. The boundary must remain fail-closed and must not expose Step Inputs in logs.

## Goals / Non-Goals

**Goals:**
- Route each Step to an explicitly configured Provider endpoint by authoritative `provider_id`.
- Define a versioned HTTP request/response contract for synchronous, cancellable execution.
- Preserve idempotency, checkpoint resume, timeout, tenant scope, and fencing metadata end to end.
- Reject invalid configuration and malformed Provider behavior.
- Supply a conformance suite usable by future Kubernetes, container, and edge Providers.

**Non-Goals:**
- Implement Kubernetes, container engine, edge, Gateway, or external-service side effects.
- Discover Providers dynamically or persist ProviderRegistry entries.
- Add mTLS/Agent transport in this change; endpoint network identity is a deployment concern until that transport change lands.
- Change DAG dispatch, Operation state transitions, pause/cancel lease revocation, or compensation.
- Proxy artifacts, logs, Secrets, or other data-plane payloads.

## Architecture

```text
JetStream StepRequested
        |
        v
Operation Worker -- read/lease/fence --> PostgreSQL Operation Store
        |
        | POST configured endpoint (v1 execution contract)
        v
Runtime Driver / Provider -- side effect --> RuntimeTarget
        |
        v
Operation Worker -- fenced transaction --> Step + Audit + Read Model + Outbox
```

No Provider reads or writes the Operation database. No execution path bypasses the Worker, and the HTTP request carries control metadata rather than artifacts or Secret values.

## Data Model

No persistent schema changes are required. Startup configuration is a JSON object whose keys are Provider IDs and values are absolute `http` or `https` execution URLs. The Worker converts the existing `ExecutionContext` to the wire request; the Provider response carries string outputs, an opaque checkpoint, a terminal call status, and an optional sanitized error.

## API Contract

The Worker sends `POST` with `Content-Type: application/json`:

```json
{
  "schemaVersion": "1.0.0",
  "execution": {
    "step_id": "...",
    "operation_id": "...",
    "tenant_id": "...",
    "project_id": "...",
    "environment_id": "...",
    "step_type": "deploy",
    "inputs": {},
    "provider_id": "k8s-prod-01",
    "checkpoint": "...",
    "idempotency_key": "...",
    "fencing_token": "..."
  }
}
```

The response is:

```json
{
  "schemaVersion": "1.0.0",
  "status": "succeeded",
  "outputs": {},
  "checkpoint": "...",
  "error": ""
}
```

`failed` responses can include a checkpoint so the existing retry transaction can resume. Responses over 1 MiB, unknown fields, trailing JSON, unsupported versions, unknown statuses, non-2xx status, or contradictory success/error fields are execution failures. Context cancellation closes the HTTP request. Providers MUST apply the idempotency key and validate the opaque active fencing token before each side effect; this client propagates but cannot independently prove Provider enforcement.

No new NATS event is introduced. Existing Step completion/failure events remain transactionally published by the Worker only.

## State Machine

```text
configured -> request_sent -> succeeded -> fenced_commit
                           -> failed    -> checkpoint_retry | terminal_failure
missing/invalid route -----------------> failed closed
timeout/lease loss --------------------> cancel request -> retry/failure
```

Provider status never directly changes Operation state. Only the existing fenced Worker transaction does so.

## Decisions

### Explicit JSON endpoint map

Use `RUNTIME_PROVIDERS` as a JSON object rather than comma-delimited syntax so Provider IDs and URLs are unambiguous and startup validation is deterministic. Dynamic service discovery was rejected because no authoritative Provider registry service exists yet and silent endpoint changes weaken auditability.

### Standard HTTP client and synchronous request

Use Go `net/http` and the Step context. This adds no dependency, naturally carries cancellation, and matches the current synchronous `StepRunner`. NATS request/reply was rejected because it would add another delivery/ack state machine inside an already leased command. In-process plugins were rejected for supply-chain isolation and upgrade safety.

### Strict bounded decoding

Limit responses to 1 MiB and disallow unknown/trailing fields. Provider protocol drift must fail visibly instead of being interpreted as success. Request bodies are naturally bounded by authoritative Step Inputs; future larger payloads must use artifact references, not this API.

### Provider-selected endpoint path

Configuration stores the complete execution URL rather than constructing paths. This supports sidecar and remote Providers while keeping the v1 payload stable. Only `http` and `https` URLs with a host are accepted.

## Compatibility Matrix

| Provider / target | Contract v1 | Checkpoint | Cancellation | Fencing conformance | Status |
|---|---:|---:|---:|---:|---|
| KubernetesTarget Provider | required | required when resumable | required | required | adapter ready, implementation out of scope |
| ContainerEngineTarget Provider | required | required when resumable | required | required | adapter ready, implementation out of scope |
| EdgeRuntimeTarget Provider | required | required | required | required | adapter ready, transport out of scope |
| ExternalServiceConnector | prohibited for container deployment | operation-specific | required | required for writes | not a deploy target |

Conformance tests verify routing, exact v1 fields, idempotency/checkpoint/fencing propagation, success, resumable failure, malformed/oversized responses, non-2xx behavior, and cancellation. Concrete Providers must run the same behavioral suite plus target-specific side-effect tests before production enablement.

## Security And Operations

- Tenant isolation: scope comes from the authoritative Operation, not the incoming message; routing is by exact Provider ID. Providers must authorize the supplied tenant scope.
- Secrets: URLs must not contain user info; Inputs must use SecretReference under existing policy. Bodies and responses are never logged.
- Supply chain: no in-process Provider binaries or new libraries are loaded. Provider images remain independently signed/scanned.
- Permissions: the Worker gains only egress to configured endpoints; Providers receive no Operation DB or NATS credentials.
- Audit: existing audit and Outbox commits record Provider ID, result, checkpoint, and fencing token; Provider-local side-effect audit is part of Provider conformance.
- Performance/capacity: one request per active Step, bounded by existing worker concurrency and Step timeout; 1 MiB max response. HTTP transport reuses connections.
- Observability: classify configuration, routing, transport, protocol, and Provider failures while omitting request/response bodies. Metrics are deferred because this service has no metrics subsystem yet.
- Upgrade: contract version `1.0.0` is explicit. A future incompatible version uses a separate adapter/configuration rollout.
- Backup/restore/DR: no new state exists. PostgreSQL and Provider-owned state retain their existing DR policies.
- Install/uninstall: configure/remove endpoint mappings and network policy. Removing a mapping causes fail-closed execution.

## Failure Modes

- Missing/unknown Provider route -> return an execution error; Worker retry/terminal policy applies.
- DNS/connectivity/non-2xx -> return a classified Provider error with any valid checkpoint; never commit success.
- Malformed, oversized, or unsupported response -> protocol error; never trust partial outputs.
- Timeout, shutdown, lease-renewal failure, or NATS ack extension failure -> cancel HTTP request; fenced commit prevents stale completion.
- Provider ignores cancellation or fencing -> stale external side effects remain possible; production conformance and Provider-side fencing are mandatory mitigations.

## Risks / Trade-offs

- [Opaque UUID fencing cannot establish ordering by itself] -> Providers must validate active tokens according to their target adapter protocol; a monotonic cross-process fencing generation is a follow-up before Providers with non-conditional APIs are certified.
- [Plain HTTP could expose sensitive metadata] -> allow it for local/test deployments but require HTTPS and network policy in production; mTLS is a separate transport change.
- [Static mappings require restart] -> prefer explicit, auditable behavior now; add registry watches only with an authoritative registry.
- [Synchronous calls hold Worker capacity] -> existing pool and Step timeout bound usage; asynchronous Provider jobs can return checkpoints in a future contract.

## Migration Plan

1. Deploy Provider endpoints and pass v1 conformance tests.
2. Add exact endpoint mappings to `RUNTIME_PROVIDERS` and egress policy.
3. Roll out Worker; invalid mappings stop startup before message consumption.
4. Dispatch a canary Operation and verify Provider plus Operation audit correlation.
5. Roll back by restoring the previous Worker image or removing mappings; pending messages remain retryable and no database migration is needed.

## Open Questions

- Define a monotonic fencing generation or target-native compare-and-set protocol in the concrete Provider change.
- Select the platform-standard mTLS identity and certificate rotation mechanism.
- Decide whether a future authoritative Provider Registry replaces static startup configuration.
