/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type storage_metric_snapshot_schema = {
    schemaVersion: string;
    providerId: string;
    targetId: string;
    resourceKind: 'StorageBackend' | 'WorkloadStorageOffering' | 'StorageClassBinding' | 'StorageClass' | 'PersistentVolumeClaim' | 'PersistentVolume';
    resourceUid: string;
    metrics: Array<{
        kind: 'capacity' | 'usage' | 'iops' | 'throughput' | 'latency' | 'health';
        unit: 'By' | '1/s' | 'By/s' | 's' | '1';
        value?: number;
        status: 'Known' | 'Elastic' | 'Unknown' | 'NotReported';
        applicability: 'Applicable' | 'Unsupported' | 'Unknown';
        source: string;
        observedAt: string;
        freshness: 'Fresh' | 'Stale' | 'Unknown';
    }>;
};
