## 1. Contract And Safety Baseline

- [x] 1.1 Define versioned storage inventory, backend, offering, binding and condition schemas with generated Go/TypeScript contract tests. [STO-001, STO-002, STO-003]
- [x] 1.2 Define dedicated read-only storage OpenAPI endpoints and scoped IAM actions; document generic proxy deprecation. [STO-004, UX-013]
- [x] 1.3 Remove the generic PV claimRef recycle action and add regression tests proving retained data is not reported as sanitized. [STO-005]
- [x] 1.4 Add agent resource/method allowlist tests and separate observer versus executor RBAC manifests. [STO-004]

## 2. Structured Runtime Discovery

- [x] 2.1 Extend RuntimeTargetObservation with Full/Delta storageInventory and stable UID/resourceVersion identity. [RT-011]
- [x] 2.2 Discover StorageClass, CSIDriver, CSINode, CSIStorageCapacity and VolumeAttachment with paginated bounded requests. [RT-011]
- [x] 2.3 Detect optional snapshot APIs without treating missing CRDs as healthy empty inventories. [RT-011]
- [x] 2.4 Project ordered storage observations, tombstones, freshness and driver evidence into PostgreSQL read models. [RT-011, RT-012]
- [x] 2.5 Add duplicate, gap, stale generation, cross-tenant and missing-driver integration tests. [RT-011, RT-012]

## 3. Phase 1 Read-Only APIs And Portal

- [x] 3.1 Implement read-only overview, backend/system, offering, driver installation and target inventory endpoints from projections. [STO-001, STO-003]
- [x] 3.2 Replace the Resource/Storage placeholder with supply-side Overview, Storage Systems, Storage Services, Drivers/Connectors and Alerts navigation. [UX-012]
- [x] 3.3 Render source, observedAt, freshness and Known/Elastic/Unknown/NotReported capacity states using UI Kit components. [UX-013, STO-003]
- [x] 3.4 Link Offering bindings to filtered Container/Storage StorageClass inventory without duplicating ownership. [UX-012, STO-002]
- [x] 3.5 Add API unit tests, projection integration tests and Portal desktop/mobile E2E for read-only inventory and empty/error/stale states. [UX-012, UX-013]

## 4. Offerings And Governed Operations

- [x] 4.1 Add additive migrations and APIs for StorageBackend, WorkloadStorageOffering and StorageClassBinding with tenant scope and optimistic concurrency. [STO-001, STO-002]
- [x] 4.2 Implement import/reconcile intents that produce immutable ExecutionPlan and Operation references. [STO-004]
- [x] 4.3 Add Provider schema allowlisting, SecretReference validation and trusted component forms for backend-specific configuration. [STO-001, STO-004]
- [x] 4.4 Add binding drift detection and reconcile operations with idempotency and fencing tests. [STO-002, STO-004]

## 5. Driver Lifecycle And Conformance

- [x] 5.1 Extend package manifests with provisioners, compatibility, capability claims, digest/signature and conformance evidence. [RT-012]
- [x] 5.2 Implement install/upgrade/uninstall intents through Provider Steps and Operation, including rollback metadata. [STO-004]
- [x] 5.3 Add conformance matrix tests for generic CSI, NFS/static PV, Ceph, cloud disk and local PV tiers. [RT-012]

## 6. Capacity, Alerts And Safe Reclaim

- [x] 6.1 Add Provider metric adapters with units, source, observedAt and bounded-cardinality telemetry. [STO-003, ALERT-011]
- [x] 6.2 Reconcile the canonical alert implementation and add executable storage alert rules using stable resource references. [ALERT-011]
- [x] 6.3 Implement Provider-specific retained-volume release/sanitize workflows with dependency checks, approval and evidence. [STO-005]
- [x] 6.4 Keep object/bucket services and ArtifactStorageProfile outside StorageClassBinding, with contract tests. [STO-006]

## 7. Migration, Validation And Cutover

- [x] 7.1 Add DB navigation migration, capability flag and compatibility redirects preserving cluster/namespace/offering/class query context. [UX-014]
- [x] 7.2 Run data import and rollback rehearsals without mutating or deleting target Kubernetes resources. [STO-002, UX-014]
- [x] 7.3 Publish operator documentation, security model, metric-source matrix, runbooks and deprecation timeline. [STO-003, STO-004, STO-005]
- [x] 7.4 Run unit, contract, integration, conformance and E2E suites plus `openspec validate --all --strict`; verify and archive only after exit criteria pass. [STO-001..006, RT-011..012, UX-012..014, ALERT-011]

## 8. N/A Assessments

- [x] 8.1 Confirm no new middleware/database is required; reuse PostgreSQL, Operation Store, Outbox and JetStream and record evidence in release notes. [STO-004]
- [x] 8.2 Confirm large-file/data-plane proxy work is N/A because storage payloads remain outside Platform API. [STO-006]
