/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { problem_details_schema } from '../models/problem_details_schema';
import type { ProviderBackendSchemaList } from '../models/ProviderBackendSchemaList';
import type { RetainedVolumeIntent } from '../models/RetainedVolumeIntent';
import type { storage_alert_rule_schema } from '../models/storage_alert_rule_schema';
import type { storage_backend_schema } from '../models/storage_backend_schema';
import type { storage_class_binding_schema } from '../models/storage_class_binding_schema';
import type { storage_inventory_schema } from '../models/storage_inventory_schema';
import type { StorageAlertRule } from '../models/StorageAlertRule';
import type { StorageBackendInput } from '../models/StorageBackendInput';
import type { StorageBackendList } from '../models/StorageBackendList';
import type { StorageClassBindingImportIntent } from '../models/StorageClassBindingImportIntent';
import type { StorageClassBindingInput } from '../models/StorageClassBindingInput';
import type { StorageClassBindingList } from '../models/StorageClassBindingList';
import type { StorageClassBindingReconcileIntent } from '../models/StorageClassBindingReconcileIntent';
import type { StorageDriverInstallationList } from '../models/StorageDriverInstallationList';
import type { StorageDriverLifecycleIntent } from '../models/StorageDriverLifecycleIntent';
import type { StorageIntentReceipt } from '../models/StorageIntentReceipt';
import type { StorageMetricSnapshotList } from '../models/StorageMetricSnapshotList';
import type { StorageOverview } from '../models/StorageOverview';
import type { workload_storage_offering_schema } from '../models/workload_storage_offering_schema';
import type { WorkloadStorageOfferingInput } from '../models/WorkloadStorageOfferingInput';
import type { WorkloadStorageOfferingList } from '../models/WorkloadStorageOfferingList';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class StorageService {
    /**
     * Get the tenant storage supply overview
     * Reads tenant-bound projections and independently authorizes storageOverview:read.
     * @returns StorageOverview Source-attributed storage supply summary.
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static getStorageOverview({
        xCorrelationId,
    }: {
        xCorrelationId: string,
    }): CancelablePromise<StorageOverview | problem_details_schema> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/storage/overview',
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            errors: {
                403: `RFC 9457 problem response.`,
            },
        });
    }
    /**
     * List tenant-visible storage systems
     * Storage systems are represented by StorageBackend projections and require storageBackend:list.
     * @returns StorageBackendList Tenant-visible storage backend projections.
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static listStorageBackends({
        xCorrelationId,
    }: {
        xCorrelationId: string,
    }): CancelablePromise<StorageBackendList | problem_details_schema> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/storage/backends',
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            errors: {
                403: `RFC 9457 problem response.`,
            },
        });
    }
    /**
     * Create a tenant storage backend desired-state record
     * @returns problem_details_schema RFC 9457 problem response.
     * @returns storage_backend_schema Storage backend record created without resolving its SecretReference.
     * @throws ApiError
     */
    public static createStorageBackend({
        xCorrelationId,
        idempotencyKey,
        requestBody,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        requestBody: StorageBackendInput,
    }): CancelablePromise<problem_details_schema | storage_backend_schema> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/storage/backends',
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }
    /**
     * List trusted storage backend provider form schemas
     * Returns only local declarative field metadata from the server allowlist; no script or remote URL is accepted or returned.
     * @returns ProviderBackendSchemaList Versioned provider schema allowlist.
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static listStorageProviderSchemas({
        xCorrelationId,
    }: {
        xCorrelationId: string,
    }): CancelablePromise<ProviderBackendSchemaList | problem_details_schema> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/storage/provider-schemas',
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
        });
    }
    /**
     * Get one tenant storage backend desired-state record
     * @returns storage_backend_schema Tenant-owned storage backend.
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static getStorageBackend({
        xCorrelationId,
        backendId,
    }: {
        xCorrelationId: string,
        backendId: string,
    }): CancelablePromise<storage_backend_schema | problem_details_schema> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/storage/backends/{backendId}',
            path: {
                'backendId': backendId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
        });
    }
    /**
     * Replace one storage backend desired-state record
     * @returns storage_backend_schema Updated storage backend record.
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static updateStorageBackend({
        xCorrelationId,
        backendId,
        ifMatch,
        idempotencyKey,
        requestBody,
    }: {
        xCorrelationId: string,
        backendId: string,
        /**
         * Quoted integer version from the latest ETag, for example `"3"`.
         */
        ifMatch: string,
        idempotencyKey: string,
        requestBody: StorageBackendInput,
    }): CancelablePromise<storage_backend_schema | problem_details_schema> {
        return __request(OpenAPI, {
            method: 'PUT',
            url: '/api/v1/storage/backends/{backendId}',
            path: {
                'backendId': backendId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'If-Match': ifMatch,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                412: `RFC 9457 problem response.`,
                428: `RFC 9457 problem response.`,
            },
        });
    }
    /**
     * Delete one storage backend desired-state record
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static deleteStorageBackend({
        xCorrelationId,
        backendId,
        ifMatch,
        idempotencyKey,
    }: {
        xCorrelationId: string,
        backendId: string,
        /**
         * Quoted integer version from the latest ETag, for example `"3"`.
         */
        ifMatch: string,
        idempotencyKey: string,
    }): CancelablePromise<problem_details_schema> {
        return __request(OpenAPI, {
            method: 'DELETE',
            url: '/api/v1/storage/backends/{backendId}',
            path: {
                'backendId': backendId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'If-Match': ifMatch,
                'Idempotency-Key': idempotencyKey,
            },
            errors: {
                412: `RFC 9457 problem response.`,
                428: `RFC 9457 problem response.`,
            },
        });
    }
    /**
     * List tenant-visible workload storage offerings
     * Returns offerings owned by the active tenant after workloadStorageOffering:list authorization.
     * @returns WorkloadStorageOfferingList Tenant-visible workload storage offerings.
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static listWorkloadStorageOfferings({
        xCorrelationId,
    }: {
        xCorrelationId: string,
    }): CancelablePromise<WorkloadStorageOfferingList | problem_details_schema> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/storage/offerings',
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            errors: {
                403: `RFC 9457 problem response.`,
            },
        });
    }
    /**
     * Create a tenant workload storage offering
     * @returns problem_details_schema RFC 9457 problem response.
     * @returns workload_storage_offering_schema Workload storage offering created.
     * @throws ApiError
     */
    public static createWorkloadStorageOffering({
        xCorrelationId,
        idempotencyKey,
        requestBody,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        requestBody: WorkloadStorageOfferingInput,
    }): CancelablePromise<problem_details_schema | workload_storage_offering_schema> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/storage/offerings',
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }
    /**
     * Get one tenant workload storage offering
     * @returns workload_storage_offering_schema Tenant-owned workload storage offering.
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static getWorkloadStorageOffering({
        xCorrelationId,
        offeringId,
    }: {
        xCorrelationId: string,
        offeringId: string,
    }): CancelablePromise<workload_storage_offering_schema | problem_details_schema> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/storage/offerings/{offeringId}',
            path: {
                'offeringId': offeringId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
        });
    }
    /**
     * Replace one workload storage offering
     * @returns workload_storage_offering_schema Updated workload storage offering.
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static updateWorkloadStorageOffering({
        xCorrelationId,
        offeringId,
        ifMatch,
        idempotencyKey,
        requestBody,
    }: {
        xCorrelationId: string,
        offeringId: string,
        /**
         * Quoted integer version from the latest ETag, for example `"3"`.
         */
        ifMatch: string,
        idempotencyKey: string,
        requestBody: WorkloadStorageOfferingInput,
    }): CancelablePromise<workload_storage_offering_schema | problem_details_schema> {
        return __request(OpenAPI, {
            method: 'PUT',
            url: '/api/v1/storage/offerings/{offeringId}',
            path: {
                'offeringId': offeringId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'If-Match': ifMatch,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                412: `RFC 9457 problem response.`,
                428: `RFC 9457 problem response.`,
            },
        });
    }
    /**
     * Delete one workload storage offering
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static deleteWorkloadStorageOffering({
        xCorrelationId,
        offeringId,
        ifMatch,
        idempotencyKey,
    }: {
        xCorrelationId: string,
        offeringId: string,
        /**
         * Quoted integer version from the latest ETag, for example `"3"`.
         */
        ifMatch: string,
        idempotencyKey: string,
    }): CancelablePromise<problem_details_schema> {
        return __request(OpenAPI, {
            method: 'DELETE',
            url: '/api/v1/storage/offerings/{offeringId}',
            path: {
                'offeringId': offeringId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'If-Match': ifMatch,
                'Idempotency-Key': idempotencyKey,
            },
            errors: {
                412: `RFC 9457 problem response.`,
                428: `RFC 9457 problem response.`,
            },
        });
    }
    /**
     * List observed storage driver installations
     * Returns installation and health projections after storageDriverInstallation:list authorization.
     * @returns StorageDriverInstallationList Tenant-visible storage driver installation projections.
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static listStorageDriverInstallations({
        xCorrelationId,
    }: {
        xCorrelationId: string,
    }): CancelablePromise<StorageDriverInstallationList | problem_details_schema> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/storage/driver-installations',
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            errors: {
                403: `RFC 9457 problem response.`,
            },
        });
    }
    /**
     * Install a storage driver through immutable planning and Operation
     * Returns references only; Portal never installs into the target synchronously.
     * @returns problem_details_schema RFC 9457 problem response.
     * @returns StorageIntentReceipt Immutable plan and Operation references were committed.
     * @throws ApiError
     */
    public static installStorageDriver({
        xCorrelationId,
        installationId,
        idempotencyKey,
        requestBody,
    }: {
        xCorrelationId: string,
        installationId: string,
        idempotencyKey: string,
        requestBody: StorageDriverLifecycleIntent,
    }): CancelablePromise<problem_details_schema | StorageIntentReceipt> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/storage/driver-installations/{installationId}/intents/install',
            path: {
                'installationId': installationId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }
    /**
     * Upgrade a storage driver through immutable planning and Operation
     * @returns problem_details_schema RFC 9457 problem response.
     * @returns StorageIntentReceipt Immutable plan and Operation references were committed.
     * @throws ApiError
     */
    public static upgradeStorageDriver({
        xCorrelationId,
        installationId,
        idempotencyKey,
        requestBody,
    }: {
        xCorrelationId: string,
        installationId: string,
        idempotencyKey: string,
        requestBody: StorageDriverLifecycleIntent,
    }): CancelablePromise<problem_details_schema | StorageIntentReceipt> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/storage/driver-installations/{installationId}/intents/upgrade',
            path: {
                'installationId': installationId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }
    /**
     * Uninstall a storage driver through immutable planning and Operation
     * @returns problem_details_schema RFC 9457 problem response.
     * @returns StorageIntentReceipt Immutable plan and Operation references were committed.
     * @throws ApiError
     */
    public static uninstallStorageDriver({
        xCorrelationId,
        installationId,
        idempotencyKey,
        requestBody,
    }: {
        xCorrelationId: string,
        installationId: string,
        idempotencyKey: string,
        requestBody: StorageDriverLifecycleIntent,
    }): CancelablePromise<problem_details_schema | StorageIntentReceipt> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/storage/driver-installations/{installationId}/intents/uninstall',
            path: {
                'installationId': installationId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }
    /**
     * Get projected storage inventory for one target
     * Inaccessible targets return a non-enumerating 404; the request requires storageInventory:read scoped to targetId.
     * @returns storage_inventory_schema Last projected target storage inventory with source and freshness.
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static getTargetStorageInventory({
        xCorrelationId,
        targetId,
    }: {
        xCorrelationId: string,
        targetId: string,
    }): CancelablePromise<storage_inventory_schema | problem_details_schema> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/storage/targets/{targetId}/inventory',
            path: {
                'targetId': targetId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            errors: {
                403: `RFC 9457 problem response.`,
                404: `RFC 9457 problem response.`,
            },
        });
    }
    /**
     * List normalized Provider storage metric snapshots for one target
     * Returns projected snapshots without target fan-out. Unavailable metrics have no value; stable resource references are never Prometheus labels.
     * @returns StorageMetricSnapshotList Tenant-bound normalized storage metric snapshots.
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static listTargetStorageMetrics({
        xCorrelationId,
        targetId,
    }: {
        xCorrelationId: string,
        targetId: string,
    }): CancelablePromise<StorageMetricSnapshotList | problem_details_schema> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/storage/targets/{targetId}/metrics',
            path: {
                'targetId': targetId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            errors: {
                404: `RFC 9457 problem response.`,
            },
        });
    }
    /**
     * List target StorageClass bindings for one offering
     * Inaccessible offerings return a non-enumerating 404; the request requires storageClassBinding:list scoped to offeringId.
     * @returns StorageClassBindingList Authorized target-local StorageClass bindings for the offering.
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static listStorageOfferingBindings({
        xCorrelationId,
        offeringId,
    }: {
        xCorrelationId: string,
        offeringId: string,
    }): CancelablePromise<StorageClassBindingList | problem_details_schema> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/storage/offerings/{offeringId}/bindings',
            path: {
                'offeringId': offeringId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            errors: {
                403: `RFC 9457 problem response.`,
                404: `RFC 9457 problem response.`,
            },
        });
    }
    /**
     * Create a tenant StorageClass binding desired-state record
     * @returns problem_details_schema RFC 9457 problem response.
     * @returns storage_class_binding_schema StorageClass binding desired-state record created; no target mutation is performed.
     * @throws ApiError
     */
    public static createStorageClassBinding({
        xCorrelationId,
        offeringId,
        idempotencyKey,
        requestBody,
    }: {
        xCorrelationId: string,
        offeringId: string,
        idempotencyKey: string,
        requestBody: StorageClassBindingInput,
    }): CancelablePromise<problem_details_schema | storage_class_binding_schema> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/storage/offerings/{offeringId}/bindings',
            path: {
                'offeringId': offeringId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }
    /**
     * Get one tenant StorageClass binding desired-state record
     * @returns storage_class_binding_schema Tenant-owned StorageClass binding.
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static getStorageClassBinding({
        xCorrelationId,
        bindingId,
    }: {
        xCorrelationId: string,
        bindingId: string,
    }): CancelablePromise<storage_class_binding_schema | problem_details_schema> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/storage/bindings/{bindingId}',
            path: {
                'bindingId': bindingId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
        });
    }
    /**
     * Replace one StorageClass binding desired-state record
     * @returns storage_class_binding_schema Updated StorageClass binding desired-state record.
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static updateStorageClassBinding({
        xCorrelationId,
        bindingId,
        ifMatch,
        idempotencyKey,
        requestBody,
    }: {
        xCorrelationId: string,
        bindingId: string,
        /**
         * Quoted integer version from the latest ETag, for example `"3"`.
         */
        ifMatch: string,
        idempotencyKey: string,
        requestBody: StorageClassBindingInput,
    }): CancelablePromise<storage_class_binding_schema | problem_details_schema> {
        return __request(OpenAPI, {
            method: 'PUT',
            url: '/api/v1/storage/bindings/{bindingId}',
            path: {
                'bindingId': bindingId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'If-Match': ifMatch,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                412: `RFC 9457 problem response.`,
                428: `RFC 9457 problem response.`,
            },
        });
    }
    /**
     * Delete one StorageClass binding desired-state record
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static deleteStorageClassBinding({
        xCorrelationId,
        bindingId,
        ifMatch,
        idempotencyKey,
    }: {
        xCorrelationId: string,
        bindingId: string,
        /**
         * Quoted integer version from the latest ETag, for example `"3"`.
         */
        ifMatch: string,
        idempotencyKey: string,
    }): CancelablePromise<problem_details_schema> {
        return __request(OpenAPI, {
            method: 'DELETE',
            url: '/api/v1/storage/bindings/{bindingId}',
            path: {
                'bindingId': bindingId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'If-Match': ifMatch,
                'Idempotency-Key': idempotencyKey,
            },
            errors: {
                412: `RFC 9457 problem response.`,
                428: `RFC 9457 problem response.`,
            },
        });
    }
    /**
     * Import an observed StorageClass through immutable planning
     * Creates an ExecutionPlan and Operation reference only; it does not synchronously mutate the target or report target success.
     * @returns problem_details_schema RFC 9457 problem response.
     * @returns StorageIntentReceipt Immutable plan and operation references were committed.
     * @throws ApiError
     */
    public static importStorageClassBinding({
        xCorrelationId,
        offeringId,
        idempotencyKey,
        requestBody,
    }: {
        xCorrelationId: string,
        offeringId: string,
        idempotencyKey: string,
        requestBody: StorageClassBindingImportIntent,
    }): CancelablePromise<problem_details_schema | StorageIntentReceipt> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/storage/offerings/{offeringId}/bindings/intents/import',
            path: {
                'offeringId': offeringId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }
    /**
     * Reconcile a pinned StorageClass binding through immutable planning
     * Creates an ExecutionPlan and Operation reference only; completion is confirmed by later observation.
     * @returns problem_details_schema RFC 9457 problem response.
     * @returns StorageIntentReceipt Immutable plan and operation references were committed.
     * @throws ApiError
     */
    public static reconcileStorageClassBinding({
        xCorrelationId,
        bindingId,
        idempotencyKey,
        ifMatch,
        requestBody,
    }: {
        xCorrelationId: string,
        bindingId: string,
        idempotencyKey: string,
        /**
         * Quoted integer version from the latest ETag, for example `"3"`.
         */
        ifMatch: string,
        requestBody: StorageClassBindingReconcileIntent,
    }): CancelablePromise<problem_details_schema | StorageIntentReceipt> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/storage/bindings/{bindingId}/intents/reconcile',
            path: {
                'bindingId': bindingId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
                'If-Match': ifMatch,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }
    /**
     * Enter a Provider-specific manual release workflow for a retained volume
     * Requires a fresh Released PV/PVC/dependency snapshot and explicit Operation approval. The workflow preserves data-retained state and never deletes claimRef as sanitization.
     * @returns problem_details_schema RFC 9457 problem response.
     * @returns StorageIntentReceipt Immutable approval-gated plan and Operation references were committed.
     * @throws ApiError
     */
    public static releaseRetainedVolume({
        xCorrelationId,
        volumeId,
        idempotencyKey,
        requestBody,
    }: {
        xCorrelationId: string,
        volumeId: string,
        idempotencyKey: string,
        requestBody: RetainedVolumeIntent,
    }): CancelablePromise<problem_details_schema | StorageIntentReceipt> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/storage/retained-volumes/{volumeId}/intents/release',
            path: {
                'volumeId': volumeId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }
    /**
     * Request Provider-specific sanitization of a retained volume
     * Fails closed unless the Provider has current action-specific conformance evidence. Sanitized state requires conformant Provider evidence matching the fenced Operation.
     * @returns problem_details_schema RFC 9457 problem response.
     * @returns StorageIntentReceipt Immutable approval-gated plan and Operation references were committed.
     * @throws ApiError
     */
    public static sanitizeRetainedVolume({
        xCorrelationId,
        volumeId,
        idempotencyKey,
        requestBody,
    }: {
        xCorrelationId: string,
        volumeId: string,
        idempotencyKey: string,
        requestBody: RetainedVolumeIntent,
    }): CancelablePromise<problem_details_schema | StorageIntentReceipt> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/storage/retained-volumes/{volumeId}/intents/sanitize',
            path: {
                'volumeId': volumeId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }
    /**
     * List tenant storage metric alert rules
     * @returns any Tenant-owned storage alert rules.
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static listStorageAlertRules({
        xCorrelationId,
    }: {
        xCorrelationId: string,
    }): CancelablePromise<{
        schemaVersion: string;
        items: Array<StorageAlertRule>;
    } | problem_details_schema> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/storage/alert-rules',
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
        });
    }
    /**
     * Create a tenant storage metric alert rule
     * Validates metric applicability, status, freshness and tenant-owned notification SecretReferences before saving.
     * @returns problem_details_schema RFC 9457 problem response.
     * @returns StorageAlertRule Storage alert rule created.
     * @throws ApiError
     */
    public static createStorageAlertRule({
        xCorrelationId,
        idempotencyKey,
        requestBody,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        requestBody: storage_alert_rule_schema,
    }): CancelablePromise<problem_details_schema | StorageAlertRule> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/storage/alert-rules',
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                422: `RFC 9457 problem response.`,
            },
        });
    }
}
