/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { storage_condition_schema } from './storage_condition_schema';
export type storage_inventory_schema = {
    schemaVersion: string;
    tenantId: string;
    targetId: string;
    source: string;
    observedAt: string;
    freshness: 'Fresh' | 'Stale' | 'Unknown';
    storageClasses: Array<{
        uid: string;
        resourceVersion: string;
        name: string;
        provisioner: string;
        reclaimPolicy: 'Delete' | 'Retain';
        volumeBindingMode: 'Immediate' | 'WaitForFirstConsumer';
        allowVolumeExpansion: boolean;
        isDefault: boolean;
        source: string;
        observedAt: string;
        freshness: 'Fresh' | 'Stale' | 'Unknown';
        conditions: Array<storage_condition_schema>;
    }>;
    csiDrivers: Array<{
        uid: string;
        resourceVersion: string;
        name: string;
        attachRequired: boolean;
        podInfoOnMount: boolean;
        storageCapacity: boolean;
        source: string;
        observedAt: string;
        freshness: 'Fresh' | 'Stale' | 'Unknown';
        conditions: Array<storage_condition_schema>;
    }>;
    csiNodes: Array<{
        uid: string;
        resourceVersion: string;
        name: string;
        drivers: Array<string>;
        source: string;
        observedAt: string;
        freshness: 'Fresh' | 'Stale' | 'Unknown';
    }>;
    csiStorageCapacities: Array<{
        uid: string;
        resourceVersion: string;
        name: string;
        namespace: string;
        storageClassName: string;
        status: 'Known' | 'Elastic' | 'Unknown' | 'NotReported';
        value?: number;
        unit: 'By';
        source: string;
        observedAt: string;
        freshness: 'Fresh' | 'Stale' | 'Unknown';
    }>;
    volumeAttachments: Array<{
        uid: string;
        resourceVersion: string;
        name: string;
        driverName: string;
        nodeName: string;
        persistentVolumeName: string;
        attached: boolean;
        source: string;
        observedAt: string;
        freshness: 'Fresh' | 'Stale' | 'Unknown';
    }>;
    snapshotApi: {
        status: 'Supported' | 'Unsupported' | 'NotInstalled' | 'Unknown';
        apiVersion?: string;
        source: string;
        observedAt: string;
        freshness: 'Fresh' | 'Stale' | 'Unknown';
    };
};
