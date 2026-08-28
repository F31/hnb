/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { storage_condition_schema } from './storage_condition_schema';
export type workload_storage_offering_schema = {
    schemaVersion: string;
    id: string;
    scope: 'Tenant' | 'Global';
    tenantId?: string;
    backendId?: string;
    name: string;
    description?: string;
    consumptionModel: string;
    serviceMode: 'Block' | 'File';
    accessModes: Array<'ReadWriteOnce' | 'ReadOnlyMany' | 'ReadWriteMany' | 'ReadWriteOncePod'>;
    volumeExpansion: 'Supported' | 'Unsupported' | 'Unknown';
    snapshots: 'Supported' | 'Unsupported' | 'Unknown';
    clones: 'Supported' | 'Unsupported' | 'Unknown';
    topology?: Record<string, Array<string>>;
    protectionClass: string;
    conditions: Array<storage_condition_schema>;
    version: number;
    createdAt: string;
    updatedAt: string;
};
