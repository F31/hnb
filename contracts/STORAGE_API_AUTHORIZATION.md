# Workload Storage API Authorization

`contracts/openapi/storage/v1/openapi.yaml` is the source of truth for the
versioned workload storage API. Every operation carries an
`x-hnb-authorization` contract that maps directly to the existing
`ScopedPermission` tuple. The owning service must evaluate that tuple against
the verified active tenant on every request; browser visibility is not an
authorization decision.

| Endpoint | Resource kind | Action | Resource ID |
|---|---|---|---|
| `GET /api/v1/storage/overview` | `storageOverview` | `read` | none |
| `GET /api/v1/storage/backends` | `storageBackend` | `list` | none |
| `POST /api/v1/storage/backends` | `storageBackend` | `create` | none |
| `GET /api/v1/storage/backends/{backendId}` | `storageBackend` | `read` | `backendId` |
| `PUT /api/v1/storage/backends/{backendId}` | `storageBackend` | `update` | `backendId` |
| `DELETE /api/v1/storage/backends/{backendId}` | `storageBackend` | `delete` | `backendId` |
| `GET /api/v1/storage/offerings` | `workloadStorageOffering` | `list` | none |
| `POST /api/v1/storage/offerings` | `workloadStorageOffering` | `create` | none |
| `GET /api/v1/storage/offerings/{offeringId}` | `workloadStorageOffering` | `read` | `offeringId` |
| `PUT /api/v1/storage/offerings/{offeringId}` | `workloadStorageOffering` | `update` | `offeringId` |
| `DELETE /api/v1/storage/offerings/{offeringId}` | `workloadStorageOffering` | `delete` | `offeringId` |
| `GET /api/v1/storage/driver-installations` | `storageDriverInstallation` | `list` | none |
| `GET /api/v1/storage/targets/{targetId}/inventory` | `storageInventory` | `read` | `targetId` |
| `GET /api/v1/storage/targets/{targetId}/metrics` | `storageInventory` | `read` | `targetId` |
| `GET /api/v1/storage/offerings/{offeringId}/bindings` | `storageClassBinding` | `list` | `offeringId` |
| `POST /api/v1/storage/offerings/{offeringId}/bindings` | `storageClassBinding` | `create` | `offeringId` |
| `GET /api/v1/storage/bindings/{bindingId}` | `storageClassBinding` | `read` | `bindingId` |
| `PUT /api/v1/storage/bindings/{bindingId}` | `storageClassBinding` | `update` | `bindingId` |
| `DELETE /api/v1/storage/bindings/{bindingId}` | `storageClassBinding` | `delete` | `bindingId` |
| `GET /api/v1/storage/alert-rules` | `storageAlertRule` | `list` | none |
| `POST /api/v1/storage/alert-rules` | `storageAlertRule` | `create` | none |

Storage alert rule creation requires `Idempotency-Key`, like every persistent
storage POST. The service rejects missing or malformed keys before validation.

Target and offering IDs are context, not tenant proof. The service must bind
them to the active tenant before returning data. Inaccessible target- and
offering-scoped resources return a non-enumerating `404`.

## Generic Proxy Deprecation

The generic Kubernetes path/method proxy is deprecated for all storage
management. New storage callers must use the dedicated `/api/v1/storage/*`
contracts and must not receive storage access through a broad proxy
permission. Generic proxy execution is not an authorization boundary.

The v1 inventory endpoints remain read-only and never fan out to a
RuntimeTarget. Backend, offering, and binding CRUD changes only PostgreSQL
desired-state records, requires `Idempotency-Key`, and uses `If-Match`/`ETag`
for optimistic concurrency. Driver lifecycle, import/reconcile, StorageClass,
PVC expansion, or reclaim changes remain out of scope and must use later typed
intent endpoints that create an immutable ExecutionPlan and Operation.
