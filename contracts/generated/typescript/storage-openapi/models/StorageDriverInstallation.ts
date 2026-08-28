/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { storage_condition_schema } from './storage_condition_schema';
export type StorageDriverInstallation = {
    schemaVersion: string;
    id: string;
    tenantId: string;
    targetId: string;
    packageId: string;
    packageVersion: string;
    operationId?: string;
    lifecycleState: 'Pending' | 'Installing' | 'Ready' | 'Upgrading' | 'Degraded' | 'Failed' | 'Uninstalling' | 'Removed';
    healthState: 'Unknown' | 'Healthy' | 'Degraded' | 'Unhealthy';
    source: string;
    observedAt: string;
    freshness: 'Fresh' | 'Stale' | 'Unknown';
    conditions: Array<storage_condition_schema>;
};
