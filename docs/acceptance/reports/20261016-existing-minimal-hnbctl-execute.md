# Acceptance: hnbctl applier, execute gating and uninstall on live K3s (2026-10-16)

- run_id: `existing-minimal-hnbctl-execute-20261016`
- commit: `c23f6e4128e92d7a9457807dce5b10e0b0704e73` (working tree carries the installer changes under review)
- reports: `docs/acceptance/reports/20261016-existing-minimal-hnbctl-execute.md`
- verifier: none (interactive hnbctl-driven run; commands and outputs recorded below)

## Environment

- Host `43.142.101.136`; Ubuntu 26.04 LTS, kernel `7.0.0-30-generic`, amd64.
- Existing single-node K3s `v1.36.5+k3s1`; `hnb-system` at 19/19 workloads (unchanged before and after).
- `hnbctl` (linux/amd64): `sha256:422720ff94a533b8db7f03f4c0167018aec1ce6562fe671fec1f7a6fb4d6db32`.
- Helm `v3.16.4` installed only to render the fixture chart.
- Config `hnb.yaml` (existing, Minimal), `release.lock.yaml` / `release.exec.lock.yaml`, kubeconfig copy at
  `/home/ubuntu/e2e/`; runs root `/home/ubuntu/e2e/runs`, `/home/ubuntu/e2e/runs-exec`.

## Scope

The `hnbctl install --execute` existing-cluster applier (render → `kubectl apply` → rollout →
phase/run finalisation) driven against the live cluster, plus the `uninstall` decision matrix.
This is a driver-level verification with a fixture SecurityObject (a harmless ConfigMap), not a
re-materialisation of the 19-workload platform (that is the separate existing-Minimal E2E report).

## 1. Certification and planning

- `hnbctl matrix` reports `profile Minimal: certified`, `route existing certified`, bootstrap/HA
  blocked.
- `hnbctl install -f hnb.yaml --plan` (exit 0): reason `PLANNING_ONLY` + `NO_NODE_INIT_ACTION`;
  run `5b88c013669545032676b42dcee0a64a` with `deployment.yaml`/`release.lock.yaml` snapshots (0600),
  `executionAuthorized=false`, state `blocked`.

## 2. install --execute on the live cluster (applier)

Command (fixture chart renders one ConfigMap; rollout target is an already-ready Deployment):

```
hnbctl install -f hnb.yaml --lock release.exec.lock.yaml --runs-dir runs-exec \
  --execute --chart-package hnb.tgz --bindings bindings.yaml \
  --workload Deployment/apiserver --format json
```

- exit 0, `executionAuthorized=true`.
- `run-state.json`: `status=succeeded`; phases `planning`, `platform-release`, `platform-verified` all
  `succeeded` (runs `ce423ec625461baf181f3b5c0d71f5d9`, `c84a44e64832784465043f7e4c79c2cb`;
  a re-run is idempotent).
- Side effect observed: `kubectl get cm hnb-applier-e2e -n hnb-system` → `{"verified":"true"}`,
  proving a real `kubectl apply` reached the cluster. Cleaned up afterwards.
- `hnbctl logs --runs-dir runs-exec --tail 20` shows the ordered transition journal
  (`run.created`, `phase.changed`, `run.changed`).
- `hnbctl history --runs-dir runs-exec --state succeeded` lists the succeeded runs;
  `hnbctl status --run-id <id>` reports the platform layer (partial delivery, because identity/
  infrastructure/migration phases are not part of this fixture).

## 3. Fail-closed rollout

- `--workload Deployment/does-not-exist` → exit 4 (`PLATFORM_NOT_VERIFIED`),
  `platform-verified=failed` (`PLATFORM_ROLLOUT_FAILED`), run finalized `blocked` (run
  `6f60660c2975c1e3e618d13e8e596d0f`). No success is fabricated when rollout does not converge.
- `--execute` without `--chart-package`/`--bindings` → exit 2, names the missing applier inputs.

## 4. uninstall decision matrix (run `5b88c013669545032676b42dcee0a64a`)

| invocation | result |
| --- | --- |
| `uninstall --installation <id> --plan` | exit 0; `authorized=true`; deletes RBAC + release + serviceaccounts; **retains** namespace and all PVC secrets/StorageClass |
| `uninstall ... --delete-data --plan` | exit 0; `authorized=false`, `dataLossAcknowledged=false` (deletion listed but not acknowledged) |
| `uninstall ... --delete-data` (no TTY) | exit 3 (`PREFLIGHT_BLOCKED`); "data removal not confirmed; no changes made" |
| `uninstall ... --delete-data --yes --format json` | exit 4 (`PLATFORM_NOT_VERIFIED`); `authorized=true`, `dataLossAcknowledged=true`; PVC in delete set, **namespace retained**; execution deferred to the server-authoritative removal contract |
| `uninstall -f <changed> --run-dir <dir>` | exit 2; "does not match the recorded install run" |
| `uninstall --run-dir <tampered dir>` | exit 2; "snapshot identity mismatch" |

`uninstall` never executes deletion locally: a fully authorized destructive plan still returns
`PLATFORM_NOT_VERIFIED` and defers to the platform removal contract.

## Notes / not covered

- A full platform `install --execute` (19 workloads) was not executed: no release-engineering
  artifact (packaged Chart + ReleaseLock with real image manifest digests) exists on the host, and
  applying a real chart could mutate the already-certified deployment. The applier path is therefore
  verified here with a harmless fixture resource, and in-process by
  `TestShortcutExecuteAppliesCertifiedExistingRoute` (fake `helm`/`kubectl`).
- `status` reports "delivery is partial" for a fixture run without identity/infrastructure/migration
  phases; the run itself is `succeeded`. This layered view is intentional and fail-closed.
