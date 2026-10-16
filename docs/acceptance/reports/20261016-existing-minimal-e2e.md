# Acceptance: existing-cluster Minimal full-platform delivery (2026-10-16)

- run_id: `existing-minimal-e2e-20261016`
- commit: `c23f6e4128e92d7a9457807dce5b10e0b0704e73` (working tree also carries the installer
  changes under review; the registry `commit` field records this base)
- command: `sudo python3 hnb-e2e-verify.py --hnbctl /home/ubuntu/hnbctl-e2e --restart-workloads --output /home/ubuntu/hnb-e2e-result.json`
- exit_code: 0
- status: passed
- report artifact: `docs/acceptance/reports/20261016-existing-minimal-e2e.md`
- result artifact: `docs/acceptance/reports/20261016-existing-minimal-e2e-result.json`
- verifier: `docs/acceptance/reports/20261016-existing-minimal-e2e-verify.py`

## Environment

- Host `43.142.101.136`; Ubuntu 26.04 LTS, kernel `7.0.0-30-generic`, amd64, cgroup v2, AppArmor active.
- Existing single-node K3s `v1.36.5+k3s1`, containerd `2.3.4-k3s1.36`.
- Platform namespace `hnb-system`; TopoLVM StorageClass `hnb-topolvm` (Retain, WaitForFirstConsumer, allowVolumeExpansion).

## Scope

Existing-route Minimal **full platform delivery** and rolling recovery on the observed OS:

- 19 platform workloads ready, three-plane schema ledgers at 103, functional portal APIs and a
  connected agent tunnel, owned PVCs on the verified StorageClass.
- Rolling restart of PostgreSQL, NATS and all deployments preserves data identities.

Not covered: kubeadm bootstrap, Cilium CNI, offline delivery, HA, production credential
lifecycle, and 2C4G/15G qualification. The platform was materialized by the acceptance
harness; the `hnbctl install --execute` applier and migration pipelines are exercised by
unit tests with a fake cluster and synthetic certified evidence, not by this run.

## Artifact identity

- `hnbctl`: `sha256:7e13306dd0aa5fa7c97829942e6c29f923adcd178012cfcee150a3e52fe06e6b`
- verifier: `sha256:c7394ccd2dab078f4454598840c44aa2cbfaf0362606f42a48cc332e4a539b99`

## Assertions (all true)

- `observedOSKernelWithinScopedEvidence`
- `draftCreatedOwnerOnlyWithoutInstalling`
- `nineteenWorkloadsReady`
- `threePlaneMigrationLedgers103`
- `postgresql2GiNats1GiOnTopoLVM`
- `loginReadAPIsAgentAndAnonymousRejection`
- `nineteenWorkloadsReadyAfterRecovery`
- `claimsUnchangedAcrossRecovery`
- `threePlaneMigrationLedgersPreserved`
- `allWorkloadPodsRecreated`

## Evidence

- Workloads: exactly 19 deployments/statefulsets with matching ready replicas before and after.
- Ledgers: `platform=103, market=103, artifact=103` before and after recovery.
- Storage: `postgresql-data=2Gi`, `nats-data=1Gi`, both `hnb-topolvm`.
- Functional: `login=200`, `/api/v1/workspaces=200`, `/api/v1/tenants=200`, `/api/v1/agents=200`,
  agent `connected`; anonymous `/api/v1/workspaces=401`.
- Recovery: all workload Pod UIDs change; PVC UIDs/volumes and ledgers are preserved.

## Certification decision

This run promotes, in `CurrentEvidence`:

- `Profiles[Minimal]`: `certified` (scope: platform delivery; the bootstrap **route** gate still
  blocks kubeadm bootstrap, so this is not a bootstrap or HA certification).
- `Routes[existing]`: `certified`.
- `OSAdaptations[ubuntu-26.04 / existing / k3s]`: `verified` -> `certified`.

`CertifiedOS` deliberately excludes `ubuntu-26.04`: that list is the bootstrap OS set, and this
existing-K3s evidence must never authorize a kubeadm bootstrap release. `Routes[bootstrap]` and
the HA/Enterprise profiles remain `blocked`.
