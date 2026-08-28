## Context

HNB currently exposes workload storage through a container plugin that directly proxies Kubernetes StorageClass/PV/PVC reads and writes, while the resource plugin has only a storage placeholder. The target must support heterogeneous cloud disks, NAS/NFS, SAN, local volumes, external Ceph and other CSI-compatible systems without conflating marketplace packages, observed drivers, backend systems, service offerings or cluster-local Kubernetes objects. App Market `ArtifactStorageProfile` remains a separate bounded context.

Stakeholders are platform administrators managing supply, tenant/project administrators selecting approved offerings, application operators consuming StorageClasses/PVCs, Provider authors, security/audit operators and SREs.

## Goals / Non-Goals

**Goals:**

- Establish a versioned, tenant-safe workload-storage domain and read model.
- Discover structured CSI and Kubernetes storage facts with source and freshness.
- Present Resource/Storage as the supply view and Container/Storage as the consumption view.
- Map business offerings to cluster-local StorageClasses explicitly.
- Route every target mutation through immutable ExecutionPlan and Operation.
- Permit provider-specific configuration without weakening Secret, schema or authorization boundaries.

**Non-Goals:**

- CSI is not treated as a backend capacity, performance or installation API.
- Object/bucket storage is not represented as an ordinary PV/PVC offering.
- Phase 1 does not install drivers, create StorageClasses, reclaim data or invent unavailable capacity.
- Artifact storage ownership and database tables are unchanged.

## Architecture

```text
Provider/Extension Catalog              RuntimeTarget Observer
┌────────────────────────┐             ┌──────────────────────────┐
│ StorageDriverPackage   │             │ SC/CSI/VA/Snapshot facts│
└───────────┬────────────┘             └────────────┬─────────────┘
            │ install Operation                      │ observation
            ▼                                        ▼
┌────────────────────────┐              ┌──────────────────────────┐
│DriverInstallation      │─────────────▶│ Storage Read Model       │
└───────────┬────────────┘              └────────────┬─────────────┘
            │ connects                               │
            ▼                                        │
┌────────────────────────┐                           │
│StorageBackend          │                           │
└───────────┬────────────┘                           │
            │ exposes                                │
            ▼                                        │
┌────────────────────────┐                           │
│WorkloadStorageOffering │                           │
└───────────┬────────────┘                           │
            │ binds                                  │
            ▼                                        ▼
┌────────────────────────┐              ┌──────────────────────────┐
│StorageClassBinding     │─────────────▶│ Cluster StorageClass     │
└────────────────────────┘              └──────────────────────────┘
        Resource Portal                         Container Portal
```

No plane shares another plane's database. Portal calls versioned apiserver contracts. Observers publish bounded observations to Platform projection. Provider writes are executed by Operation workers. NATS transports commands/events but is not a storage fact source or data plane.

This change adds no deployable database, broker, cache, object store or transfer service. Storage desired state and projections use the existing PostgreSQL deployment; governed writes use the existing PostgreSQL Operation Store and Transactional Outbox; the existing relay and NATS JetStream carry references and bounded commands/events. Storage payload bytes remain in Kubernetes volumes or provider-owned storage and never traverse or persist in Platform API, Operation, Outbox or JetStream.

## Data Model

| Entity | Identity/Scope | Authoritative fields |
|---|---|---|
| StorageDriverPackage | provider catalog ID/version | package kind, provisioners, compatibility, capabilities, conformance evidence |
| StorageDriverInstallation | tenant, target, package version | desired version, Operation reference, lifecycle and observed health |
| StorageBackend | tenant, provider type, backend ID | display metadata, SecretReference, connection/health state, extension attributes |
| WorkloadStorageOffering | tenant/global offering ID/version | service mode, access modes, topology, expansion/snapshot/clone facets, protection class |
| StorageClassBinding | offering, target, StorageClass UID | class name/UID/resourceVersion, sync status, topology/default flags |
| StorageObservation | tenant, target, kind, Kubernetes UID | source, generation/sequence, observedAt, freshness, normalized fields, raw reference |

Free-form provider attributes are display-only unless covered by a versioned provider schema. Credentials are always tenant-owned SecretReference. Kubernetes UID/resourceVersion are retained for identity and concurrency.

## API And Event Contracts

- `GET /api/v1/storage/overview`
- `GET /api/v1/storage/backends`
- `GET /api/v1/storage/offerings`
- `GET /api/v1/storage/driver-installations`
- `GET /api/v1/storage/targets/{targetId}/inventory`
- `GET /api/v1/storage/offerings/{id}/bindings`
- Future mutations accept intents and return Operation references; they never return synchronous target success.
- `RuntimeTargetObservation` gains a versioned `storageInventory` section supporting Full/Delta semantics and the same generation/sequence fencing as nodes.
- Domain events contain IDs, versions, conditions and Operation references, not credentials, volume payloads or large manifests.
- Every storage API request and response uses bounded JSON metadata. Binary, multipart, upload, download and storage data-plane proxy routes are outside this API boundary.

## State Machines

```text
DriverInstallation: Pending -> Installing -> Ready -> Upgrading -> Ready
                                 |             |          |
                                 v             v          v
                               Failed       Degraded    Failed
Ready/Degraded -> Uninstalling -> Removed

StorageClassBinding: Discovered -> Imported -> Active -> Drifted
                                      |          |
                                      v          v
                                   Rejected   Retired

Observation freshness: Fresh -> Stale -> Fresh
                             \-> Unknown
```

Backend health and observation freshness remain separate from lifecycle state. Clearing a PV claim reference is not a reclaim state transition.

## Decisions

1. **Five entities instead of a three-layer polymorphic record.** Package, installation, backend, offering and binding have different identity, ownership and lifecycle. Alternative generic JSON `resource` rows were rejected because they erase authorization and validation boundaries.
2. **Observed inventory is separate from desired state.** Kubernetes facts come from ordered RuntimeTarget observations; desired driver/offering changes come from Operations. Alternative request-time proxy fan-out was rejected for latency, availability and tenant safety.
3. **Offering rather than Capability/Class.** `WorkloadStorageOffering` represents business service semantics; `StorageClassBinding` carries target-specific implementation. Alternative one-to-one StorageClass mapping cannot support multi-cluster delivery.
4. **Provider schemas are constrained extensions.** Common facets remain typed; provider forms can contribute trusted components and versioned schemas using SecretReference. Arbitrary remote JavaScript, URLs and Secret values are rejected.
5. **Dedicated storage APIs and IAM.** Read permissions and operation intents are resource-specific. Generic proxy execution is not an authorization boundary.
6. **Read-only first.** Phase 1 proves discovery, freshness, identity and navigation before any mutation. This reduces migration and data-loss risk.

## Security And Isolation

- Every row and API query is tenant-bound; target and namespace scope are checked server-side.
- Observer identity is bound to tenant/target and storage inventory generation.
- Observer and executor ServiceAccounts are separate; agent paths/methods are allowlisted.
- Provider credentials use SecretReference and are resolved only in the executing Step.
- Supply-chain metadata includes package digest/signature, compatibility and conformance expiry.
- Mutations record actor, tenant, target, Kubernetes UID/resourceVersion, plan, Operation and correlation ID.

## Performance, Capacity, DR, And Observability

- Full discovery is paginated and bounded per target; Delta observations update explicit resources only.
- Read APIs use PostgreSQL projections and do not fan out to clusters.
- Capacity values include unit, source, observedAt and status (`Known`, `Elastic`, `Unknown`, `NotReported`).
- Metrics avoid unbounded PVC/PV/volume-handle labels; detailed facts remain in the read model.
- Monitor observation delay/failures, stale targets, driver readiness, binding drift, Operation latency/failure and alert delivery.
- PostgreSQL backup/restore and existing outbox recovery cover desired/read models; after restore, observers reconcile current facts. NATS replay alone never reconstructs authority.

## Compatibility And Conformance

| Package/System | Block/File | Expansion | Snapshot | Capacity source | Initial tier |
|---|---|---|---|---|---|
| Generic CSI | declared/observed | discovered | conditional on snapshot stack | CSIStorageCapacity or unknown | T1 read-only |
| NFS/static PV | file | provider-specific | provider-specific | exporter/provider | T1 read-only |
| Rook/Ceph | block/file | provider-specific | CSI + Ceph evidence | Ceph exporter/provider | T2 |
| Cloud disk | block | provider API/CSI | provider API/CSI | elastic/quota | T2 |
| Local PV | block/file | usually unsupported | provider-specific | node inventory | T2 high-risk |

Conformance covers package identity, supported Kubernetes versions, discovery truth, Secret handling, idempotent Operations, fencing, rollback metadata and capability claims. UI labels never constitute conformance evidence.

## Failure Modes

- Observer unavailable -> retain last facts, mark stale, prohibit unsafe writes.
- Driver metadata exists but workloads unhealthy -> installation is Degraded, not Ready.
- StorageClass references unknown provisioner -> inventory remains visible with missing-driver condition.
- Provider capacity unavailable -> show Unknown/Elastic, never zero.
- Binding drift/resourceVersion conflict -> mark Drifted and require reconcile Operation.
- Operation timeout/cancel -> preserve Operation truth and reconcile via later observation.
- Backend credential failure -> expose sanitized condition; never log Secret content.

## Risks / Trade-offs

- [Broader model increases initial work] -> phase delivery and start with read-only inventory.
- [Provider metrics are inconsistent] -> require source/freshness and allow Unknown rather than fake normalization.
- [Schema Engine cannot yet render complex secure forms] -> use trusted plugin components until form contracts mature.
- [Route duplication confuses users] -> keep explicit supply/consumption labels and compatibility redirects.
- [Legacy proxy remains exploitable] -> introduce allowlists and remove dangerous generic actions before enabling supply mutations.

## Migration Plan

1. Add contracts and read-only structured discovery without changing existing routes.
2. Populate storage read models and expose resource-side inventory behind a capability flag.
3. Import existing StorageClasses into offerings/bindings without mutating cluster objects.
4. Add dedicated IAM/API and split observer/executor RBAC.
5. Enable governed intents for installation, offering binding and PVC expansion.
6. Add provider-specific reclaim, snapshot and observability only after conformance.
7. Redirect legacy routes after telemetry shows no incompatible clients.

Rollback disables the capability/navigation and dedicated APIs while preserving observations and existing Kubernetes resources. Database migrations use additive tables/indexes and reversible navigation changes; rollback does not delete target resources.

## Release And Lifecycle Impact

- **Install:** apply the additive storage migrations to the existing PostgreSQL database and deploy the updated Platform API, observer/projector, Operation worker and Portal artifacts. Do not provision a new database, message broker, object store, transfer proxy or middleware service.
- **Upgrade:** back up PostgreSQL before migrations, apply migrations in order and roll existing components through their normal upgrade path. Existing Operation/Outbox/JetStream compatibility and rollback procedures apply; there is no storage-specific middleware upgrade.
- **Backup:** include storage desired-state tables, read models, Operations and unpublished Outbox rows in the existing PostgreSQL backup. Kubernetes volume contents and provider backend data are not Platform API data and require provider/workload backup policies. JetStream is transport, not backup authority.
- **Restore:** restore PostgreSQL using the existing platform procedure, restart the existing Outbox relay/workers, and allow target observers to reconcile projected inventory. Restoring metadata does not restore, copy or overwrite storage payloads.
- **Uninstall/rollback:** disable storage routes/navigation and stop storage reconcilers before reversing storage metadata migrations when retention policy permits. Never delete PV/PVC contents, backend data or provider storage as a side effect. Driver uninstall remains a separately authorized Operation, not a platform feature-uninstall side effect.

The release evidence and concrete repository references for these N/A assessments are recorded in `evidence/release-notes-tasks-8.1-8.2.md`.

## Open Questions

- Which tenant/global ownership policy applies to shared offerings?
- Which Provider packages qualify for T1 beyond generic discovery?
- What retention period is required for storage observations and capacity history?
- Which canonical alert implementation will own storage alert rules before Phase 5?
