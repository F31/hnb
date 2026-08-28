/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { storage_condition_schema } from './storage_condition_schema';
export type storage_class_binding_schema = {
    schemaVersion: string;
    id: string;
    tenantId: string;
    offeringId: string;
    offeringVersion: number;
    targetId: string;
    bindingTarget: string;
    storageClassName: string;
    storageClassUid: string;
    storageClassResourceVersion: string;
    syncState: 'Discovered' | 'Imported' | 'Active' | 'Drifted' | 'Rejected' | 'Retired';
    isDefault: boolean;
    source: string;
    freshness: 'Fresh' | 'Stale' | 'Unknown';
    topology?: Record<string, Array<string>>;
    conditions: Array<storage_condition_schema>;
    version: number;
    observedAt: string;
    createdAt: string;
    updatedAt: string;
};
