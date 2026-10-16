# Deployment profile planning

These overlays define component selection and starting requests. They contain no
image identities or credentials: provide release-locked image digests and existing
Secret bindings separately. No overlay certifies a deployment.

Merge in this order (later files win):

| Profile | Overlay sequence | Backings | Minimum independent control-plane domains |
|---|---|---|---|
| Startup (initial composition, not a certified profile) | `startup.yaml`, selected `capability-*.yaml` | bundled single PostgreSQL + file JetStream | 1 |
| Minimal | `minimal.yaml` | bundled single PostgreSQL + file JetStream | 1 (single point of failure) |
| LiteHA | `minimal.yaml`, `lite-ha.yaml` | in-cluster CloudNativePG PostgreSQL + 3-node JetStream (external optional) | 3 |
| StandardHA | previous sequence, `standard-ha.yaml` | in-cluster CloudNativePG PostgreSQL + 3-node JetStream (external optional) | 3 |
| Enterprise | previous sequence, `enterprise.yaml` | in-cluster CloudNativePG PostgreSQL + 3-node JetStream (external optional) | 3 |

HA overlays render **in-cluster** backings on the control nodes: a CloudNativePG
`Cluster` (3 instances, per-instance PVC, host anti-affinity) and a 3-node
JetStream cluster (headless routes, per-node PVC). The chart gate is structural
only: it requires `global.backings=in-cluster-ha` (or `external` with both
bundled subcharts disabled) and at least 3 schedulable control-plane domains
(`global.controlPlaneDomains`), else it fails with `HA_CONTROL_NODES_INSUFFICIENT`
or `EXTERNAL_BACKING_MISCONFIGURED`. Rendering an HA overlay is **not**
certification: an HA profile stays uncertified until its own Kubernetes,
PostgreSQL failover/recovery, JetStream node/leader failure, ingress and
persistence drills exist, and install is still refused by the capability
evidence gate.

Minimal includes the T0/T1 list fixed in task 1.5, including alert-manager,
cluster-agent, network-registry and cilium-provider. Independent network/Edge/
Karmada/GSLB Packs stay off. A larger profile does not automatically enable Packs.

Startup retains Core, Portal and Market and loads independent runtime, network,
Agent/RBAC, alert and plugin-execution components only when their capability
overlay is selected. See [lightweight startup](../../../../docs/deployment/lightweight-startup.md)
for component lists, bindings, storage limits and capacity commands. Overlay
selection is explicit deployment-time loading, not an automatic plugin installer.
To restore the complete component set, apply `minimal.yaml` last. Preserve existing
PVC sizes when changing compositions; Startup's smaller defaults are fresh-install only.

For fresh-install small-data evaluation, append `startup-small-disk.yaml` after
`startup.yaml`: PG 2Gi + JetStream 1Gi. With explicitly declared `reserveDiskGi: 6`,
startup scope and external cache/backup, disk planning is 10Gi. Default system
reserve instead requires 14Gi; local cache/backup with the 6Gi reserve requires
35Gi. The system/runtime/required local image footprint must be measured against
that 6Gi budget. This is not certification of running the platform on a 10Gi disk.

## Requests and disk budgets

The capacity command uses **actual rendered container requests**, including
sidecars and the maximum of regular-container sums and sequential init-container
requests. Replicas multiply requests. PVCs and StatefulSet claim templates supply
persistent storage demand; a referenced PVC absent from the input is unverified.
Missing requests or unsupported workload/quantity forms are errors, not zeroes.

Starting reservations, explicitly separate in the report:

- Each declared node: 1 CPU, 2 GiB memory and 20 GiB system disk.
- Each node: 20 GiB download/image cache (estimate; release size may require more).
- Backup: one full copy of rendered persistent storage, not an RPO/RTO guarantee.
- 30% extra on workload requests and persistent/cache/backup disk.
- Each Pod must fit at least one declared node after CPU/memory system reserve.

Minimal renders 19 workloads including PG/NATS: **1725m CPU, 2624 MiB memory,
30 GiB PVCs**. With one node and the default delivery budgets: **3243m CPU,
5460 MiB memory, 124 GiB disk**. Values are Helm-merged parent/subchart defaults
and operator overrides; artifact/credential bindings should not silently replace
resource policy. This is an estimate, not a load test or production recommendation.

`--scope startup` selects 10% headroom and 500m/1024Mi/10Gi per-node reserve.
`--cache-source external --backup-source external` removes those budgets from
node disk planning, but does not configure external storage. Startup's 10 workloads
request **1075m / 1792Mi / 15Gi** and require **1683m / 2996Mi / 27Gi** under
those options. Full Minimal still requires **2398m / 3911Mi / 43Gi** under the
same options. Recompute after adding capabilities or overriding PVC sizes.

External HA backing planning budgets (must be assessed on their own hosts):
at least three PostgreSQL instances, each 1 CPU/2 GiB/100 GiB SSD, and three
JetStream servers, each 500m/1 GiB/50 GiB persistent storage, with independent
failure domains. Account separately for replication, snapshots, full backups,
cache, retained messages, actual traffic and growth. These are untested starting
budgets; no external capacity or availability is inferred from HNB replicas.

## Capacity report

```bash
helm template hnb deploy/charts/hnb \
  -f release-and-secret-bindings.yaml \
  -f deploy/charts/hnb/overlays/minimal.yaml > rendered.yaml
hnbctl capacity --profile Minimal --manifest rendered.yaml --nodes capacity-nodes.yaml
```

Node input is one YAML list, using total capacity **reserved for this deployment**:

```yaml
- name: cp-1
  failureDomain: zone-a
  controlPlane: true
  cpuMilli: 16000
  memoryMi: 32768
  diskGi: 200
```

This input is an operator declaration, not a live node observation. The report
includes a manifest SHA-256 and outputs only workload identity/resource numbers,
never environment values or Secret contents. Exit 0 means the declared planning
assessment passed; `installAuthorized` always remains false. Insufficient
capacity/HA/optional Pack checks return nonzero with a JSON report. It does not
prove scheduling constraints, StorageClass behavior, PVC locality, ingress,
release compatibility, credentials, provider readiness or full delivery.
