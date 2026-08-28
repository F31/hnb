# Workload Storage Operator Guide

This guide covers the workload-storage control plane implemented by OpenSpec tasks 1-7.3. It describes platform mechanisms, not vendor readiness. The bundled conformance evidence is fixture data with `providerImplemented=false` and does not certify generic CSI, NFS, Ceph, cloud disk, local PV, or any vendor Provider for production.

## Architecture And Ownership

| Concern | Owner and authority |
|---|---|
| Driver package identity, compatibility, capability claims, digest/signature, and conformance evidence | Provider/Extension manifest and `StorageDriverPackage` |
| Driver desired lifecycle | `StorageDriverInstallation`; changes use immutable `ExecutionPlan` and `Operation` |
| Storage system credentials and health | Tenant-owned `StorageBackend`; credentials are `SecretReference` values only |
| Business service semantics | `WorkloadStorageOffering` |
| Target-local implementation | `StorageClassBinding`, pinned to target, StorageClass UID, and resourceVersion |
| Kubernetes storage facts | Cluster-agent observation projected into PostgreSQL; the apiserver does not fan out to a target |
| Resource Portal | Supply view: overview, systems, services, drivers/connectors, and alerts |
| Container Portal | Consumption view: StorageClass, PVC, PV, and Snapshot; it does not own Offering or Binding data |
| Object/bucket and artifact storage | Outside this control plane. App Market retains `ArtifactStorageProfile`; bucket services are not StorageClass bindings |

PostgreSQL holds desired state, ordered observations, metrics, alert rules, plans, Operations, and audit references. Existing Operation Store, transactional outbox, and JetStream carry commands/events; JetStream is neither storage authority nor data plane. Volume payloads stay between workloads and storage systems.

## Security Model And IAM

All storage APIs derive tenant identity from trusted request context. Client-supplied target, offering, binding, or resource IDs are context, not proof of tenancy. Resource-scoped lookups must remain tenant-bound; inaccessible target/offering records use non-enumerating responses. Portal action visibility is not authorization.

Grant only the required scoped permissions:

| Operation | IAM resource and action |
|---|---|
| Supply overview | `storageOverview:read` |
| Backends | `storageBackend:list/read/create/update/delete` |
| Offerings | `workloadStorageOffering:list/read/create/update/delete` |
| Driver inventory/lifecycle | `storageDriverInstallation:list/install/upgrade/uninstall`; HTTP lifecycle routes enforce `list/create/update/delete` respectively |
| Target inventory and metrics | `storageInventory:read`, scoped to target ID |
| Storage alert rules | `storageAlertRule:list/create` |
| Bindings | `storageClassBinding:list/read/create/update/delete/import/reconcile` |
| Retained-volume workflow | `retainedVolume:release` or `retainedVolume:sanitize`; HTTP intent routes enforce resource-scoped `execute` |

Additional controls:

- Dedicated `/api/v1/storage/*` routes replace generic Kubernetes proxy authorization for storage management. Storage resources in the cluster-agent proxy allowlist are GET-only.
- The observer ServiceAccount lists/watches storage and optional snapshot resources. The executor ServiceAccount has explicit mutation rules and is not used by the observer deployment. The current Helm chart also binds the executor to the observer role so it can preflight operations; do not run the observer pod as the executor account.
- Backend and notification credentials must be tenant-owned `SecretReference` records. Inline passwords, tokens, kubeconfigs, private keys, and Secret values are rejected from lifecycle/conformance evidence.
- Desired-state CRUD requires `Idempotency-Key`; updates use `If-Match`/ETag. Target operations additionally pin projection version, Kubernetes UID/resourceVersion where applicable, Provider version/digest, and fencing generation.
- Retained-volume and Provider lifecycle changes require the normal Operation approval/audit path. Preserve actor, tenant, target, plan, Operation, correlation ID, and terminal evidence.

## Discovery And Freshness

The cluster agent performs bounded, paginated reads for StorageClass (512), CSIDriver (128), CSINode (5,000), CSIStorageCapacity (4,096), VolumeAttachment (10,000), and snapshot objects (512 classes, 10,000 snapshots, 10,000 contents). Each page has a request timeout, and repeated continuation tokens, page-limit exhaustion, or item-limit overflow fail the observation instead of returning a partial healthy inventory.

Each fact carries Kubernetes UID, resourceVersion, source, and `observedAt`. Full observations tombstone omitted resources; Delta observations update or tombstone only explicit resources. The projector applies tenant/observer binding plus generation and sequence ordering before updating the read model.

Snapshot discovery distinguishes:

- `Installed`: `snapshot.storage.k8s.io/v1` and all three listable resources are present.
- `NotInstalled`: the API group/resource endpoint is absent.
- `Unsupported`: v1 or a required listable resource is unavailable.

Freshness is computed from `observedAt + runtime_targets.stale_threshold_seconds`. Treat `Stale` or `Unknown` as unavailable evidence: retain the last facts for diagnosis, do not infer health, and do not authorize freshness-sensitive operations. Missing capacity is `Elastic`, `Unknown`, or `NotReported`, never zero. A StorageClass without matching registration/workload evidence remains visible with a missing-driver condition; a single evidence source never proves a driver Ready.

Operator checks:

```sh
curl -H "Authorization: Bearer $TOKEN" -H "X-Correlation-ID: $CORRELATION_ID" \
  "$HNB_API/api/v1/storage/targets/$TARGET_ID/inventory"

kubectl auth can-i list storageclasses.storage.k8s.io \
  --as="system:serviceaccount:$NAMESPACE:$OBSERVER_SERVICE_ACCOUNT"
kubectl auth can-i patch storageclasses.storage.k8s.io \
  --as="system:serviceaccount:$NAMESPACE:$OBSERVER_SERVICE_ACCOUNT"
```

The first `kubectl auth` check should return `yes`; the second should return `no`.

## Metric Source Matrix

Provider adapters declare one stable source, a positive freshness window, and applicability for all canonical metrics. Sources below are allowable evidence classes, not bundled vendor integrations.

| Metric | Canonical unit | Permitted source/evidence | Absence behavior |
|---|---|---|---|
| Capacity | `By` | CSIStorageCapacity for target-local provisioning capacity; Provider/exporter for backend totals; provider quota may justify `Elastic` | `Elastic`, `Unknown`, or `NotReported`; never inferred from PV/PVC sums |
| Usage | `By` | Conformant Provider/exporter measurement | `NotReported` or `Unsupported` |
| IOPS | `1/s` | Conformant Provider/exporter measurement | `NotReported` or `Unsupported`; CSI alone is insufficient |
| Throughput | `By/s` | Conformant Provider/exporter measurement | `NotReported` or `Unsupported` |
| Latency | `s` | Conformant Provider/exporter measurement | `NotReported` or `Unsupported` |
| Health | `1` | Provider/exporter or separately correlated registration/controller/node evidence | `Unknown` or `NotReported`; missing data is not healthy |

Normalization rejects undeclared capabilities, duplicate kinds, negative/non-finite values, values on unsupported metrics, and values attached to unavailable states. Prometheus exports only `provider`, `metric`, `unit`, `source`, `freshness`, and `applicability` on `hnb_storage_metric_value` and `hnb_storage_metric_age_seconds`. Tenant IDs, target IDs, resource UIDs, PVC/PV names, backend IDs, volume handles, offerings, and bindings must not become labels.

The `/api/v1/storage/targets/{targetId}/metrics` route returns tenant/target-bound normalized snapshots from PostgreSQL and marks expired rows stale at read time. Provider snapshot ingestion/adapter registration is not bundled, so production data still requires a conformant Provider integration and real evidence.

## Safe Retained-Volume Runbook

Never remove a PV `spec.claimRef` or label a volume sanitized based on Kubernetes phase alone.

1. Stop and escalate if the target observation is stale, the PV is not `Released` with reclaim policy `Retain`, deleted-PVC evidence is absent, or any Pod/StatefulSet dependency remains.
2. Verify tenant/target/volume identity, target projection version, PV and PVC UID/resourceVersion, the intended Provider, and an action-specific, unexpired Provider conformance reference.
3. Choose `release` only for an explicit manual workflow that preserves `dataRetained=true`. Choose `sanitize` only when the Provider can produce method, completion time, evidence reference, and SHA-256 evidence digest.
4. Submit the dedicated intent with a unique correlation ID and idempotency key. The body must acknowledge approval and include typed observation evidence; arbitrary parameters and inline secrets are not accepted.

```sh
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "X-Correlation-ID: $CORRELATION_ID" \
  -H "Idempotency-Key: $IDEMPOTENCY_KEY" \
  "$HNB_API/api/v1/storage/retained-volumes/$VOLUME_ID/intents/sanitize" \
  --data @retained-volume-intent.json
```

5. Review the immutable plan. It must contain one `storage.retained-volume.sanitize` or `.manual-release` Step, Provider/version/digest, explicit approval policy, target/PV/PVC/dependency fencing, and no compensation that claims to restore erased data.
6. Approve through the normal Operation approval API. Monitor the Operation; an HTTP `202` means committed references, not completed sanitization.
7. Accept `Sanitized` only with Provider evidence matching volume, Operation, Step, idempotency key, and fencing generation. `ManualReleaseRequired` must include `dataRetained=true` and must not make the volume reusable.
8. On timeout, conflict, missing evidence, or Provider failure, stop. Preserve the PV and evidence, mark data retained/unknown, and do not retry with changed identity under the same idempotency key.

There is no certified retained-volume Provider in this repository. Until one supplies current real conformance evidence, the safe operational outcome is fail closed/manual handling outside HNB without claiming sanitization.

## Driver Lifecycle And Conformance Runbook

1. Confirm the manifest binds package ID/version, provisioners, Kubernetes compatibility, capability claims, package digest/signature, Provider protocol `2.0.0`, and unexpired evidence.
2. Run the version-bound matrix contract:

```sh
go test ./cmd/provider-conformance/...
go run ./cmd/provider-conformance \
  -storage-matrix cmd/provider-conformance/testdata/storage-matrix.v1.json \
  -storage-evidence cmd/provider-conformance/testdata/storage-evidence.v1.json
```

3. Do not interpret the bundled fixture pass as certification. Real evidence must bind the exact package, Kubernetes, matrix, suite, HNB Core, and Provider protocol versions, set an implemented Provider, and remain current.
4. Before install/upgrade/uninstall, verify Provider registration, health/license flags, action declaration, action-specific evidence, target Kubernetes version, target projection version, and upgrade/rollback path. The resolver fails closed if any item is absent.
5. Submit only the dedicated driver intent. Verify the plan pins package/Provider digests and uses `storage.driver.install`, `.upgrade`, or `.uninstall`, idempotency, fencing, SecretReferences, and bounded scalar parameters.
6. For install, rollback is limited to operation-owned resources. For upgrade, rollback is limited to the declared management relationship and only when the package declares the prior version as a rollback target. Uninstall has no implied rollback.
7. Treat `202` as plan/Operation creation. Declare Ready only after fresh CSIDriver registration, CSINode/controller/node workload evidence, and consistent StorageClass references; package metadata alone is insufficient.
8. On failure, preserve the Operation and Provider output, execute only declared compensation, and reconcile from a later observation. Never report success from a synchronous proxy response.

## Alerts

Storage rules use tenant, target, resource kind, stable UID, and optional namespace/name plus Provider, metric kind/unit/source, freshness, threshold, duration, and optional Binding/Offering/Operation/runbook navigation. Rule creation rejects unsupported, unavailable, or stale metrics and validates tenant-owned channel `SecretReference` records. Evaluation inserts/deduplicates canonical `alert_instances` only while the normalized metric is `Applicable`, `Known`, and `Fresh`.

Investigate in this order: observation age, adapter/source identity, applicability and status, target/resource UID, related Binding/Offering, then Operation. Do not change an unavailable metric to zero to force evaluation. Resolve source/freshness first.

`/api/v1/storage/alert-rules` list/create routes have dedicated authorization metadata and storage OpenAPI coverage. Notification delivery must still be verified independently after rule evaluation.

## Compatibility Route Migration

Current supported routes are `/resource/storage` for supply and `/container/instances/storage` for consumption. Offering links to the consumption route preserve `target`, `cluster`, `offering`, and `storageClass`; access is still server-authorized.

Task 7.1 adds the `storage.supply` capability, database navigation version, canonical `/container/storage` route, and a query-preserving compatibility redirect from `/container/instances/storage`.

When those controls are implemented, migrate in stages:

1. Keep both routes and enable the new supply navigation only for a canary tenant.
2. Verify bookmarks and Offering links preserve `target`, `cluster`, `namespace`, `offering`, and `storageClass`, including URL encoding and unauthorized contexts.
3. Observe route errors and legacy traffic for one release cycle; keep APIs additive.
4. Enable compatibility redirects only after desktop/mobile E2E and rollback rehearsal pass.
5. Remove the legacy route only after the deprecation gates below are met.

## Rollback

For a Portal/API cutover failure, disable the future capability/navigation flag, restore the prior database navigation version, disable compatibility redirects, and keep `/container/instances/storage` reachable. Do not delete PostgreSQL observations, desired-state records, plans, Operations, audit evidence, or Kubernetes storage resources.

For discovery regressions, stop rollout of the observer, retain the last projection as stale, and restore the prior observer binary/RBAC. Do not rewrite generation/sequence cursors or convert stale facts to healthy empty inventory.

For Provider operation failures, stop new approvals, preserve immutable plans/Operations, and use only declared compensation. Database rollback and NATS replay do not undo target storage mutations. Reconcile target truth from fresh observations after recovery.

Task 7.2 import/rollback rehearsal evidence is recorded in `openspec/changes/generic-resource-storage-control-plane/evidence/task-7.2-storage-import-rehearsal.md`; the importer is PostgreSQL-only and verifies that observed inventory and Kubernetes resources are not mutated or deleted.

## Deprecation Timeline

| Stage | Gate | Operator policy |
|---|---|---|
| Now | Tasks 1-7.3 implemented | Generic storage proxy management is deprecated; use dedicated storage APIs. Keep the compatibility route. No vendor is certified. Provider metric ingestion and notification delivery require release evidence. |
| Cutover candidate | Tasks 7.1 and 7.2 complete; metrics/alert blockers closed; contract, integration, conformance, and E2E checks pass | Canary capability/navigation and compatibility redirects; retain rollback path and legacy route. |
| Deprecation notice | At least one successful release cycle with route telemetry, bookmark/context tests, and rollback evidence | Announce the exact legacy-route removal release; reject new integrations using generic proxy writes. |
| Removal | A subsequent major or explicitly breaking release after the notice window | Remove the legacy route/redirect only; retain audit/Operation/read-model history according to platform retention policy. |

No calendar removal date or vendor-readiness date is committed by this change.

## Release Validation

```sh
npm run contracts:check
go test ./cmd/cluster-agent/... ./cmd/apiserver/... ./cmd/platform-api/... ./cmd/provider-conformance/... ./pkg/storagemetrics/... ./pkg/alert/...
openspec validate --all --strict
```

Also run PostgreSQL migration/integration checks with `HNB_TEST_POSTGRES_DSN` where available, and Portal storage unit/E2E suites before cutover. Release approval must record Provider ingestion/notification delivery readiness and real Provider evidence separately from fixture conformance.

## References

- [Storage API authorization](../contracts/STORAGE_API_AUTHORIZATION.md)
- [Storage OpenAPI](../contracts/openapi/storage/v1/openapi.yaml)
- [Provider metric adapter contract](storage-provider-metrics.md)
- [Provider conformance matrix](storage-provider-conformance-matrix.md)
- [Artifact storage ownership boundary](../contracts/APP_MARKET_ARTIFACT_STORAGE.md)
