# Acceptance: profile upgrade rehearsal and scale guardrails (2026-10-16)

- run_id: `profile-upgrade-rehearsal-20261016`
- commit: `c23f6e4128e92d7a9457807dce5b10e0b0704e73` (working tree carries the change under review)
- report: `docs/acceptance/reports/20261016-profile-upgrade-rehearsal.md`
- hnbctl (linux/amd64): `sha256:41fe0da0c2301a52089a05a3192d0379e4ad64081947fee267b2de41e73630ad`

## Environment

- Host `43.142.101.136`; Ubuntu 26.04 LTS, kernel `7.0.0-30-generic`, amd64.
- Existing single-node K3s `v1.36.5+k3s1`; `hnb-system` at 19/19 workloads before and after.
- Fixture chart (`hnb.tgz`, sha256 `d89861c3…`) renders one harmless ConfigMap; rollout target
  `Deployment/apiserver` is already ready.

## Scope

The gated, resumable **profile upgrade executor** (run kind `upgrade`) driven by `hnbctl upgrade
--execute` against the live cluster, plus the `scale` planner and guardrails. The only currently
certified profile is Minimal, so the rehearsal is a Minimal -> Minimal transition declared in a
release-train lock; it exercises the full phase pipeline and gate without changing the platform.

## Upgrade plan and execution

- `hnbctl upgrade -f hnb.yaml --lock release.upgrade.lock.yaml --to Minimal` (dry-run, exit 0):
  authorized plan with `release` + `verify` steps; no cluster contact.
- `hnbctl upgrade … --to Minimal --execute --chart-package hnb.tgz --bindings bindings.yaml
  --workload Deployment/apiserver --runs-dir runs-upgrade` (exit 0):
  - run `af5985b31af82b22f94c875c7e85dd15`, `kind=upgrade`, `status=succeeded`,
  - phases `preflight`, `backup`, `platform-release`, `platform-verified` all `succeeded`,
  - a real `kubectl apply` created `ConfigMap/hnb-applier-e2e` (cleaned up afterwards).
- Fail-closed guarantees verified by unit tests: blocked plan refused before any phase; backup
  prerequisite fails closed before `database-migration`; irreversible migration refused without
  `--allow-irreversible`; failed one-shot phase (`database-migration`) refuses re-run; resume skips
  already-succeeded phases.

## Rollback

- `PlanRollback` + `RunUpgradeRollback`: a reversible plan yields an authorized rollback with
  reversed steps; any irreversible migration or managed-claim growth refuses the rollback.
  Verified by unit tests (no live data is mutated by the rehearsal).

## Scale planner and guardrails

- `hnbctl scale -f hnb.yaml --component apiserver --replicas 3` → authorized plan (1 -> 3).
- `hnbctl scale -f hnb.yaml --storage postgresql=1` → `STORAGE_SHRINK_REFUSED: postgresql 1Gi < 2Gi`.
- Unit tests cover below-quorum refusal, single-domain co-location cap, in-flight-upgrade block and
  unknown component/storage rejection.
- `hnbctl scale --execute` is wired (run kind `scale`, phases `preflight → infra-delta →
  platform-verified`) and was executed against the live cluster:
  - `scale -f hnb.yaml --component apiserver --replicas 3 --execute` → `3/3` ready, run
    `8a4d075b5666563c0aaa87e744c787d1` `succeeded`;
  - `scale -f hnb.yaml --component apiserver --replicas 1 --current apiserver=3 --execute` → `1/1`
    ready, run `0302d26bb43e575ce3790dae870034e5` `succeeded`;
  - the platform remained at 19/19 workloads; `--current` supplies the observed baseline so a scale
    is planned relative to the live replica count, not the profile default.
- Storage expansion was executed against the live cluster: `scale -f hnb.yaml --storage nats=2
  --execute` grew `pvc/nats-data` from `1Gi` to `2Gi` (run `0e3b39c3d468db6a226baf8b148d931a`,
  `succeeded`); `--current-storage nats=2 --storage nats=1` is refused with `STORAGE_SHRINK_REFUSED`.
  This growth is one-way (PVCs cannot shrink) and is the intended production behaviour.
- Vertical scaling was executed: `scale -f hnb.yaml --memory portal=96Mi --execute` raised
  `deployment/portal` memory from `64Mi` to `96Mi` (run `f8ffc1aa…`, `succeeded`); reverting with
  `--memory portal=64Mi --current-memory portal=96Mi --execute` restored `64Mi` (run `c56007f0…`),
  with the missing CPU request defaulted from the profile shape. The platform remained at 19/19.

## Not covered

- A higher-profile upgrade (Lite/Standard/Pro) cannot be executed: those profiles are not yet
  certified, so `PlanUpgrade` returns `TARGET_PROFILE_NOT_CERTIFIED`. The executor wiring is proven
  at the CLI level (`TestUpgradeExecuteRunsOnCertifiedProfile`) and the toolkit level.
- `scale --execute`, existing-route ownership-boundary enforcement for scaling (task 5.3), and
  PostgreSQL/JetStream data-plane failover drills remain open.
