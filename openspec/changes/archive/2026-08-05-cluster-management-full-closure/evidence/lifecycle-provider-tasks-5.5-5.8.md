# Tasks 5.5-5.8 Kubernetes/Edge Lifecycle Provider Implementation

## New independent runtime-target lifecycle provider

A dedicated `cmd/runtime-target-lifecycle-provider` module now implements the
namespaced runtime-target lifecycle steps. It is deliberately separate from the
generic workload `kubernetes-provider` / `edge-provider` binaries, which remain
responsible for Deployment / EdgeApplication workloads and are **not** routed
lifecycle steps (no generic-Provider fallback, per RT-009).

- `internal/provider/profile.go` resolves the two lifecycle provider IDs:
  `runtime-target.lifecycle.kubernetes` (KubernetesTarget, Agent observation,
  kubeconfig purpose) and `runtime-target.lifecycle.edge` (EdgeRuntimeTarget,
  CloudCore observation, cloudcore-client purpose).
- `internal/provider/input.go` whitelists and canonicalizes planner-authored
  step inputs against the published lifecycle step-input Schemas. It rejects
  unknown keys, schema drift (`inputs.schemaVersion` must be `1.0.0`), targetKind
  mismatch, step-type/action mismatch, idempotency-key and fencing-generation
  mismatch, and unsafe CloudCore endpoints (userinfo / query / fragment).
- `internal/provider/memory.go` provides the default in-memory management-relation
  manager and observer registry ports. Real runtimes replace these ports; they
  never write Read Models directly.
- `internal/provider/http.go` serves `GET /healthz` and `POST /v2/steps:execute`
  with protocol `2.0.0`, echoing attempt / idempotency / fencing / provider
  version / digest / protocol version and rejecting mismatches with typed
  `StatusError`s (`INVALID_REQUEST`, `SCOPE_DENIED`, `UNSUPPORTED_ACTION`,
  `RESOURCE_CONFLICT`, `FENCED`, `TARGET_UNAVAILABLE`, `CANCELLED`).

## Runtime minimal-privilege secret handling

- `internal/provider/types.go` defines the `SecretResolver` port. The server only
  passes a non-sensitive `SecretReference` (provider/scope/name/version) plus the
  tenant, lifecycle provider ID and purpose; no secret value is ever part of the
  lifecycle step inputs or outputs, matching the planner's Schema allowlist.
- `MetadataOnlySecretResolver` validates the reference shape as a fail-closed
  development default; production runs a real resolver port.

## Idempotency, fencing, cancellation and ownership

- `LifecycleManager.Apply` enforces: same tenant + targetKind, fencing-generation
  monotonicity, and equal-generation exact-replay (step/operation/idempotency/
  attempt/action must all match) before returning the stored result.
- `unmanage` fails closed unless the target is currently managed, and records a
  `managed=false` relation without deleting any non-Operation-owned resources.
- The HTTP handler cancels cleanly on `context.Canceled`/`DeadlineExceeded`
  (408 `CANCELLED`, retryable).

## Operation-worker input preservation

- `cmd/operation-worker/internal/driver/http.go` now forwards the planner-authored
  `execution.Inputs` as `map[string]any` instead of stringifying nested values,
  so `credentialSecretRef` objects reach the provider unchanged.

## Verification

- `go build ./...` and `go test ./... -race -count=1` in
  `cmd/runtime-target-lifecycle-provider`: pass (Kubernetes create/import/upgrade
  routing, Edge register routing + endpoint validation + secret redaction,
  replay/fencing, schema-drift rejection, cancellation).
- `go build ./...` and `go test ./... -count=1` in `cmd/operation-worker`: pass.
- `docker-compose` config includes a `runtime-target-lifecycle-provider` service
  and routes the two lifecycle provider IDs to it with pinned
  `requiredProvider`/`providerVersion`/`providerDigest`.

## Not yet a live-stack conformance

The default `MemoryManager` and `MetadataOnlySecretResolver` are development
ports. Task checkboxes for 5.5-5.8 remain **unchecked**: producing the required
live DRILL evidence (real Kubernetes/CloudCore fixtures, Agent/CloudCore
observation convergence to the projector, real side effects) is still blocked on
an actual Kubernetes/Edge cluster and the task-6.x projector stack. This change
is the minimal in-repo implementation of the lifecycle provider boundary without
writing Read Models.
