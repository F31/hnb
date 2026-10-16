# Provider conformance registration and first real Operation — 2026-10-13

## Result and scope

**Partial.** The provider qualification path is implemented and exercised on the real
host, a signed `KubernetesTarget`/`import` report was registered through the owning
plane, and a real Operation was created by the formal API chain. The Operation did
**not** succeed: `operation-worker` had no provider route bound, so the step failed
with a recorded reason. This is not a completed runtime import, not a Minimal or HA
certification, and not evidence that provider execution works through the platform.

## Run identity

- Run IDs: `startup-conformance-admission-20261013`,
  `isolated-import-conformance-70af0293-6915-4487-b34e-db94c1e277cf`,
  `startup-runtime-operation-20261013`.
- Base commit: `c23f6e4128e92d7a9457807dce5b10e0b0704e73` plus uncommitted working-tree changes.
- Environment: `106.75.34.133`, Ubuntu 24.04, 2 vCPU/3914 MiB, single-node K3s.
- Conformance run: `2026-10-13T15:05:12Z` – `2026-10-13T15:05:27Z`, all 7 assertions passed.
- Registration: `2026-10-13T15:21:38Z`. Operation created `2026-10-13T15:21:55Z`.
- Retained observations: `20261013-conformance-admission-negative.json`,
  `20261013-import-conformance-isolated.json`,
  `20261013-import-conformance-registration.json`, and
  `20261013-startup-runtime-operation.json`.
- Superseded failures kept on record: `20261013-conformance-isolated-auth-fixture-failed.json`,
  `20261013-conformance-isolated-replay-failed.json`.

## What was implemented

1. **Signed, action-scoped conformance reports.** `pkg/core.ConformanceReport`
   (`provider-conformance/v1`) binds run/source/environment identity, provider version,
   OCI artifact digest, base-manifest identity digest, matrix version, target kind,
   action, validity window, and per-category content-addressed evidence. Verification
   requires an Ed25519 signature from a configured trusted signer, all assertions
   passed, unique names, and complete contract/functional/security/fault/idempotency/
   fencing coverage. Only `KubernetesTarget`/`import` is accepted.
2. **Owning-plane endpoint** `POST /v1/providers/{id}/conformance` with provider
   `execute` permission, separate public-key trust mounted from a Secret, and
   fail-closed behaviour when trust is not configured.
3. **Atomic registration.** The report is re-checked against the current manifest
   under a row lock in one transaction; the signed report and the exact matrix-cell
   evidence are stored together, so a concurrent manifest replacement causes the
   registration to fail instead of qualifying stale material.
4. **Observer replay idempotency fix.** `PGObserverRegistry` no longer increments the
   observation generation when the same registration repeats.

## Defects found and fixed on the real host

| Defect | Evidence | Fix |
| --- | --- | --- |
| `SaveManifest` used `ON CONFLICT (provider_id)` but the shipped table has only an `id` primary key and a non-unique `provider_id` index | manifest POST returned 500 on the real database | row-locked upsert that updates exactly one row, inserts when absent, and refuses duplicates |
| nil Go slices produced SQL `NULL` for `capabilities`/`actions`, which are `NOT NULL` | first corrected run failed with `null value ... violates not-null constraint` | send an empty array instead of NULL |
| `GetManifest` returned a raw `sql.ErrNoRows`, so a missing provider surfaced as 500 instead of 404 | GET returned 500 for an unregistered provider | map to `ErrNotFound` |
| exact replay of an import incremented the observer generation from 1 to 2 | isolated run assertion `idempotency` failed | make observer registration a no-op when the source is unchanged |
| evaluation script re-tagged an **old** image to a **new** digest name, so a recorded digest did not describe the running content | containerd digest mismatch caught during deployment | added `verify-import-digest.sh`; every applied deployment now verifies the digest reference before rollout |

## Real-host assertions

- Negative admission: anonymous 401; untrusted signer, tampered report, failed
  assertion, and expired window all rejected; `operations` and `provider_manifests`
  remained empty during the negative run.
- Isolated conformance against the deployed provider image: anonymous execution 401,
  unsupported protocol 400, missing credential 403, real TLS discovery and observer
  registration succeeded, exact replay preserved generation 1, older fencing
  generation rejected with `FENCED`, cross-tenant credential 403.
- Registration accepted for `KubernetesTarget`/`import` only, bound to provider
  version `startup-eval-20261013` and the exact artifact and manifest digests, with a
  four-hour evaluation window.
- Real Operation: `POST /api/v1/runtime-intents` returned 202 with an intent, plan and
  operation identifier; the worker consumed the step and persisted a terminal failure
  plus a failed-message outbox event.

## Remaining blockers

1. `operation-worker` still runs with `RUNTIME_PROVIDERS={}`; the provider endpoint,
   pinned version/digest and audience are not bound.
2. Even once bound, the current chart cannot deliver a usable service token: a Secret
   file mounted under `fsGroup` resolves to mode `0440`, while
   `iam.FileTokenSource` requires owner-only access. This was measured on the host,
   not inferred.
3. Service tokens are capped at 60 seconds by `MaxAccessTokenTTL`, and no rotation
   source is configured, so a statically mounted token cannot serve a running worker.
4. A qualified provider remains usable only for its exact registered cell; no other
   action is certified.

The provider qualification recorded here expires four hours after registration and
must not be treated as ongoing support.