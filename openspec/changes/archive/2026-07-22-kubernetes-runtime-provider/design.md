## Context

Runtime Driver v1 can invoke independent Providers. `kind-graphify` is a healthy Kubernetes v1.36.1 target exposed through the host kubeconfig. The first Provider must prove a real target mutation without becoming an arbitrary privileged YAML gateway.

## Goals / Non-Goals

**Goals:** execute bounded Deployment create/idempotent observe/delete, enforce namespace and ownership policy, preserve Operation metadata, expose health, and pass fake-client plus kind E2E.

**Non-Goals:** arbitrary manifests, cluster-scoped objects, Services, Secrets, Helm, multi-cluster discovery, autoscaling, or production mTLS.

## Architecture

```text
Operation Worker -> HTTP v1 -> Kubernetes Provider -> apps/v1 API
                        |               |
                 no DB/NATS creds   namespace RBAC
```

The Provider receives only Runtime Driver fields and scalar Deployment inputs. It never accesses PostgreSQL/NATS or proxies images/logs.

## Data Model And API

The existing v1 envelope is reused. `deploy` requires `namespace`, `name`, `image`, and optional `replicas` (1-10). `delete` requires `namespace`, `name`, and `expected_fencing_token`. Managed Deployments contain labels/annotations for managed-by, tenant, operation, step, idempotency, and fencing. Outputs return namespace, name, UID, resourceVersion, and action; checkpoint is `deployment:<namespace>/<name>:<uid>`.

## State Machine

```text
validate -> get -> absent/create -> wait available -> succeeded
                -> same owner/idempotency/fence -> observe -> succeeded
                -> conflict -> failed
delete -> get -> ownership/fence CAS -> UID-precondition delete -> succeeded
```

## Decisions

- Use typed client-go v0.34.5: it supports Go 1.24 and stable apps/v1 against the validated v1.36.1 server. Shelling out to kubectl was rejected due to injection, credential, and error-model risks.
- Accept scalar inputs rather than YAML. This constrains permissions and makes validation/conformance deterministic.
- Exact `ALLOWED_NAMESPACES` configuration is mandatory. Namespace creation and cluster-wide RBAC are rejected.
- Existing objects require HNB ownership plus matching tenant. Deploy replay also requires exact idempotency and fencing annotations. Different opaque fencing tokens conflict rather than overwrite; monotonic lease generations remain required for safe cross-lease takeover.
- Delete uses the resource UID precondition and requires the caller-provided expected current fencing token.
- Request body is limited to 64 KiB; rollout waits under the caller context. `/healthz` verifies process health, while startup performs Kubernetes discovery.

## Failure Modes

Invalid input/namespace/action returns 400; ownership/fencing conflict returns 409; Kubernetes unavailable or rollout failure returns 503/failed; cancellation stops polling. No failure is reported as success.

## Security And Operations

Tenant scope is copied from authoritative context and checked against owned resources. No Secret values or request bodies are logged. In-cluster installation uses namespace-scoped Deployment CRUD/get/list/watch only. Provider image signing/scanning is required before production; local E2E uses a host process. Audit metadata remains on the resource and in Operation audit. One request creates at most ten replicas; concurrency is bounded by the HTTP server and Worker. Kubernetes etcd owns backup/DR. Uninstall retains workloads.

## Compatibility Matrix And Conformance

| Component | Version | Status |
|---|---|---|
| Runtime Driver contract | 1.0.0 | required/tested |
| client-go | 0.34.5 | Go 1.24 compatible |
| Kubernetes | 1.36.1 | kind E2E tested |
| Resource | apps/v1 Deployment | deploy/delete only |

Conformance covers contract validation, namespace denial, ownership/fencing conflicts, idempotent replay, cancellation, rollout, UID-precondition delete, least privilege, and real kind create/replay/delete.

## Risks / Trade-offs

- [Client/server minor skew exceeds upstream support] -> stable apps/v1 is directly E2E tested; upgrade to client-go v0.36 when project Go reaches 1.26.
- [Opaque UUID fences cannot order attempts] -> reject token changes; add monotonic generation before supporting takeover/update.
- [Host kubeconfig is privileged in local E2E] -> production manifests use dedicated namespace Role and ServiceAccount.

## Migration Plan

Deploy RBAC/Provider, configure Worker endpoint, run canary, then enable plans. Rollback removes the mapping and Provider; workloads remain. No schema/data migration exists.

## Open Questions

- Monotonic fencing generation and controlled update semantics.
- Production mTLS identity and image registry location.
