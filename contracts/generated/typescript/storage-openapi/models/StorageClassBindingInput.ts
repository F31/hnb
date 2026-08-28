/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type StorageClassBindingInput = {
    offeringVersion: number;
    targetId: string;
    bindingTarget: string;
    storageClassName: string;
    storageClassUid: string;
    storageClassResourceVersion: string;
    syncState: 'Discovered' | 'Imported' | 'Active' | 'Drifted' | 'Rejected' | 'Retired';
    isDefault: boolean;
    source: string;
    observedAt: string;
    freshness: 'Fresh' | 'Stale' | 'Unknown';
    topology?: Record<string, Array<string>>;
};
