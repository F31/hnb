# Startup runtime capability preflight — 2026-10-13

## Result and scope

**Partial; the real ImportRuntimeTarget Operation execution remains blocked.**
The existing Startup environment now has 12 ready workloads, including the two
runtime providers. Secret registration succeeds through Portal/apiserver and
platform-api. Import submission reaches a 503 rejection, with no Operation created.
This is not a successful runtime import, worker execution, complete Minimal
installation, clean-host, reboot, capacity, offline or HA acceptance.

## Run identity

- Run ID: `ubuntu-startup-runtime-preflight-20261013`
- Base commit: `c23f6e4128e92d7a9457807dce5b10e0b0704e73` plus uncommitted working-tree changes.
- Environment: `106.75.34.133`, Ubuntu 24.04, 2 vCPU/3914 MiB, existing single-node K3s.
- Started: `2026-10-13T14:16:35.013734+00:00`.
- Finished: `2026-10-13T14:17:35.576163+00:00`.
- Verification command: direct SSH, `sudo -n python3 - < /tmp/opencode/verify-runtime-host.py`.
- Exit code: 0 for the **preflight assertions**, including the expected rejection;
  not an Operation success code.
- Retained observations: `20261013-startup-runtime-preflight.json`.
- Artifact identities: `20261013-startup-runtime-artifacts.json`.

## Changes and verification

1. Added `platform-api.masterKeySecret`, referencing Secret key `key` through
   `HNB_MASTER_KEY`. The lifecycle provider uses the same `hnb-eval-kms` Secret.
   The generated key has an owner-only backup on the evaluation host; it is not
   included in this evidence.
2. Built and imported linux/amd64 runtime-provider evaluation images. Rendered
   Startup followed by `capability-runtime.yaml`, with digest bindings. Applied
   the three selected Deployments/provider resources, then the corrected apiserver
   Deployment. Existing PostgreSQL/NATS/PVC manifests were not reapplied.
3. Fixed kubernetes-provider Chart startup wiring: explicit `ALLOWED_NAMESPACES`
   (`hnb-runtime-eval` for this evaluation), `MAX_REPLICAS`, `LISTEN_ADDRESS=:8080`,
   and `/healthz` readiness. The initial revision crashed because its required
   namespace environment was absent. Its initial rollout did not prove stable
   execution because it lacked a readiness probe.
4. Fixed the BFF Import semantic digest. Typed nil dependency slices boxed as
   interfaces were serialized as `null`, while platform-api omitted unrelated
   retained-volume fields. The BFF now includes those fields only for the two
   retained-volume intent kinds, matching platform-api. A canonical Import digest
   regression test passed with `go test -race ./internal/handler` in `cmd/apiserver`.
5. Final Chart/profile suite: 55 tests passed. `go test -race ./...` in
   `cmd/hnbctl` also passed; `git diff --check` was clean. Chart checks are
   rendering checks, not provider conformance evidence.

## Real-host assertions

| Check | Observed |
| --- | --- |
| Workloads | 12 ready before and after a 60-second observation window |
| Additional restarts in that window | 0 |
| Both provider `/healthz` | 200 |
| Both providers, anonymous POST `/v1/execute` | 401 |
| Portal login | 200 and an access token issued |
| Session/bootstrap, workspaces, tenants | HTTP 200 |
| Formal `/api/v1/secrets:register` | 201, `local-aes` reference version 1 |
| Database evaluation secret | One non-empty `AES-256-GCM` encrypted record |
| Formal `/api/v1/runtime-intents` | 503, BFF `UPSTREAM_UNAVAILABLE` |
| Operations / provider manifests | 0 / 0 |

The credential was a one-hour token for the existing default ServiceAccount,
with no new RBAC grants, in a kubeconfig pointing at the management cluster.
The kubeconfig and token are not retained in the report. This confirms the
registration/encryption path; provider decryption and execution are not proven.
HTTP 200 for session/bootstrap is recorded only as HTTP availability, not an
assertion about its business payload.

## Remaining prerequisites

- Register the provider through the owning-plane API.
- Generate and retain real action-specific evidence for the exact compatibility
  matrix cell, provider version and artifact. Lifecycle admission requires
  `production_ready`, an unexpired timestamp and matching evidence reference
  (`cmd/platform-api/internal/api/lifecycle_provider.go`).
- Establish an authenticated, authorized evidence-ingestion path. The current
  manifest POST creates conformance level `none`; `UpdateConformanceLevel` has
  no production caller in `cmd/`. The existing generic conformance CLI does not
  provide the lifecycle matrix-cell evidence needed by admission. This is a
  prerequisite gap, not permission to fabricate an eligible manifest.
- Bind the worker's exact provider manifest digest/version and provider-audience
  service token. The current worker still has `RUNTIME_PROVIDERS={}`.
- Verify queued/running/terminal state, provider execution, idempotency and
  failure semantics before performing Operation write-load capacity tests.

The raw observation is the BFF's generic 503, not a retained raw platform-api
`PROVIDER_ROUTE_NOT_FOUND` response. The empty registry and source resolver
establish the missing routing prerequisite; no lower-level error response is
claimed. No migration replay or direct business-table mutation was used.
