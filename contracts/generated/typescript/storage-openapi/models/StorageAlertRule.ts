/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type StorageAlertRule = {
    schemaVersion: string;
    id: string;
    name: string;
    description?: string;
    severity: 'critical' | 'warning' | 'info';
    enabled: boolean;
    resource: {
        tenantId: string;
        targetId: string;
        kind: 'StorageBackend' | 'WorkloadStorageOffering' | 'StorageClassBinding' | 'StorageClass' | 'PersistentVolumeClaim' | 'PersistentVolume';
        uid: string;
        namespace?: string;
        name?: string;
    };
    metric: {
        providerId: string;
        kind: 'capacity' | 'usage' | 'iops' | 'throughput' | 'latency' | 'health';
        unit: 'By' | '1/s' | 'By/s' | 's' | 1;
        source: string;
        freshFor: string;
        operator: 'gt' | 'gte' | 'lt' | 'lte';
        threshold: number;
    };
    duration: string;
    channels: Array<Record<string, any>>;
    context: Record<string, any>;
    version: number;
};
