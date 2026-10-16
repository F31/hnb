# Real runtime-target import through the formal API — 2026-10-13

## Result and scope

**Partial but substantive.** A `KubernetesTarget` import was executed end to end
through the public chain: Portal → apiserver → platform-api intent → ExecutionPlan →
Operation → NATS outbox → operation-worker → service-identity call → lifecycle provider →
real Kubernetes connectivity check → observer registration. The Operation reached
`succeeded` with `1/1` step completed.

This is a single import of the evaluation cluster into itself, executed with an
evaluation-only credential. It is not a Minimal installation, not an HA/offline
result, not a clean-host certification, and the service-credential lifecycle remains
unfinished (see Remaining blockers).

## Run identity

- Run IDs: `startup-worker-provider-binding-20261013`,
  `startup-worker-live-import-20261013`, `startup-operation-idempotency-20261013`.
- Base commit: `c23f6e4128e92d7a9457807dce5b10e0b0704e73` plus uncommitted working-tree changes.
- Environment: `106.75.34.133`, Ubuntu 24.04, 2 vCPU/3914 MiB, single-node K3s.
- Successful operation: `d423131c-fbd1-4301-b757-ddedfb5b5a2c`, created
  `2026-10-13T15:47:13Z`; worker committed the step with outbox event
  `42b47129-4f1e-4d49-8f76-53588d77f88a`.
- Imported target: `f13b8fa2-5af7-546d-894f-adb2708fb0c8`, `REGISTERING`, observer
  source `agent`, generation `1`.
- Retained observations: `20261013-worker-provider-binding.json`,
  `20261013-startup-live-import.json`, `20261013-operation-idempotency.json`.

## Chart change

`operation-worker` could not read any provider token: a Secret mounted under `fsGroup`
resolves to mode `0440` on the host, and `iam.FileTokenSource` rejects group-readable
bearer tokens. The chart now stages the declared `serviceTokenFiles` through an
owner-only init container into a 64 KiB in-memory volume mounted at
`/var/run/hnb-provider-tokens`, and refuses to render when `runtimeProviders` is
configured without a token list. The staged copy is pinned to the same digest rules
as every other image.

## Execution evidence

- The worker's `RUNTIME_PROVIDERS` pinned `providerVersion=startup-eval-20261013` and
  `providerDigest` equal to the digest the platform resolved for the registered
  manifest; the applied manifest was checked against the live API before rollout.
- `stage-provider-tokens` completed, the worker started, and the provider accepted the
  staged service token; an anonymous or file-permission failure would have been
  reported instead.
- The provider decrypted the registered kubeconfig, connected to the API server and
  failed only because the earlier evaluation credential had expired. After a fresh
  credential was registered through `/api/v1/secrets:register`, the same chain
  succeeded and the step output recorded `action=import`, `managed=true`, the target
  identifier and `observationKind=Agent`.

## Idempotency and failure semantics

| Check | Result |
| --- | --- |
| Same idempotency key, same body | HTTP 202, `replayed: true`, same operation, Operation count unchanged, one intent row |
| Same idempotency key, different body | HTTP 409 `IDEMPOTENCY_CONFLICT`, no new Operation |
| New idempotency key | New Operation executed once; observer generation stayed `1` |

An earlier replay attempt mistakenly sent the internally scoped key
(`tenant:kind:action:key`); the platform correctly treated it as a separate request and
created operation `659bffac`, which is still `queued`. That row is an artifact of the
test script, retained rather than deleted.

## Remaining blockers

1. **Service-credential rotation is not implemented.** `iam.MaxAccessTokenTTL` caps
   every access token at 60 seconds, and the staged copy only refreshes when the pod
   restarts. The successful import required submitting within 8 seconds of minting the
   token. A production worker needs an issuance and rotation path; none exists.
2. The imported target remains `REGISTERING` with an `agent` observation source. No
   Agent has authenticated, so the managed read model is not established.
3. Provider qualification is limited to the `KubernetesTarget`/`import` cell with a
   four-hour window and must not be read as platform or HA support.

## Failures retained on record

Operations `56889fcd` (no provider registered) and `e0dd3813` (provider registered but
the target credential had expired) remain stored with their reasons, together with
the earlier admission-negative and conformance runs.

## Newly found durability defect: orphaned steps never re-drive

Operation `659bffac` exposed a real gap. Its step-requested message was published at
`15:48:23.46`, one second before the worker pod that would consume it was created at
`15:48:24` during a token-rotation restart. Nine minutes later the step is still
`pending` with `retry=0/3`, no lease and no start time, and the Operation remains
`queued` at `0/1`, while the worker is ready with zero restarts.

The consumer is configured with `AckWait` 60s and `MaxDeliver` 4, and
`pkg/core.ProviderReconciler` reconciles provider bindings only. No component
re-drives an orphaned pending step, so a lost step message leaves the Operation stuck
forever with no timeout and no alert.

This contradicts any claim that operation execution self-heals after a worker restart.
Evidence: `20261013-orphaned-step-defect.json`. A fix must re-drive the step without
risking duplicate execution; that design decision is not taken here.