/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { secret_reference_schema } from './secret_reference_schema';
import type { storage_condition_schema } from './storage_condition_schema';
export type storage_backend_schema = {
    schemaVersion: string;
    id: string;
    tenantId: string;
    providerType: string;
    providerSchemaVersion?: string;
    backendId: string;
    displayName: string;
    description?: string;
    secretReference?: secret_reference_schema;
    connectionState?: 'Unknown' | 'Connected' | 'Disconnected';
    healthState: 'Unknown' | 'Healthy' | 'Degraded' | 'Unhealthy';
    source: string;
    observedAt: string;
    freshness: 'Fresh' | 'Stale' | 'Unknown';
    capacity?: {
        status: 'Known' | 'Elastic' | 'Unknown' | 'NotReported';
        value?: number;
        unit: 'By';
        source: string;
        observedAt: string;
        freshness: 'Fresh' | 'Stale' | 'Unknown';
    };
    attributes?: Record<string, (string | number | boolean)>;
    conditions: Array<storage_condition_schema>;
    version: number;
    createdAt: string;
    updatedAt: string;
};
