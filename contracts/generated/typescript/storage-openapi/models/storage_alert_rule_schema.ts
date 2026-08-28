/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { secret_reference_schema } from './secret_reference_schema';
export type storage_alert_rule_schema = {
    name: string;
    description?: string;
    severity: 'critical' | 'warning' | 'info';
    enabled?: boolean;
    resource: {
        targetId: string;
        kind: 'StorageBackend' | 'WorkloadStorageOffering' | 'StorageClassBinding' | 'StorageClass' | 'PersistentVolumeClaim' | 'PersistentVolume';
        uid: string;
        namespace?: string;
        name?: string;
    };
    metric: {
        providerId: string;
        kind: 'capacity' | 'usage' | 'iops' | 'throughput' | 'latency' | 'health';
        unit: 'By' | '1/s' | 'By/s' | 's' | '1';
        source: string;
        freshFor: string;
        operator: 'gt' | 'gte' | 'lt' | 'lte';
        threshold: number;
    };
    duration: string;
    channels?: Array<{
        type: 'email' | 'webhook' | 'sms';
        configReference: string;
        secretReference: secret_reference_schema;
    }>;
    context?: {
        bindingId?: string;
        offeringId?: string;
        operationId?: string;
        runbookRef?: string;
        navigationRef?: string;
    };
};
