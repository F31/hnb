# Workload Storage Release Evidence: Tasks 8.1 And 8.2

## Release Decision

This change requires no new middleware or database. It reuses the platform's existing:

| Concern | Reused capability | Repository evidence | Release consequence |
|---|---|---|---|
| Desired state and inventory projections | PostgreSQL | `database/postgresql/migrations/069_storage_inventory_projection.sql` and `070_storage_desired_state.sql` through `076_storage_alert_rules.sql` | Apply additive migrations to the existing platform database; do not provision another database. |
| Governed lifecycle and reconcile work | PostgreSQL Operation Store | `cmd/platform-api/internal/store/store.go` and storage intent routes in `contracts/openapi/storage/v1/openapi.yaml` | Existing Operation workers execute plans; no storage-specific workflow engine is installed. |
| Atomic dispatch | Transactional Outbox | `cmd/platform-api/internal/store/operations.go` and its integration tests | Storage operations commit through the existing transaction/outbox path; no second queue or scheduler is introduced. |
| Asynchronous command/event transport | NATS JetStream | `deploy/nats/minimal/nats-deployment.yaml`, `deploy/nats/lite-ha/nats-deployment.yaml` and `openspec/architecture.md` | Reuse the installed JetStream deployment and its existing profile sizing/HA policy; no new stream technology or broker is required. |

PostgreSQL remains authoritative. JetStream transports bounded references, commands and events and is neither a storage inventory fact source nor a backup authority. No cache, object store, transfer service or side database is added by this change.

## Large-File And Data-Plane Assessment

Large-file/data-plane proxy work is **N/A**. The Platform API owns storage metadata, desired state, observations and Operation references only. Actual workload bytes remain in Kubernetes PV/PVC data paths or provider-owned storage systems.

- `contracts/openapi/storage/v1/openapi.yaml` exposes bounded JSON control-plane endpoints only. It has no binary or multipart media type and no upload, download, payload or data-plane proxy route.
- Runtime observations contain normalized inventory metadata and stable resource identity, not volume contents.
- Operation and Outbox records contain identifiers, plans, conditions and evidence references, not storage payload bytes.
- JetStream carries commands/events and references only. It is not a volume transfer path.
- Object/bucket payload handling and `ArtifactStorageProfile` remain outside this workload-storage bounded context.
- `scripts/contracts.test.mjs` rejects storage paths named as upload/download/proxy/payload routes, binary schemas, and octet-stream or multipart request/response media types.

Any future requirement to copy, upload, download, stream, back up or restore storage payload bytes requires a separate design and threat/performance review. It must not be added to `/api/v1/storage` as an incidental route.

## Installation And Upgrade

Installation applies the existing numbered PostgreSQL migrations and rolls out updated Platform API, observer/projector, Operation worker and Portal artifacts. Existing PostgreSQL and JetStream endpoints, credentials, network policy, monitoring and capacity profiles are reused. There is no new service, PVC, port, certificate, middleware chart or database lifecycle to install.

Before upgrade, take the normal PostgreSQL backup. Apply migrations in numeric order, then roll existing components using the standard platform procedure. Operation Store, Outbox and event contracts retain their existing compatibility rules. Rollback disables the storage capability/routes and uses the supplied migration rollback path only when the platform's data-retention policy permits; it never rolls back target Kubernetes resources.

## Backup And Restore

The existing PostgreSQL backup must include storage desired state, inventory/read models, Operations and unpublished Outbox rows. JetStream persistence may be protected under the platform's existing broker procedure, but replay is not sufficient to reconstruct authority and no new storage-specific JetStream backup is required.

Kubernetes volume contents, snapshots and provider backend data are explicitly excluded from the Platform API backup. They remain subject to workload/provider backup policy.

Restore PostgreSQL through the existing platform restore procedure, restart the existing Outbox relay and Operation workers, and let RuntimeTarget observers reconcile current inventory. A metadata restore must not copy, overwrite, claim to restore or mark as sanitized any PV/PVC or provider payload.

## Uninstall And Rollback

There is no middleware/database instance to uninstall for this capability. Disable storage routes/navigation and stop storage-specific reconciliation before removing storage metadata where approved. Preserve audit and Operation evidence according to retention policy.

Capability uninstall must not delete Kubernetes PVs/PVCs, snapshots, backend data, provider-owned payloads, PostgreSQL/JetStream shared infrastructure or unrelated platform records. A storage driver uninstall is a distinct, authorized and audited Operation with its own dependency checks; it is not implied by uninstalling or rolling back this platform capability.

## Verification

Run the focused boundary check with:

```sh
npm run contracts:test
```

The storage OpenAPI test verifies the exact route set and IAM metadata in addition to the no-large-payload boundary, preventing an accidental storage data-plane route from entering the contract unnoticed.
